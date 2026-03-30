package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
	"github.com/lucasaugustodev/cdp-mcp/recipes"
)

// RegisterRecording registers all recording tools.
func RegisterRecording(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "record_start",
		Description: "Start recording user interactions (clicks, inputs, changes) on the connected page. Events are captured for later replay.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleRecordStart)

	server.RegisterTool(mcp.Tool{
		Name:        "record_stop",
		Description: "Stop recording and return the captured steps. Optionally save as a named recipe.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Optional name to save the recipe as. If empty, just returns the steps without saving.",
				},
			},
		},
	}, handleRecordStop)

	server.RegisterTool(mcp.Tool{
		Name:        "record_list",
		Description: "List all saved recipes.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleRecordList)

	server.RegisterTool(mcp.Tool{
		Name:        "record_replay",
		Description: "Replay a saved recipe step by step. For each step: find element by selector, fallback to text/ariaLabel, then click.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the recipe to replay",
				},
			},
			"required": []string{"name"},
		},
	}, handleRecordReplay)
}

func handleRecordStart(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	if IsRecording() {
		return mcp.ErrorResult("Already recording. Stop the current recording first with record_stop.")
	}

	// Clear previous events
	GetRecordedEvents()

	// Set up event handler
	conn.OnRecordEvent(func(evt cdp.RecordEvent) {
		AppendRecordEvent(evt)
	})

	err := conn.StartRecording()
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to start recording: %v", err))
	}

	SetRecording(true)
	return mcp.TextResult("Recording started. Interact with the page and then use record_stop to capture the steps.")
}

func handleRecordStop(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	if !IsRecording() {
		return mcp.ErrorResult("Not currently recording. Start a recording first with record_start.")
	}

	err := conn.StopRecording()
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to stop recording: %v", err))
	}

	SetRecording(false)
	events := GetRecordedEvents()

	if len(events) == 0 {
		return mcp.TextResult("Recording stopped. No events were captured.")
	}

	// Save if name provided
	name, _ := args["name"].(string)
	if name != "" {
		err := recipes.SaveRecipe(name, events)
		if err != nil {
			return mcp.ErrorResult(fmt.Sprintf("Recording stopped with %d events, but save failed: %v", len(events), err))
		}
	}

	// Format the events
	data, _ := json.MarshalIndent(events, "", "  ")

	var lines []string
	lines = append(lines, fmt.Sprintf("Recording stopped. Captured %d events.", len(events)))
	if name != "" {
		lines = append(lines, fmt.Sprintf("Saved as recipe: %q", name))
	}
	lines = append(lines, "\nSteps:")
	for i, evt := range events {
		display := evt.AriaLabel
		if display == "" {
			display = evt.Text
		}
		if len(display) > 50 {
			display = display[:47] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %d. %s %s %q at (%d,%d) selector=%q",
			i+1, evt.Action, evt.TagName, display, evt.X, evt.Y, evt.Selector))
		if evt.Value != "" {
			lines = append(lines, fmt.Sprintf("     value=%q", evt.Value))
		}
	}
	lines = append(lines, "\nRaw JSON:")
	lines = append(lines, string(data))

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleRecordList(args map[string]interface{}) mcp.ToolResult {
	names := recipes.ListRecipes()
	if len(names) == 0 {
		return mcp.TextResult("No saved recipes. Use record_start/record_stop to create one.")
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Saved recipes (%d):", len(names)))
	for i, name := range names {
		recipe, err := recipes.LoadRecipe(name)
		stepCount := 0
		if err == nil {
			stepCount = len(recipe.Steps)
		}
		lines = append(lines, fmt.Sprintf("  [%d] %s (%d steps)", i, name, stepCount))
	}
	lines = append(lines, "\nUse record_replay with a recipe name to replay it.")

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleRecordReplay(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.ErrorResult("Missing 'name' argument.")
	}

	recipe, err := recipes.LoadRecipe(name)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to load recipe %q: %v", name, err))
	}

	if len(recipe.Steps) == 0 {
		return mcp.TextResult(fmt.Sprintf("Recipe %q has no steps.", name))
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Replaying recipe %q (%d steps)...", name, len(recipe.Steps)))

	for i, step := range recipe.Steps {
		display := step.AriaLabel
		if display == "" {
			display = step.Text
		}
		if len(display) > 50 {
			display = display[:47] + "..."
		}

		lines = append(lines, fmt.Sprintf("\nStep %d: %s %s %q", i+1, step.Action, step.TagName, display))

		switch step.Action {
		case "click":
			err := replayClick(conn, step)
			if err != nil {
				lines = append(lines, fmt.Sprintf("  FAILED: %v", err))
			} else {
				lines = append(lines, "  OK")
			}

		case "input", "change":
			err := replayInput(conn, step)
			if err != nil {
				lines = append(lines, fmt.Sprintf("  FAILED: %v", err))
			} else {
				lines = append(lines, fmt.Sprintf("  OK (value=%q)", step.Value))
			}
		}

		// Wait between steps
		time.Sleep(500 * time.Millisecond)
	}

	lines = append(lines, fmt.Sprintf("\nReplay complete. Executed %d steps.", len(recipe.Steps)))
	return mcp.TextResult(strings.Join(lines, "\n"))
}

// replayClick tries to find and click an element from a recorded step.
func replayClick(conn *cdp.Connection, step cdp.RecordEvent) error {
	// Try by selector first
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

	// Fallback: find by text/ariaLabel
	searchText := step.AriaLabel
	if searchText == "" {
		searchText = step.Text
	}
	if searchText != "" {
		searchJSON, _ := json.Marshal(strings.ToLower(searchText))
		js := fmt.Sprintf(`
			(() => {
				const query = %s;
				const allEls = document.querySelectorAll('*');
				for (const el of allEls) {
					const text = (el.textContent || '').trim().toLowerCase();
					const ariaLabel = (el.getAttribute('aria-label') || '').toLowerCase();
					if (text.includes(query) || ariaLabel.includes(query)) {
						const rect = el.getBoundingClientRect();
						if (rect.width > 2 && rect.height > 2) {
							return JSON.stringify({x: Math.round(rect.x + rect.width/2), y: Math.round(rect.y + rect.height/2)});
						}
					}
				}
				return 'not_found';
			})()
		`, string(searchJSON))

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

	// Last resort: click at recorded coordinates
	if step.X > 0 && step.Y > 0 {
		return conn.DispatchMouseClick(step.X, step.Y)
	}

	return fmt.Errorf("could not find element to click")
}

// replayInput tries to find an element and set its value.
func replayInput(conn *cdp.Connection, step cdp.RecordEvent) error {
	if step.Selector != "" {
		err := conn.TypeInElement(step.Selector, step.Value)
		if err == nil {
			return nil
		}
	}

	// Fallback: try clicking the element position first, then type
	if step.X > 0 && step.Y > 0 {
		conn.DispatchMouseClick(step.X, step.Y)
		time.Sleep(100 * time.Millisecond)
	}

	// Type character by character
	for _, ch := range step.Value {
		conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyDown",
			"text": string(ch),
		})
		conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyUp",
		})
		time.Sleep(30 * time.Millisecond)
	}

	return nil
}
