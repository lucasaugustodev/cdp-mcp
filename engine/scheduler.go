package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/recipes"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

// lastRunTimes tracks when each task was last executed (in-memory).
var lastRunTimes = make(map[string]time.Time)

// Start is the main engine loop. It runs forever, checking tasks every 10 seconds.
func Start() {
	log.Println("[engine] Task engine started")

	// Inject DOM observers for monitor/workflow tasks
	go StartMonitors()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		runCycle()
		<-ticker.C
	}
}

// runCycle loads all enabled tasks and executes any that are due.
func runCycle() {
	tasks := config.LoadTasks()
	now := time.Now()

	for _, task := range tasks {
		if !task.Enabled {
			continue
		}

		switch task.Type {
		case "polling":
			checkPollingTask(task, now)
		case "schedule":
			checkScheduleTask(task, now)
		}
	}
}

// checkPollingTask checks if a polling task's interval has elapsed and executes it if so.
func checkPollingTask(task config.Task, now time.Time) {
	var pc config.PollingConfig
	if err := json.Unmarshal(task.Config, &pc); err != nil {
		log.Printf("[engine] Failed to parse polling config for task %s: %v", task.ID, err)
		return
	}

	interval := parseInterval(pc.Interval)
	if interval == 0 {
		log.Printf("[engine] Invalid interval %q for task %s", pc.Interval, task.ID)
		return
	}

	lastRun, ok := lastRunTimes[task.ID]
	if !ok && task.LastRun != "" {
		lastRun, _ = time.Parse(time.RFC3339, task.LastRun)
	}

	if !lastRun.IsZero() && now.Sub(lastRun) < interval {
		return
	}

	log.Printf("[engine] Executing polling task %s (%s)", task.ID, task.Name)
	executeAction(task.AppID, pc.Action)
	updateTaskTimes(task, now, now.Add(interval))
}

// checkScheduleTask checks if the current time matches a scheduled task and executes it.
func checkScheduleTask(task config.Task, now time.Time) {
	var sc config.ScheduleConfig
	if err := json.Unmarshal(task.Config, &sc); err != nil {
		log.Printf("[engine] Failed to parse schedule config for task %s: %v", task.ID, err)
		return
	}

	if !matchesSchedule(sc.Cron, sc.Days, now) {
		return
	}

	// Prevent re-execution within the same minute
	lastRun, ok := lastRunTimes[task.ID]
	if ok && now.Sub(lastRun) < time.Minute {
		return
	}

	log.Printf("[engine] Executing schedule task %s (%s)", task.ID, task.Name)
	executeAction(task.AppID, sc.Action)
	updateTaskTimes(task, now, time.Time{})
}

// executeAction executes a single action for a given app.
func executeAction(appID string, action config.Action) {
	switch action.Type {
	case "recipe":
		executeRecipeAction(appID, action)
	case "prompt":
		log.Printf("[engine] Prompt action (AI execution not yet implemented): %s", action.Text)
		tools.LogActivity("engine:prompt", action.Text, "logged (not executed)")
	case "notify":
		log.Printf("[engine] Notify: %s", action.Text)
		tools.LogActivity("engine:notify", action.Text, "sent")
	default:
		log.Printf("[engine] Unknown action type: %s", action.Type)
	}
}

// executeRecipeAction loads and replays a recipe via CDP.
func executeRecipeAction(appID string, action config.Action) {
	recipe, err := recipes.LoadRecipe(action.RecipeID)
	if err != nil {
		log.Printf("[engine] Failed to load recipe %q: %v", action.RecipeID, err)
		tools.LogActivity("engine:recipe", action.RecipeID, fmt.Sprintf("error: %v", err))
		return
	}

	// Get connection for this app
	appState := tools.GetAppState(appID)
	if appState == nil || appState.Conn == nil || appState.Conn.IsClosed() {
		log.Printf("[engine] No active connection for app %q, skipping recipe %q", appID, action.RecipeID)
		tools.LogActivity("engine:recipe", action.RecipeID, "skipped: no connection")
		return
	}

	conn := appState.Conn
	executed := 0
	for i, step := range recipe.Steps {
		var stepErr error
		switch step.Action {
		case "click":
			stepErr = replayClickStep(conn, step)
		case "input", "change":
			stepErr = replayInputStep(conn, step)
		}
		if stepErr != nil {
			log.Printf("[engine] Recipe %q step %d failed: %v", action.RecipeID, i+1, stepErr)
		} else {
			executed++
		}
		time.Sleep(500 * time.Millisecond)
	}

	result := fmt.Sprintf("executed %d/%d steps", executed, len(recipe.Steps))
	log.Printf("[engine] Recipe %q complete: %s", action.RecipeID, result)
	tools.LogActivity("engine:recipe", action.RecipeID, result)
}

// replayClickStep finds and clicks an element from a recorded step.
func replayClickStep(conn *cdp.Connection, step cdp.RecordEvent) error {
	// Try by CSS selector first
	if step.Selector != "" {
		selectorJSON, _ := json.Marshal(step.Selector)
		js := fmt.Sprintf(`
			(() => {
				const el = document.querySelector(%s);
				if (!el) return 'not_found';
				const rect = el.getBoundingClientRect();
				return JSON.stringify({x: Math.round(rect.x + rect.width/2), y: Math.round(rect.y + rect.height/2)});
			})()
		`, string(selectorJSON))

		result, err := conn.EvaluateString(js)
		if err == nil && result != "not_found" {
			var pos struct {
				X int `json:"x"`
				Y int `json:"y"`
			}
			if json.Unmarshal([]byte(result), &pos) == nil {
				return conn.DispatchMouseClick(pos.X, pos.Y)
			}
		}
	}

	// Fallback: click at recorded coordinates
	if step.X > 0 && step.Y > 0 {
		return conn.DispatchMouseClick(step.X, step.Y)
	}

	return fmt.Errorf("no selector or coordinates for click step")
}

// replayInputStep focuses an element and sets its value from a recorded step.
func replayInputStep(conn *cdp.Connection, step cdp.RecordEvent) error {
	if step.Selector == "" {
		return fmt.Errorf("no selector for input step")
	}

	selectorJSON, _ := json.Marshal(step.Selector)
	valueJSON, _ := json.Marshal(step.Value)
	js := fmt.Sprintf(`
		(() => {
			const el = document.querySelector(%s);
			if (!el) return 'not_found';
			el.focus();
			el.value = %s;
			el.dispatchEvent(new Event('input', {bubbles: true}));
			el.dispatchEvent(new Event('change', {bubbles: true}));
			return 'ok';
		})()
	`, string(selectorJSON), string(valueJSON))

	result, err := conn.EvaluateString(js)
	if err != nil {
		return err
	}
	if result == "not_found" {
		return fmt.Errorf("element not found: %s", step.Selector)
	}
	return nil
}

// updateTaskTimes updates a task's LastRun and NextRun in config.
func updateTaskTimes(task config.Task, lastRun time.Time, nextRun time.Time) {
	lastRunTimes[task.ID] = lastRun
	task.LastRun = lastRun.Format(time.RFC3339)
	if !nextRun.IsZero() {
		task.NextRun = nextRun.Format(time.RFC3339)
	}
	if err := config.UpdateTask(task); err != nil {
		log.Printf("[engine] Failed to update task %s times: %v", task.ID, err)
	}
}

// parseInterval parses duration strings like "30m", "1h", "5m", "2h30m".
func parseInterval(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Try standard Go duration parsing first (handles "30m", "1h", "2h30m", "5m")
	d, err := time.ParseDuration(s)
	if err == nil {
		return d
	}

	// Fallback: try parsing as plain number of minutes
	if mins, err := strconv.Atoi(s); err == nil {
		return time.Duration(mins) * time.Minute
	}

	return 0
}

// matchesSchedule checks if the current time matches a schedule definition.
// cron: "HH:MM" format time string (also supports "weekdays 09:00" combined format)
// days: "daily", "weekdays" (mon-fri), "weekends" (sat-sun), or empty (= daily)
func matchesSchedule(cron string, days string, now time.Time) bool {
	timeStr := cron
	dayFilter := strings.ToLower(strings.TrimSpace(days))

	// Support combined format: "weekdays 09:00" in the cron field
	parts := strings.Fields(cron)
	if len(parts) == 2 {
		dayFilter = strings.ToLower(parts[0])
		timeStr = parts[1]
	}

	// Check day filter
	if !matchesDay(dayFilter, now) {
		return false
	}

	// Parse HH:MM
	timeParts := strings.Split(timeStr, ":")
	if len(timeParts) != 2 {
		return false
	}

	hour, err1 := strconv.Atoi(timeParts[0])
	minute, err2 := strconv.Atoi(timeParts[1])
	if err1 != nil || err2 != nil {
		return false
	}

	return now.Hour() == hour && now.Minute() == minute
}

// matchesDay checks if the current weekday matches the day filter.
func matchesDay(filter string, now time.Time) bool {
	switch filter {
	case "", "daily":
		return true
	case "weekdays":
		wd := now.Weekday()
		return wd >= time.Monday && wd <= time.Friday
	case "weekends":
		wd := now.Weekday()
		return wd == time.Saturday || wd == time.Sunday
	default:
		return true
	}
}
