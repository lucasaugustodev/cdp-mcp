package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

// ExecuteWorkflow parses a workflow-type task and executes its actions sequentially
// with a 500ms delay between each action. Called by monitor.go when a workflow trigger fires.
func ExecuteWorkflow(task config.Task) {
	var wc config.WorkflowConfig
	if err := json.Unmarshal(task.Config, &wc); err != nil {
		log.Printf("[workflow] Failed to parse workflow config for task %s: %v", task.ID, err)
		return
	}

	if len(wc.Actions) == 0 {
		log.Printf("[workflow] No actions defined for task %s", task.ID)
		return
	}

	log.Printf("[workflow] Starting workflow %s (%s) with %d actions", task.ID, task.Name, len(wc.Actions))

	executed := 0
	for i, action := range wc.Actions {
		log.Printf("[workflow] Task %s: executing action %d/%d (type=%s)", task.ID, i+1, len(wc.Actions), action.Type)
		executeAction(task.AppID, action)
		executed++

		// Delay between actions (skip after the last one)
		if i < len(wc.Actions)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	result := fmt.Sprintf("workflow complete: %d/%d actions executed", executed, len(wc.Actions))
	log.Printf("[workflow] Task %s: %s", task.ID, result)
	tools.LogActivity("engine:workflow", task.ID, result)
}
