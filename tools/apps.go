package tools

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
)

// RegisterApps registers the app_list and app_add MCP tools.
func RegisterApps(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "app_list",
		Description: "List all configured apps with their current connection status (connected/disconnected).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleAppList)

	server.RegisterTool(mcp.Tool{
		Name:        "app_add",
		Description: "Add a new app to the configuration and optionally connect to it via CDP.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Display name of the app (e.g. 'WhatsApp', 'My CRM')",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"webview2", "pwa", "webapp", "electron", "native"},
					"description": "App type: webview2, pwa, webapp, electron, or native",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "URL for webapp/pwa types (optional for others)",
				},
				"headless": map[string]interface{}{
					"type":        "boolean",
					"description": "Run in headless mode (default true)",
				},
				"auto_start": map[string]interface{}{
					"type":        "boolean",
					"description": "Auto-start on platform launch (default false)",
				},
			},
			"required": []string{"name", "type"},
		},
	}, handleAppAdd)
}

func handleAppList(args map[string]interface{}) mcp.ToolResult {
	apps := config.ListApps()

	if len(apps) == 0 {
		return mcp.TextResult("No configured apps. Use app_add to register an app.")
	}

	var lines []string
	lines = append(lines, "Configured apps:")

	for i, app := range apps {
		appState := GetAppState(app.ID)
		var statusStr string
		if appState != nil && appState.Conn != nil && !appState.Conn.IsClosed() {
			portInfo := fmt.Sprintf("CDP:%d", appState.Port)
			if app.Headless {
				portInfo += " headless"
			}
			statusStr = fmt.Sprintf("\U0001F7E2 connected %s", portInfo)
		} else {
			statusStr = "\u26AA disconnected"
		}
		lines = append(lines, fmt.Sprintf("[%d] %s (%s) — %s", i, app.Name, app.Type, statusStr))
	}

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleAppAdd(args map[string]interface{}) mcp.ToolResult {
	name, _ := args["name"].(string)
	appType, _ := args["type"].(string)
	url, _ := args["url"].(string)

	if name == "" {
		return mcp.ErrorResult("Missing required argument 'name'.")
	}
	if appType == "" {
		return mcp.ErrorResult("Missing required argument 'type'.")
	}

	validTypes := map[string]bool{"webview2": true, "pwa": true, "webapp": true, "electron": true, "native": true}
	if !validTypes[appType] {
		return mcp.ErrorResult(fmt.Sprintf("Invalid type %q. Must be one of: webview2, pwa, webapp, electron, native.", appType))
	}

	// Derive ID from name
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")

	// Parse optional booleans with defaults
	headless := true
	if h, ok := args["headless"].(bool); ok {
		headless = h
	}
	autoStart := false
	if a, ok := args["auto_start"].(bool); ok {
		autoStart = a
	}

	appCfg := config.AppConfig{
		ID:        id,
		Name:      name,
		Type:      appType,
		URL:       url,
		Headless:  headless,
		AutoStart: autoStart,
	}

	if err := config.AddApp(appCfg); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to add app: %v", err))
	}

	// Try to connect based on type
	switch appType {
	case "webview2", "pwa", "electron":
		result := tryConnectCDP(appCfg)
		if result != "" {
			return mcp.TextResult(fmt.Sprintf("Added %q (%s) to configuration.\n%s", name, appType, result))
		}
		return mcp.TextResult(fmt.Sprintf("Added %q (%s) to configuration. No active CDP connection found — use connect to attach later.", name, appType))

	case "webapp":
		if url == "" {
			return mcp.TextResult(fmt.Sprintf("Added %q (%s) to configuration. No URL provided — set one to enable auto-launch.", name, appType))
		}
		result := launchWebApp(appCfg)
		return mcp.TextResult(fmt.Sprintf("Added %q (%s) to configuration.\n%s", name, appType, result))

	default:
		return mcp.TextResult(fmt.Sprintf("Added %q (%s) to configuration.", name, appType))
	}
}

// tryConnectCDP scans existing CDP ports for a matching page and connects.
func tryConnectCDP(app config.AppConfig) string {
	activePorts := cdp.ScanCDPPorts()
	nameLower := strings.ToLower(app.Name)

	for _, port := range sortedKeys(activePorts) {
		for _, page := range activePorts[port] {
			if page.Type != "page" {
				continue
			}
			if strings.Contains(strings.ToLower(page.Title), nameLower) ||
				strings.Contains(strings.ToLower(page.URL), nameLower) {
				conn, err := cdp.Dial(page.WebSocketDebuggerURL)
				if err != nil {
					return fmt.Sprintf("Found matching page but failed to connect: %v", err)
				}
				conn.PageTitle = page.Title
				conn.PageURL = page.URL
				_ = conn.SetViewport(1440, 900)
				SetAppConn(app.ID, conn, page.Title, page.URL, port)
				return fmt.Sprintf("Connected to CDP on port %d (%s).", port, page.Title)
			}
		}
	}
	return ""
}

// launchWebApp opens a URL in Edge with CDP enabled and tries to connect.
func launchWebApp(app config.AppConfig) string {
	port := findFreePort()
	edgePath := `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`

	cmd := exec.Command(edgePath,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--new-window",
		app.URL,
	)
	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("Failed to launch Edge: %v", err)
	}

	// Wait for Edge to start and CDP to become available
	time.Sleep(4 * time.Second)

	urlLower := strings.ToLower(app.URL)
	for retry := 0; retry < 5; retry++ {
		activePorts := cdp.ScanCDPPorts()
		if pages, ok := activePorts[port]; ok {
			for _, page := range pages {
				if page.Type != "page" {
					continue
				}
				if strings.Contains(strings.ToLower(page.URL), urlLower) || page.URL != "" {
					conn, err := cdp.Dial(page.WebSocketDebuggerURL)
					if err != nil {
						continue
					}
					conn.PageTitle = page.Title
					conn.PageURL = page.URL
					_ = conn.SetViewport(1440, 900)
					SetAppConn(app.ID, conn, page.Title, page.URL, port)
					return fmt.Sprintf("Launched in Edge and connected via CDP on port %d.", port)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Sprintf("Launched Edge on port %d but could not connect yet. Try connect later.", port)
}
