# CDP MCP Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform cdp-mcp from a tool collection into a full platform with multi-app management, task scheduling, monitoring, recording, agent generation, and a rich web dashboard.

**Architecture:** Multi-connection state replaces single connection. Apps/tasks/recipes persisted to ~/.cdp-mcp/. Dashboard redesigned with tabs+tasks panel. Task engine runs polling/monitoring/workflows/schedules. Per-app agent files generated for Claude Code.

**Tech Stack:** Go, gorilla/websocket, CDP, embedded HTML/CSS/JS dashboard, MCP JSON-RPC stdio

**Spec:** `docs/specs/2026-03-30-cdp-mcp-platform-design.md`

---

## Sub-Plans (execute in order)

### Sub-Plan 1: Multi-Connection + Persistence (Foundation)
### Sub-Plan 2: Dashboard Redesign (Tabs + Tasks Panel)
### Sub-Plan 3: Task System (Polling, Monitor, Workflow, Schedule)
### Sub-Plan 4: Agent Generation
### Sub-Plan 5: Notifications

---

## Sub-Plan 1: Multi-Connection + Persistence

### Task 1: App Config Persistence

**Files:**
- Create: `config/apps.go`
- Create: `config/config.go`

- [ ] **Step 1: Create config package with AppConfig struct**

```go
// config/config.go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

func DataDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cdp-mcp")
	os.MkdirAll(dir, 0755)
	return dir
}
```

```go
// config/apps.go
package config

type AppConfig struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`       // "webview2", "pwa", "electron", "webapp", "native"
	LaunchCmd  string            `json:"launchCmd,omitempty"`
	URL        string            `json:"url,omitempty"`
	CDPPort    int               `json:"cdpPort,omitempty"`
	Headless   bool              `json:"headless"`
	AutoStart  bool              `json:"autoStart"`
	Credentials map[string]string `json:"credentials,omitempty"`
	Recipes    []string          `json:"recipes,omitempty"`
	Tasks      []string          `json:"tasks,omitempty"`
}

type AppsFile struct {
	Apps []AppConfig `json:"apps"`
}

var (
	appsMu sync.RWMutex
	apps   *AppsFile
)

func LoadApps() *AppsFile
func SaveApps(a *AppsFile) error
func GetApp(id string) *AppConfig
func AddApp(app AppConfig) error
func RemoveApp(id string) error
func ListApps() []AppConfig
```

- [ ] **Step 2: Implement Load/Save/CRUD operations**

Full implementations reading/writing `~/.cdp-mcp/apps.json`.

- [ ] **Step 3: Build and verify**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add config/
git commit -m "feat: app config persistence with CRUD"
```

### Task 2: Multi-Connection State

**Files:**
- Rewrite: `tools/state.go`

- [ ] **Step 1: Replace single connection with connection map**

Replace the single `conn` field with:
```go
type AppState struct {
	Conn      *cdp.Connection
	Config    config.AppConfig
	Recording bool
	Events    []cdp.RecordEvent
}

type State struct {
	mu          sync.RWMutex
	apps        map[string]*AppState // appId → state
	activeAppID string               // currently selected
	lastScan    []AppEntry
	activityLog []ActivityEntry
}
```

- [ ] **Step 2: Update all accessor functions**

- `GetConn()` → returns active app's connection
- `SetConn()` → takes appId, stores in map
- `GetConnInfo()` → returns active app info
- `GetAppState(appId)` → get specific app
- `SetActiveApp(appId)` → switch active app
- `ListConnectedApps()` → all apps with live connections

- [ ] **Step 3: Update all tools that call GetConn()**

Every tool in interaction.go, recording.go, advanced.go, discovery.go that calls `GetConn()` continues to work because `GetConn()` returns the active app's connection.

Add optional `app` parameter handling: if args contain `"app": "whatsapp"`, use that app instead of active.

- [ ] **Step 4: Build and verify**

```bash
go build -o cdp-mcp.exe .
```

- [ ] **Step 5: Commit**

```bash
git add tools/state.go tools/*.go
git commit -m "feat: multi-connection state with app map"
```

### Task 3: App Management MCP Tools

**Files:**
- Create: `tools/apps.go`

- [ ] **Step 1: Implement app_list and app_add tools**

```go
func RegisterApps(server *mcp.Server)
func handleAppList(args map[string]interface{}) mcp.ToolResult
func handleAppAdd(args map[string]interface{}) mcp.ToolResult
```

`app_add` accepts: name, type, url (for webapp), headless, autoStart. Auto-detects launchCmd for known apps. Connects via CDP. Saves to apps.json.

- [ ] **Step 2: Register in main.go**

```go
tools.RegisterApps(server)
```

- [ ] **Step 3: Build, test with stdin**

- [ ] **Step 4: Commit**

### Task 4: Auto-load apps on startup

**Files:**
- Modify: `main.go`

- [ ] **Step 1: On startup, load apps.json and auto-connect enabled apps**

```go
func autoLoadApps() {
	apps := config.ListApps()
	for _, app := range apps {
		if app.AutoStart {
			// try to connect via CDP
		}
	}
}
```

- [ ] **Step 2: Commit**

---

## Sub-Plan 2: Dashboard Redesign

### Task 5: Dashboard HTML — Tabs Layout

**Files:**
- Rewrite: `dashboard/static.go`

- [ ] **Step 1: Complete rewrite of dashboard HTML**

New layout with:
- Tab bar: one tab per app from apps.json, "+" button
- Main area: large screenshot (resizable with window), clickable, keyboard input
- Activity log: below screenshot, per-app filtered, monospace
- Tasks panel: right sidebar, task list + recipes
- Toolbar: Teach, Recipes, Settings buttons
- Add App modal: installed apps list + web app form

Dark theme with proper contrast (min #ccc on #0a0a0a).

- [ ] **Step 2: Commit**

### Task 6: Dashboard API — Multi-App

**Files:**
- Rewrite: `dashboard/handler.go`

- [ ] **Step 1: Update API endpoints for multi-app**

- `GET /api/apps` — list configured apps with connection status
- `GET /api/apps/:id/screenshot` — screenshot of specific app
- `POST /api/apps/:id/click` — click on specific app
- `POST /api/apps/:id/type` — type on specific app
- `POST /api/apps/:id/select` — set as active app
- `POST /api/apps/add` — add new app (calls config + connect)
- `DELETE /api/apps/:id` — remove app
- `GET /api/apps/:id/activity` — activity log for app
- `GET /api/apps/:id/tasks` — tasks for app
- `GET /api/apps/:id/recipes` — recipes for app
- `WS /ws` — WebSocket for live updates (screenshots + activity)

- [ ] **Step 2: Commit**

### Task 7: Dashboard — Teach (Recording) Integration

**Files:**
- Modify: `dashboard/handler.go`
- Modify: `dashboard/static.go`

- [ ] **Step 1: Add record start/stop endpoints**

- `POST /api/apps/:id/teach/start` — start CDP recording on app
- `POST /api/apps/:id/teach/stop` — stop recording, save recipe, return recipe JSON

- [ ] **Step 2: Update HTML with Teach button and recording indicator**

Red border on screenshot during recording. Activity log shows recorded actions in real-time.

- [ ] **Step 3: Commit**

---

## Sub-Plan 3: Task System

### Task 8: Task Persistence

**Files:**
- Create: `config/tasks.go`

- [ ] **Step 1: Task structs and CRUD**

```go
type Task struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"` // "polling", "monitor", "workflow", "schedule"
	AppID     string          `json:"appId"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"` // type-specific config
	LastRun   time.Time       `json:"lastRun,omitempty"`
	NextRun   time.Time       `json:"nextRun,omitempty"`
}

type PollingConfig struct {
	Interval string `json:"interval"` // "30m", "1h", etc
	Action   Action `json:"action"`
}

type MonitorConfig struct {
	Selector  string `json:"selector"`
	TextMatch string `json:"textMatch"`
	Action    Action `json:"action"`
}

type WorkflowConfig struct {
	Trigger MonitorConfig `json:"trigger"`
	Actions []Action      `json:"actions"`
}

type ScheduleConfig struct {
	Cron   string `json:"cron"`
	Action Action `json:"action"`
}

type Action struct {
	Type     string `json:"type"` // "recipe", "prompt", "notify"
	RecipeID string `json:"recipeId,omitempty"`
	Text     string `json:"text,omitempty"`
}
```

- [ ] **Step 2: Load/Save/CRUD for tasks.json**
- [ ] **Step 3: Commit**

### Task 9: Task Engine — Polling + Schedule

**Files:**
- Create: `engine/scheduler.go`

- [ ] **Step 1: Implement polling engine**

Goroutine that checks all enabled polling tasks every 10s. When interval elapsed: connect to app, execute action (recipe or prompt via CDP), update lastRun/nextRun.

- [ ] **Step 2: Implement schedule engine**

Cron evaluator that checks every minute. On match: execute action.

- [ ] **Step 3: Start engine in main.go**

```go
go engine.Start()
```

- [ ] **Step 4: Commit**

### Task 10: Task Engine — DOM Monitor

**Files:**
- Create: `engine/monitor.go`

- [ ] **Step 1: Implement DOM observer injection**

For each enabled monitor task: inject MutationObserver via CDP that watches for selector + textMatch. On match: console.log event → Go handler via Runtime.consoleAPICalled.

- [ ] **Step 2: Implement action execution on trigger**

When DOM observer fires: execute action chain (notify, recipe, prompt).

- [ ] **Step 3: Commit**

### Task 11: Task Engine — Workflow

**Files:**
- Create: `engine/workflow.go`

- [ ] **Step 1: Implement workflow = monitor trigger + action chain**

Same as monitor but executes multiple actions sequentially.

- [ ] **Step 2: Commit**

### Task 12: Task MCP Tools

**Files:**
- Create: `tools/tasks.go`

- [ ] **Step 1: Implement task_create, task_list, task_pause, task_resume, task_delete**

- [ ] **Step 2: Register in main.go**
- [ ] **Step 3: Commit**

### Task 13: Dashboard — Tasks Panel

**Files:**
- Modify: `dashboard/static.go`
- Modify: `dashboard/handler.go`

- [ ] **Step 1: Add task list UI in right panel**

Shows all tasks for selected app with status badges, last/next run times.

- [ ] **Step 2: Add task CRUD endpoints**

- `POST /api/apps/:id/tasks` — create task
- `PUT /api/tasks/:taskId` — update task
- `DELETE /api/tasks/:taskId` — delete task
- `POST /api/tasks/:taskId/toggle` — enable/disable

- [ ] **Step 3: Add "Add Task" modal in dashboard**
- [ ] **Step 4: Commit**

---

## Sub-Plan 4: Agent Generation

### Task 14: Agent File Generator

**Files:**
- Create: `agents/generator.go`

- [ ] **Step 1: Implement agent markdown generator**

```go
func GenerateAgentFile(app config.AppConfig, recipes []string, tasks []config.Task) error
```

Writes to `~/.claude/agents/{appid}.md` with:
- App connection info
- Available recipes list
- Active tasks list
- Instructions for using cdp-tools

- [ ] **Step 2: Call generator on app_add and on recipe/task changes**
- [ ] **Step 3: Commit**

### Task 15: Dashboard — Agent Status

**Files:**
- Modify: `dashboard/static.go`

- [ ] **Step 1: Show agent file path in app settings**

In the settings gear for each app, show the generated agent file path and a "Regenerate" button.

- [ ] **Step 2: Commit**

---

## Sub-Plan 5: Notifications

### Task 16: Windows Toast Notifications

**Files:**
- Create: `notify/toast.go`

- [ ] **Step 1: Implement Windows toast via PowerShell**

```go
func SendToast(title, message string) error {
	script := fmt.Sprintf(`
		[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
		$template = [Windows.UI.Notifications.ToastNotification]::new(...)
	`)
	// or use BurntToast if available
}
```

- [ ] **Step 2: Integrate with task engine — on trigger, send toast**
- [ ] **Step 3: Commit**

### Task 17: Dashboard Notification Badges

**Files:**
- Modify: `dashboard/static.go`

- [ ] **Step 1: Tab badges with notification count**

When a task triggers for an app, increment its tab badge. Activity log highlights in yellow.

- [ ] **Step 2: Commit**

---

## Summary

| Sub-Plan | Tasks | Files Created/Modified |
|----------|-------|----------------------|
| 1. Multi-Connection | 1-4 | config/apps.go, config/config.go, tools/state.go, tools/apps.go, main.go |
| 2. Dashboard | 5-7 | dashboard/static.go, dashboard/handler.go |
| 3. Task System | 8-13 | config/tasks.go, engine/scheduler.go, engine/monitor.go, engine/workflow.go, tools/tasks.go |
| 4. Agent Gen | 14-15 | agents/generator.go |
| 5. Notifications | 16-17 | notify/toast.go |

Total: 17 tasks, ~12 new files, ~5 modified files
