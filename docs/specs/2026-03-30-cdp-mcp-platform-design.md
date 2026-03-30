# CDP MCP Platform — Design Spec

## Goal

Transform cdp-mcp from a tool collection into a full platform for AI agents to control, monitor, and automate any Windows app. Users manage apps via a web dashboard, teach agents via recording, schedule recurring tasks, and invoke per-app agents from Claude Code.

## Architecture

```
┌────────────────────┐     stdio      ┌─────────────────┐     CDP/WS      ┌────────────┐
│  Claude Code       │ ◄────────────► │   cdp-mcp.exe   │ ◄────────────► │ Chrome/Edge │
│  @whatsapp agent   │   MCP JSON-RPC │                 │                 │ WebView2   │
│  @linkedin agent   │                │  ┌───────────┐  │                 │ Electron   │
└────────────────────┘                │  │ Scheduler │  │                 └────────────┘
                                      │  │ Monitor   │  │
┌────────────────────┐     HTTP/WS    │  │ Recipes   │  │     Win32 API   ┌────────────┐
│  Dashboard         │ ◄────────────► │  │ Agents    │  │ ◄────────────► │ Native apps│
│  localhost:9400    │                │  └───────────┘  │   UIA fallback  └────────────┘
└────────────────────┘                └─────────────────┘
                                             │
                                      ~/.cdp-mcp/
                                      ├── apps.json        (configured apps)
                                      ├── recipes/         (recorded flows)
                                      ├── tasks.json       (scheduled/recurring tasks)
                                      └── agents/          (generated agent configs)
```

## Components

### 1. Dashboard (Web UI — localhost:9400)

**Layout:** Tabs + Tasks Panel (Option C from brainstorming)

- **Tab bar (top):** One tab per configured app. Green dot = connected, gray = disconnected. "+" button opens Add App modal.
- **Main area (center):** Live screenshot of selected app via CDP `Page.captureScreenshot`. Clickable (sends `Input.dispatchMouseEvent`). Keyboard input forwarded via `Input.dispatchKeyEvent`. Resizes with browser window.
- **Activity log (bottom of main):** Shows tool calls from agents + task executions + user teach actions. Per-app filtered. Monospace, timestamped.
- **Tasks panel (right):** Lists all tasks for selected app: polling, monitoring, workflows, schedules. Each shows status (active/paused/scheduled), last run, next run. "+ Add" button. Recipes section at bottom with quick-replay buttons.
- **Toolbar (top right):** "Teach" button (starts recording), "Recipes" count badge, settings gear.

**Contrast requirement:** All text must be clearly readable. Minimum: #ccc on #0a0a0a background. Badges use colored backgrounds with matching light text.

### 2. Add App Flow

**Modal with two panels:**

**Left: Installed Apps**
- Auto-detect via CDP port scan + Windows process enumeration
- Shows: app name, CDP support badge (CDP/CDP?/Native), connection status
- Already-added apps show "✓ Added"
- Click "+ Add" to configure and add

**Right: Web App (custom)**
- Fields: URL, name, email/username (optional), password (optional)
- Checkboxes: Headless, Auto-start on launch
- "Create App + Agent" button

**On add:**
1. If app needs CDP and doesn't have it → offer force_restart
2. Connect via CDP
3. Save to `~/.cdp-mcp/apps.json`
4. Generate agent config at `~/.claude/agents/{appname}.md`
5. App appears as new tab in dashboard

### 3. App Configuration (apps.json)

```json
{
  "apps": [
    {
      "id": "whatsapp",
      "name": "WhatsApp",
      "type": "webview2",
      "launchCmd": "explorer.exe shell:AppsFolder\\5319275A.WhatsAppDesktop_cv1g1gvanyjgm!App",
      "cdpPort": 9344,
      "headless": true,
      "autoStart": true,
      "recipes": ["send-message", "check-inbox"],
      "tasks": ["check-msgs-30min", "auto-reply-lucas"]
    },
    {
      "id": "myapp",
      "name": "My CRM",
      "type": "webapp",
      "url": "https://app.mycrm.com",
      "credentials": {"email": "user@x.com", "password": "encrypted..."},
      "headless": true,
      "autoStart": false,
      "recipes": [],
      "tasks": []
    }
  ]
}
```

### 4. Task System

Four task types, all stored in `~/.cdp-mcp/tasks.json`:

**4.1 Polling (⏰)**
```json
{
  "id": "check-msgs-30min",
  "type": "polling",
  "appId": "whatsapp",
  "interval": "30m",
  "action": {"type": "recipe", "recipeId": "check-inbox"},
  "enabled": true,
  "lastRun": "2026-03-30T14:00:00Z",
  "nextRun": "2026-03-30T14:30:00Z"
}
```
Implementation: Go ticker goroutine. On tick: connect to app if not connected, execute action (recipe or prompt via agent), log result.

**4.2 Monitor (👁)**
```json
{
  "id": "payment-notify",
  "type": "monitor",
  "appId": "whatsapp",
  "condition": {
    "type": "dom_observer",
    "selector": "[data-testid='msg-container']",
    "textMatch": "pagamento|pix|transferência"
  },
  "action": {"type": "notify+recipe", "notification": "Payment received!", "recipeId": "reply-thanks"},
  "enabled": true
}
```
Implementation: Inject MutationObserver via CDP `Runtime.evaluate`. Observer watches for DOM changes matching selector + text pattern. On match: sends event via `console.log` → CDP `Runtime.consoleAPICalled` → Go handler → execute action.

**4.3 Workflow (⚡)**
```json
{
  "id": "auto-reply-lucas",
  "type": "workflow",
  "appId": "whatsapp",
  "trigger": {
    "type": "dom_observer",
    "selector": "[data-testid='msg-container']",
    "textMatch": "Lucas"
  },
  "actions": [
    {"type": "recipe", "recipeId": "open-chat-lucas"},
    {"type": "prompt", "text": "Reply saying I'll check later"},
    {"type": "notify", "text": "Auto-replied to Lucas"}
  ],
  "enabled": true
}
```
Implementation: Same DOM observer as Monitor, but on trigger executes a chain of actions sequentially.

**4.4 Schedule (📅)**
```json
{
  "id": "morning-linkedin",
  "type": "schedule",
  "appId": "linkedin",
  "cron": "0 9 * * 1-5",
  "action": {"type": "prompt", "text": "Like the first 3 posts on my feed"},
  "enabled": true,
  "lastRun": "2026-03-30T09:00:00Z"
}
```
Implementation: Cron-like scheduler goroutine. Evaluates cron expressions every minute. On match: connect to app, execute action.

### 5. Agent Generation

When an app is added, generate `~/.claude/agents/{appname}.md`:

```markdown
# {AppName} Agent

You control {AppName} via CDP MCP tools (cdp-tools).

## Connection
- Connect with: connect {"target": "{AppName}"}
- CDP port: {port}
- Mode: {headless/visible}

## Available Recipes
{list of recipes with descriptions}

## Active Tasks
{list of active tasks}

## Instructions
- Always connect first before any action
- Use `find` with `click=true` for fast interactions
- Use `js` for precise DOM manipulation
- Verify actions with `screenshot` when unsure
- Respond in the user's language
```

The agent file is auto-updated when recipes or tasks change.

### 6. Notification System

**Dashboard notifications:**
- Tab badge with count (red dot)
- Activity log entry highlighted in yellow
- Sound (optional, configurable)

**Windows toast notifications:**
- Use PowerShell `New-BurntToastNotification` or `[Windows.UI.Notifications]` API
- Title: app name, Body: notification text
- Click action: open dashboard to that app's tab

**Auto-action:**
- Notification can trigger recipe/prompt execution
- Result logged in activity
- Chained: notify → execute → verify → notify result

### 7. Persistence

All state in `~/.cdp-mcp/`:
```
~/.cdp-mcp/
├── apps.json           # configured apps
├── tasks.json          # all tasks (polling, monitor, workflow, schedule)
├── recipes/            # recorded flows as JSON
│   ├── send-message.json
│   └── check-inbox.json
├── agents/             # generated agent .md files (copied to ~/.claude/agents/)
├── activity.log        # activity log (last 1000 entries)
└── cdp-ports.json      # port registry
```

### 8. MCP Tools (additions to existing 22)

New tools for task management from Claude Code:

| Tool | Description |
|------|-------------|
| `task_create` | Create a new task (polling/monitor/workflow/schedule) |
| `task_list` | List all tasks, optionally filtered by app |
| `task_pause` | Pause a task |
| `task_resume` | Resume a paused task |
| `task_delete` | Delete a task |
| `app_list` | List configured apps with status |
| `app_add` | Add an app programmatically |

Total: 29 tools.

### 9. Multi-Connection Support

Current state.go holds one connection. Change to map:

```go
type AppState struct {
    Conn     *cdp.Connection
    Title    string
    URL      string
    Port     int
    AppID    string
    Tasks    []*Task
    Recording bool
}

connections map[string]*AppState // appId → state
activeApp   string               // currently selected in dashboard
```

All existing tools use `activeApp` by default. Tools can accept optional `app` parameter to target specific app.

## Out of Scope (v1)

- Cross-platform (Linux/macOS) — Windows only for now
- Mobile device control
- Multi-user / auth on dashboard
- Cloud deployment of dashboard
- Encrypted credential storage (plaintext in v1, encrypt later)

## Success Criteria

1. User adds WhatsApp via dashboard → agent file created → `@whatsapp` works in Claude Code
2. User teaches "send message" via Teach → recipe saved → replayable
3. Polling task checks WhatsApp every 30min → logs results
4. DOM monitor detects new message → Windows toast notification → auto-replies
5. Schedule runs recipe at 9am → likes LinkedIn posts
6. Dashboard shows all activity in real-time
7. Everything persists across restarts
