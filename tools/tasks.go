package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
)

// RegisterTasks registers all task management tools.
func RegisterTasks(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "task_create",
		Description: "Create a new automated task (polling, monitor, workflow, or schedule) for a connected app.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Human-readable task name",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"polling", "monitor", "workflow", "schedule"},
					"description": "Task type: polling (repeated checks), monitor (watch for changes), workflow (trigger+actions), schedule (cron-based)",
				},
				"appId": map[string]interface{}{
					"type":        "string",
					"description": "ID of the app this task belongs to",
				},
				"config": map[string]interface{}{
					"type":        "object",
					"description": "Type-specific configuration. Polling: {interval, action}. Monitor: {selector, textMatch, action}. Workflow: {trigger, actions}. Schedule: {cron, days, action}.",
				},
			},
			"required": []string{"name", "type", "appId", "config"},
		},
	}, handleTaskCreate)

	server.RegisterTool(mcp.Tool{
		Name:        "task_list",
		Description: "List all tasks, optionally filtered by app ID. Shows status, type, and last/next run times.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"appId": map[string]interface{}{
					"type":        "string",
					"description": "Optional: filter tasks by app ID",
				},
			},
		},
	}, handleTaskList)

	server.RegisterTool(mcp.Tool{
		Name:        "task_pause",
		Description: "Pause a running task by its ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"taskId": map[string]interface{}{
					"type":        "string",
					"description": "ID of the task to pause",
				},
			},
			"required": []string{"taskId"},
		},
	}, handleTaskPause)

	server.RegisterTool(mcp.Tool{
		Name:        "task_resume",
		Description: "Resume a paused task by its ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"taskId": map[string]interface{}{
					"type":        "string",
					"description": "ID of the task to resume",
				},
			},
			"required": []string{"taskId"},
		},
	}, handleTaskResume)

	server.RegisterTool(mcp.Tool{
		Name:        "task_delete",
		Description: "Permanently delete a task by its ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"taskId": map[string]interface{}{
					"type":        "string",
					"description": "ID of the task to delete",
				},
			},
			"required": []string{"taskId"},
		},
	}, handleTaskDelete)
}

func handleTaskCreate(args map[string]interface{}) mcp.ToolResult {
	name, _ := args["name"].(string)
	taskType, _ := args["type"].(string)
	appID, _ := args["appId"].(string)
	configRaw := args["config"]

	if name == "" {
		return mcp.ErrorResult("Missing required argument: name")
	}
	if taskType == "" {
		return mcp.ErrorResult("Missing required argument: type")
	}
	if appID == "" {
		return mcp.ErrorResult("Missing required argument: appId")
	}

	// Validate task type
	validTypes := map[string]bool{"polling": true, "monitor": true, "workflow": true, "schedule": true}
	if !validTypes[taskType] {
		return mcp.ErrorResult(fmt.Sprintf("Invalid task type %q. Must be one of: polling, monitor, workflow, schedule", taskType))
	}

	// Marshal config to json.RawMessage
	configBytes, err := json.Marshal(configRaw)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Invalid config: %v", err))
	}

	taskID := config.GenerateTaskID()
	task := config.Task{
		ID:      taskID,
		Type:    taskType,
		AppID:   appID,
		Name:    name,
		Enabled: true,
		Config:  json.RawMessage(configBytes),
	}

	if err := config.AddTask(task); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to create task: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Task created successfully.\nID: %s\nName: %s\nType: %s\nApp: %s\nStatus: enabled", taskID, name, taskType, appID))
}

func handleTaskList(args map[string]interface{}) mcp.ToolResult {
	appID, _ := args["appId"].(string)

	var tasks []config.Task
	if appID != "" {
		tasks = config.ListTasksByApp(appID)
	} else {
		tasks = config.LoadTasks()
	}

	if len(tasks) == 0 {
		if appID != "" {
			return mcp.TextResult(fmt.Sprintf("No tasks found for app %q.", appID))
		}
		return mcp.TextResult("No tasks found. Use task_create to create one.")
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Tasks (%d):", len(tasks)))
	lines = append(lines, "")

	for _, t := range tasks {
		status := "enabled"
		if !t.Enabled {
			status = "paused"
		}
		lastRun := t.LastRun
		if lastRun == "" {
			lastRun = "never"
		}
		nextRun := t.NextRun
		if nextRun == "" {
			nextRun = "-"
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s", t.ID, t.Name))
		lines = append(lines, fmt.Sprintf("    Type: %s | Status: %s | App: %s", t.Type, status, t.AppID))
		lines = append(lines, fmt.Sprintf("    Last run: %s | Next run: %s", lastRun, nextRun))
		lines = append(lines, "")
	}

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleTaskPause(args map[string]interface{}) mcp.ToolResult {
	taskID, _ := args["taskId"].(string)
	if taskID == "" {
		return mcp.ErrorResult("Missing required argument: taskId")
	}

	task := config.GetTask(taskID)
	if task == nil {
		return mcp.ErrorResult(fmt.Sprintf("Task %q not found.", taskID))
	}

	if !task.Enabled {
		return mcp.TextResult(fmt.Sprintf("Task %q (%s) is already paused.", taskID, task.Name))
	}

	task.Enabled = false
	if err := config.UpdateTask(*task); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to pause task: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Task paused.\nID: %s\nName: %s", taskID, task.Name))
}

func handleTaskResume(args map[string]interface{}) mcp.ToolResult {
	taskID, _ := args["taskId"].(string)
	if taskID == "" {
		return mcp.ErrorResult("Missing required argument: taskId")
	}

	task := config.GetTask(taskID)
	if task == nil {
		return mcp.ErrorResult(fmt.Sprintf("Task %q not found.", taskID))
	}

	if task.Enabled {
		return mcp.TextResult(fmt.Sprintf("Task %q (%s) is already running.", taskID, task.Name))
	}

	task.Enabled = true
	if err := config.UpdateTask(*task); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to resume task: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Task resumed.\nID: %s\nName: %s", taskID, task.Name))
}

func handleTaskDelete(args map[string]interface{}) mcp.ToolResult {
	taskID, _ := args["taskId"].(string)
	if taskID == "" {
		return mcp.ErrorResult("Missing required argument: taskId")
	}

	task := config.GetTask(taskID)
	if task == nil {
		return mcp.ErrorResult(fmt.Sprintf("Task %q not found.", taskID))
	}

	name := task.Name
	if err := config.RemoveTask(taskID); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to delete task: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Task deleted.\nID: %s\nName: %s", taskID, name))
}
