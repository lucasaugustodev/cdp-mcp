package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/notify"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

// activeMonitors tracks which task IDs have an active observer injected.
var (
	monitorMu       sync.Mutex
	activeMonitors  = make(map[string]bool)
	monitorSubReady = make(map[string]bool) // tracks per-connection console subscriptions
)

// monitorEvent is the JSON payload emitted by the injected MutationObserver.
type monitorEvent struct {
	TaskID    string `json:"taskId"`
	Text      string `json:"text"`
	Selector  string `json:"selector"`
	Timestamp int64  `json:"timestamp"`
}

// StartMonitors iterates all enabled monitor/workflow tasks and injects
// DOM MutationObservers via CDP. Called from engine.Start().
func StartMonitors() {
	tasks := config.LoadTasks()

	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		if task.Type != "monitor" && task.Type != "workflow" {
			continue
		}

		appState := tools.GetAppState(task.AppID)
		if appState == nil || appState.Conn == nil || appState.Conn.IsClosed() {
			log.Printf("[monitor] No active connection for app %q, skipping task %s", task.AppID, task.ID)
			continue
		}

		injectObserver(appState.Conn, task)
		subscribeConsoleForMonitor(appState.Conn, task.AppID)
	}
}

// injectObserver injects a JavaScript MutationObserver into the page via CDP.
// The observer watches for added DOM nodes matching the task's selector/textMatch
// and emits events via console.log with the __CDPMONITOR__ prefix.
func injectObserver(conn *cdp.Connection, task config.Task) {
	var selector, textMatch string

	switch task.Type {
	case "monitor":
		var mc config.MonitorConfig
		if err := json.Unmarshal(task.Config, &mc); err != nil {
			log.Printf("[monitor] Failed to parse monitor config for task %s: %v", task.ID, err)
			return
		}
		selector = mc.Selector
		textMatch = mc.TextMatch
	case "workflow":
		var wc config.WorkflowConfig
		if err := json.Unmarshal(task.Config, &wc); err != nil {
			log.Printf("[monitor] Failed to parse workflow config for task %s: %v", task.ID, err)
			return
		}
		selector = wc.Trigger.Selector
		textMatch = wc.Trigger.TextMatch
	default:
		return
	}

	// Sanitize values for JS injection (escape backslashes and single quotes)
	taskID := escapeJS(task.ID)
	selector = escapeJS(selector)
	textMatch = escapeJS(textMatch)

	js := fmt.Sprintf(`
		(() => {
			if (window.__cdpMonitor_%s) return 'already_active';
			window.__cdpMonitor_%s = true;

			const observer = new MutationObserver((mutations) => {
				for (const m of mutations) {
					for (const node of m.addedNodes) {
						if (node.nodeType !== 1) continue;
						const text = (node.textContent || '').toLowerCase();
						const match = '%s'.toLowerCase();
						const terms = match.split('|');
						if (terms.some(t => text.includes(t.trim()))) {
							console.log('__CDPMONITOR__' + JSON.stringify({
								taskId: '%s',
								text: node.textContent.substring(0, 200),
								selector: '%s',
								timestamp: Date.now()
							}));
						}
					}
				}
			});

			const target = document.querySelector('%s') || document.body;
			observer.observe(target, { childList: true, subtree: true });
			return 'observer_started';
		})()
	`, taskID, taskID, textMatch, taskID, selector, selector)

	// Enable Runtime domain so console events are emitted
	_, _ = conn.Send("Runtime.enable", map[string]interface{}{})

	result, err := conn.EvaluateString(js)
	if err != nil {
		log.Printf("[monitor] Failed to inject observer for task %s: %v", task.ID, err)
		return
	}

	monitorMu.Lock()
	activeMonitors[task.ID] = true
	monitorMu.Unlock()

	log.Printf("[monitor] Observer injected for task %s: %s", task.ID, result)
}

// subscribeConsoleForMonitor subscribes to Runtime.consoleAPICalled on the
// connection to catch __CDPMONITOR__ events. Only subscribes once per appID.
func subscribeConsoleForMonitor(conn *cdp.Connection, appID string) {
	monitorMu.Lock()
	if monitorSubReady[appID] {
		monitorMu.Unlock()
		return
	}
	monitorSubReady[appID] = true
	monitorMu.Unlock()

	conn.Subscribe("Runtime.consoleAPICalled", func(params json.RawMessage) {
		var data struct {
			Type string `json:"type"`
			Args []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"args"`
		}
		if json.Unmarshal(params, &data) != nil {
			return
		}
		if data.Type != "log" || len(data.Args) == 0 {
			return
		}

		val := data.Args[0].Value
		const prefix = "__CDPMONITOR__"
		if len(val) <= len(prefix) || val[:len(prefix)] != prefix {
			return
		}

		jsonStr := val[len(prefix):]
		var evt monitorEvent
		if json.Unmarshal([]byte(jsonStr), &evt) != nil {
			log.Printf("[monitor] Failed to parse monitor event JSON: %s", jsonStr)
			return
		}

		handleMonitorEvent(conn, evt.TaskID, jsonStr)
	})
}

// handleMonitorEvent is called when a monitor triggers. It parses the event,
// loads the task config, executes the configured action(s), and logs activity.
func handleMonitorEvent(conn *cdp.Connection, taskID string, eventData string) {
	var evt monitorEvent
	if err := json.Unmarshal([]byte(eventData), &evt); err != nil {
		log.Printf("[monitor] Failed to parse event data for task %s: %v", taskID, err)
		return
	}

	task := config.GetTask(taskID)
	if task == nil {
		log.Printf("[monitor] Task %s not found, ignoring event", taskID)
		return
	}

	log.Printf("[monitor] Event triggered for task %s (%s): %s", taskID, task.Name, truncate(evt.Text, 80))

	switch task.Type {
	case "monitor":
		var mc config.MonitorConfig
		if err := json.Unmarshal(task.Config, &mc); err != nil {
			log.Printf("[monitor] Failed to parse config for task %s: %v", taskID, err)
			return
		}
		executeMonitorAction(task.AppID, mc.Action, evt)

	case "workflow":
		var wc config.WorkflowConfig
		if err := json.Unmarshal(task.Config, &wc); err != nil {
			log.Printf("[monitor] Failed to parse workflow config for task %s: %v", taskID, err)
			return
		}
		for _, action := range wc.Actions {
			executeMonitorAction(task.AppID, action, evt)
		}
	}

	tools.LogActivity("monitor:event", fmt.Sprintf("task=%s text=%s", taskID, truncate(evt.Text, 100)), "triggered")
}

// executeMonitorAction dispatches a single action triggered by a monitor event.
func executeMonitorAction(appID string, action config.Action, evt monitorEvent) {
	switch action.Type {
	case "notify":
		msg := action.Text
		if msg == "" {
			msg = fmt.Sprintf("DOM change detected: %s", truncate(evt.Text, 100))
		}
		notify.Send("CDP Monitor", msg)
		tools.LogActivity("monitor:notify", msg, "sent")

	case "recipe":
		executeAction(appID, action)

	case "prompt":
		log.Printf("[monitor] Prompt action for task %s: %s", evt.TaskID, action.Text)
		tools.LogActivity("monitor:prompt", action.Text, "logged (not executed)")

	default:
		log.Printf("[monitor] Unknown action type %q for task %s", action.Type, evt.TaskID)
	}
}

// StopMonitor injects JS to disable the observer for a given task ID.
func StopMonitor(taskID string) {
	monitorMu.Lock()
	wasActive := activeMonitors[taskID]
	delete(activeMonitors, taskID)
	monitorMu.Unlock()

	if !wasActive {
		log.Printf("[monitor] Task %s was not actively monitored", taskID)
		return
	}

	task := config.GetTask(taskID)
	if task == nil {
		return
	}

	appState := tools.GetAppState(task.AppID)
	if appState == nil || appState.Conn == nil || appState.Conn.IsClosed() {
		return
	}

	js := fmt.Sprintf(`
		(() => {
			window.__cdpMonitor_%s = false;
			return 'observer_stopped';
		})()
	`, escapeJS(taskID))

	result, err := appState.Conn.EvaluateString(js)
	if err != nil {
		log.Printf("[monitor] Failed to stop observer for task %s: %v", taskID, err)
		return
	}

	log.Printf("[monitor] Observer stopped for task %s: %s", taskID, result)
	tools.LogActivity("monitor:stop", taskID, "stopped")
}

// IsMonitorActive returns whether a monitor is currently active for a task.
func IsMonitorActive(taskID string) bool {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	return activeMonitors[taskID]
}

// escapeJS escapes a string for safe inclusion in a JS single-quoted string literal.
func escapeJS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
