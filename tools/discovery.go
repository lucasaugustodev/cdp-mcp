package tools

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
)

// RegisterDiscovery registers all discovery tools.
func RegisterDiscovery(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "list_apps",
		Description: "Scan for CDP-capable apps (browsers, Electron apps, WebView2 apps). Returns a numbered list of connectable pages.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleListApps)

	server.RegisterTool(mcp.Tool{
		Name:        "connect",
		Description: "Connect to a CDP-capable app by name (e.g. 'WhatsApp', 'Instagram') or by index from list_apps output.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "App name (e.g. 'WhatsApp') or index number (e.g. '0') from list_apps",
				},
			},
			"required": []string{"target"},
		},
	}, handleConnect)

	server.RegisterTool(mcp.Tool{
		Name:        "status",
		Description: "Return current CDP connection info (connected app, URL, port).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleStatus)
}

func handleListApps(args map[string]interface{}) mcp.ToolResult {
	activePorts := cdp.ScanCDPPorts()

	if len(activePorts) == 0 {
		return mcp.TextResult("No CDP-capable apps found. Make sure your apps are running with CDP enabled.\n\nFor Chrome: run with --remote-debugging-port=9222\nFor WebView2 apps: set WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222")
	}

	// Collect all pages
	var apps []AppEntry
	// Sort ports for deterministic output
	ports := make([]int, 0, len(activePorts))
	for p := range activePorts {
		ports = append(ports, p)
	}
	sort.Ints(ports)

	idx := 0
	for _, port := range ports {
		pages := activePorts[port]
		for _, page := range pages {
			if page.Type != "page" {
				continue
			}
			apps = append(apps, AppEntry{
				Index: idx,
				Title: page.Title,
				URL:   page.URL,
				Port:  port,
				WsURL: page.WebSocketDebuggerURL,
			})
			idx++
		}
	}

	SetLastApps(apps)

	if len(apps) == 0 {
		return mcp.TextResult("Found active CDP ports but no pages of type 'page'. Try opening a page in the browser.")
	}

	var lines []string
	for _, app := range apps {
		title := app.Title
		if title == "" {
			title = "(untitled)"
		}
		lines = append(lines, fmt.Sprintf("[%d] %s (port %d) — %s", app.Index, title, app.Port, app.URL))
	}
	lines = append(lines, fmt.Sprintf("\nTotal: %d connectable pages across %d ports", len(apps), len(activePorts)))
	lines = append(lines, "Use connect with a name or index to connect, e.g.: connect {\"target\": \"0\"}")

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleConnect(args map[string]interface{}) mcp.ToolResult {
	target, _ := args["target"].(string)
	if target == "" {
		return mcp.ErrorResult("Missing 'target' argument. Provide an app name or index from list_apps.")
	}

	// Try as index first
	if idx, err := strconv.Atoi(target); err == nil {
		apps := GetLastApps()
		if len(apps) == 0 {
			// Run a scan first
			handleListApps(nil)
			apps = GetLastApps()
		}
		if idx >= 0 && idx < len(apps) {
			app := apps[idx]
			return connectToApp(app)
		}
		return mcp.ErrorResult(fmt.Sprintf("Index %d out of range. Run list_apps first to see available apps.", idx))
	}

	// Try as name - scan ports and find matching page
	activePorts := cdp.ScanCDPPorts()
	targetLower := strings.ToLower(target)

	// Sort ports for deterministic results
	ports := make([]int, 0, len(activePorts))
	for p := range activePorts {
		ports = append(ports, p)
	}
	sort.Ints(ports)

	for _, port := range ports {
		pages := activePorts[port]
		for _, page := range pages {
			if page.Type != "page" {
				continue
			}
			titleLower := strings.ToLower(page.Title)
			urlLower := strings.ToLower(page.URL)
			if strings.Contains(titleLower, targetLower) || strings.Contains(urlLower, targetLower) {
				app := AppEntry{
					Title: page.Title,
					URL:   page.URL,
					Port:  port,
					WsURL: page.WebSocketDebuggerURL,
				}
				return connectToApp(app)
			}
		}
	}

	return mcp.ErrorResult(fmt.Sprintf("No app found matching %q. Run list_apps to see available apps.", target))
}

func connectToApp(app AppEntry) mcp.ToolResult {
	if app.WsURL == "" {
		return mcp.ErrorResult(fmt.Sprintf("App %q has no WebSocket debugger URL", app.Title))
	}

	conn, err := cdp.Dial(app.WsURL)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to connect to %q: %v", app.Title, err))
	}

	conn.PageTitle = app.Title
	conn.PageURL = app.URL

	SetConn(conn, app.Title, app.URL, app.Port)
	log.Printf("Connected to %q at port %d", app.Title, app.Port)

	return mcp.TextResult(fmt.Sprintf("Connected to %q\nURL: %s\nPort: %d", app.Title, app.URL, app.Port))
}

func handleStatus(args map[string]interface{}) mcp.ToolResult {
	title, url, port, connected := GetConnInfo()
	if !connected {
		return mcp.TextResult("Not connected to any app. Use list_apps and connect to get started.")
	}
	recording := ""
	if IsRecording() {
		recording = "\nRecording: ACTIVE"
	}
	return mcp.TextResult(fmt.Sprintf("Connected to: %s\nURL: %s\nPort: %d%s", title, url, port, recording))
}
