package tools

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
)

// RegisterDiscovery registers all discovery tools.
func RegisterDiscovery(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "inventory",
		Description: "Full scan of everything controllable on this Windows machine. Shows: CDP-connected apps (ready), CDP-available apps (need restart), and native Windows apps. This is the first tool to call to understand what's available.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleInventory)

	server.RegisterTool(mcp.Tool{
		Name:        "list_apps",
		Description: "Quick scan for CDP-capable apps only (browsers, Electron, WebView2). Returns a numbered list of connectable pages.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleListApps)

	server.RegisterTool(mcp.Tool{
		Name:        "connect",
		Description: "Connect to an app by name (e.g. 'WhatsApp', 'Instagram') or by index from list_apps. If the app doesn't have CDP enabled, set force_restart=true to kill and relaunch it with CDP support.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "App name (e.g. 'WhatsApp') or index number (e.g. '0') from list_apps",
				},
				"force_restart": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, kill and relaunch the app with CDP enabled. Required for apps not yet CDP-enabled.",
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

// knownWebView2Apps maps app exe names to their UWP launch commands
var knownWebView2Apps = map[string]string{
	"whatsapp": "5319275A.WhatsAppDesktop_cv1g1gvanyjgm",
	"linkedin": "7EE7776C.LinkedInforWindows_w1wdnht996qgy",
}

func handleInventory(args map[string]interface{}) mcp.ToolResult {
	var lines []string

	// 1. CDP-connected apps (scan ports)
	activePorts := cdp.ScanCDPPorts()
	var cdpApps []AppEntry
	idx := 0
	ports := sortedKeys(activePorts)
	for _, port := range ports {
		pages := activePorts[port]
		for _, page := range pages {
			if page.Type != "page" {
				continue
			}
			cdpApps = append(cdpApps, AppEntry{Index: idx, Title: page.Title, URL: page.URL, Port: port, WsURL: page.WebSocketDebuggerURL})
			idx++
		}
	}
	SetLastApps(cdpApps)

	if len(cdpApps) > 0 {
		lines = append(lines, "🟢 CDP READY (can control now):")
		for _, app := range cdpApps {
			title := app.Title
			if title == "" {
				title = "(untitled)"
			}
			lines = append(lines, fmt.Sprintf("  [%d] %s — %s (port %d)", app.Index, title, app.URL, app.Port))
		}
	} else {
		lines = append(lines, "🟢 CDP READY: none")
	}

	// 2. Windows with Chromium processes but no CDP (could be enabled)
	lines = append(lines, "")
	noCdpApps := detectChromiumWithoutCDP(activePorts)
	if len(noCdpApps) > 0 {
		lines = append(lines, "🟡 CDP AVAILABLE (needs restart to enable):")
		for _, app := range noCdpApps {
			lines = append(lines, fmt.Sprintf("  • %s (PID %d) — use connect with force_restart=true", app.Name, app.PID))
		}
	} else {
		lines = append(lines, "🟡 CDP AVAILABLE: none detected")
	}

	// 3. Other visible windows (native apps)
	lines = append(lines, "")
	nativeApps := detectNativeWindows()
	if len(nativeApps) > 0 {
		lines = append(lines, "⚪ NATIVE APPS (visible windows, no CDP):")
		for _, app := range nativeApps {
			lines = append(lines, fmt.Sprintf("  • %s (PID %d)", app.Name, app.PID))
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Total: %d CDP ready, %d restartable, %d native", len(cdpApps), len(noCdpApps), len(nativeApps)))
	lines = append(lines, "Use: connect {\"target\": \"WhatsApp\"} to connect to a CDP app")
	lines = append(lines, "Use: connect {\"target\": \"WhatsApp\", \"force_restart\": true} to enable CDP on an app")

	return mcp.TextResult(strings.Join(lines, "\n"))
}

type DetectedApp struct {
	Name    string
	PID     int
	ExePath string
	IsWebView2 bool
}

// detectChromiumWithoutCDP finds WebView2/Electron processes that DON'T have CDP enabled
func detectChromiumWithoutCDP(alreadyCDP map[int][]cdp.Page) []DetectedApp {
	// Find all msedgewebview2.exe processes and check if they have a debug port
	psScript := `
		Get-CimInstance Win32_Process | Where-Object {
			($_.Name -eq 'msedgewebview2.exe' -or $_.Name -eq 'chrome.exe' -or $_.Name -eq 'msedge.exe') -and
			$_.CommandLine -like '*--embedded-browser*'
		} | ForEach-Object {
			$hasDebug = $_.CommandLine -match 'remote-debugging-port'
			$appName = ''
			if ($_.CommandLine -match 'webview-exe-name=([^\s"]+)') { $appName = $Matches[1] -replace '\.exe$','' }
			elseif ($_.CommandLine -match 'msedge\.exe') { $appName = 'Edge' }
			if (-not $hasDebug -and $appName) {
				Write-Output "$($_.ProcessId)|$appName"
			}
		} | Select-Object -Unique
	`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", psScript).Output()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var results []DetectedApp
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		pid := 0
		fmt.Sscanf(parts[0], "%d", &pid)
		name := parts[1]
		if seen[name] || name == "" {
			continue
		}
		seen[name] = true
		results = append(results, DetectedApp{Name: name, PID: pid, IsWebView2: true})
	}
	return results
}

// detectNativeWindows finds visible windows that are NOT Chromium-based
func detectNativeWindows() []DetectedApp {
	psScript := `
		Get-Process | Where-Object { $_.MainWindowTitle -ne '' } | ForEach-Object {
			Write-Output "$($_.Id)|$($_.MainWindowTitle)"
		}
	`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", psScript).Output()
	if err != nil {
		return nil
	}

	// Filter out known Chromium processes and system windows
	skipTitles := []string{"claude", "agent os", "prompt de comando", "powershell", "terminal", "gerenciador de tarefas"}
	var results []DetectedApp
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		pid := 0
		fmt.Sscanf(parts[0], "%d", &pid)
		title := parts[1]
		if title == "" {
			continue
		}
		// Skip system/known windows
		titleLower := strings.ToLower(title)
		skip := false
		for _, s := range skipTitles {
			if strings.Contains(titleLower, s) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		results = append(results, DetectedApp{Name: title, PID: pid})
	}

	// Limit to 15 most relevant
	if len(results) > 15 {
		results = results[:15]
	}
	return results
}

func sortedKeys(m map[int][]cdp.Page) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
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
	forceRestart, _ := args["force_restart"].(bool)

	// Try as index first
	if idx, err := strconv.Atoi(target); err == nil {
		apps := GetLastApps()
		if len(apps) == 0 {
			handleListApps(nil)
			apps = GetLastApps()
		}
		if idx >= 0 && idx < len(apps) {
			return connectToApp(apps[idx])
		}
		return mcp.ErrorResult(fmt.Sprintf("Index %d out of range. Run list_apps first.", idx))
	}

	// Try as name — scan CDP ports for matching page
	activePorts := cdp.ScanCDPPorts()
	targetLower := strings.ToLower(target)

	for _, port := range sortedKeys(activePorts) {
		for _, page := range activePorts[port] {
			if page.Type != "page" {
				continue
			}
			if strings.Contains(strings.ToLower(page.Title), targetLower) ||
				strings.Contains(strings.ToLower(page.URL), targetLower) {
				return connectToApp(AppEntry{Title: page.Title, URL: page.URL, Port: port, WsURL: page.WebSocketDebuggerURL})
			}
		}
	}

	// Not found via CDP — check if app is running without CDP
	if !forceRestart {
		// Check if a matching process exists
		noCdpApps := detectChromiumWithoutCDP(activePorts)
		for _, app := range noCdpApps {
			if strings.Contains(strings.ToLower(app.Name), targetLower) {
				return mcp.TextResult(fmt.Sprintf(
					"%s is running but doesn't have CDP enabled.\n"+
						"To enable control, I need to restart it with CDP support.\n"+
						"Call connect with force_restart=true to proceed:\n"+
						"connect {\"target\": %q, \"force_restart\": true}\n\n"+
						"This will close and reopen %s. Your data/conversations won't be lost.",
					app.Name, target, app.Name))
			}
		}
		return mcp.ErrorResult(fmt.Sprintf("No app found matching %q. Run inventory to see all available apps.", target))
	}

	// force_restart=true — enable CDP and reconnect
	return enableCDPAndConnect(target)
}

// enableCDPAndConnect kills an app, sets CDP env var, relaunches it, and connects
func enableCDPAndConnect(appName string) mcp.ToolResult {
	appLower := strings.ToLower(appName)
	log.Printf("[CDP] Enabling CDP for %q with force restart", appName)

	// Allocate a port
	port := findFreePort()

	// Determine app type and how to relaunch
	var launchCmd string

	// Check if it's a known WebView2 UWP app
	for key, pfn := range knownWebView2Apps {
		if strings.Contains(appLower, key) {
			// Set env var globally for WebView2
			envCmd := fmt.Sprintf(
				`[System.Environment]::SetEnvironmentVariable("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--remote-debugging-port=%d", "User")`,
				port)
			exec.Command("powershell", "-NoProfile", "-Command", envCmd).Run()

			// Kill existing process
			killCmd := fmt.Sprintf(`Get-Process | Where-Object { $_.MainWindowTitle -like "*%s*" } | Stop-Process -Force -ErrorAction SilentlyContinue`, appName)
			exec.Command("powershell", "-NoProfile", "-Command", killCmd).Run()
			time.Sleep(2 * time.Second)

			// Relaunch via explorer
			launchCmd = fmt.Sprintf("explorer.exe shell:AppsFolder\\%s!App", pfn)
			cmd := exec.Command("cmd", "/C", launchCmd)
			cmd.Start()
			break
		}
	}

	// Check if it's Edge
	if strings.Contains(appLower, "edge") || strings.Contains(appLower, "instagram") || strings.Contains(appLower, "chrome") {
		// Kill Edge
		exec.Command("powershell", "-NoProfile", "-Command", `Get-Process msedge -ErrorAction SilentlyContinue | Stop-Process -Force`).Run()
		time.Sleep(2 * time.Second)

		// Relaunch with flag
		edgePath := `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
		cmd := exec.Command(edgePath, fmt.Sprintf("--remote-debugging-port=%d", port), "--restore-last-session")
		cmd.Start()
		launchCmd = "edge"
	}

	// If we don't know how to launch, try generic kill+relaunch with env var
	if launchCmd == "" {
		envCmd := fmt.Sprintf(
			`[System.Environment]::SetEnvironmentVariable("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--remote-debugging-port=%d", "User")`,
			port)
		exec.Command("powershell", "-NoProfile", "-Command", envCmd).Run()

		killCmd := fmt.Sprintf(`Get-Process | Where-Object { $_.MainWindowTitle -like "*%s*" } | Stop-Process -Force -ErrorAction SilentlyContinue`, appName)
		exec.Command("powershell", "-NoProfile", "-Command", killCmd).Run()

		return mcp.TextResult(fmt.Sprintf(
			"Set CDP env var for port %d and killed %s.\n"+
				"Please reopen %s manually, then run connect again.\n"+
				"(Auto-launch not supported for this app yet)",
			port, appName, appName))
	}

	// Wait for app to start and CDP to be available
	log.Printf("[CDP] Waiting for %s to start on port %d...", appName, port)
	time.Sleep(5 * time.Second)

	// Try to connect (retry a few times)
	for retry := 0; retry < 5; retry++ {
		activePorts := cdp.ScanCDPPorts()
		appLower := strings.ToLower(appName)
		for _, p := range sortedKeys(activePorts) {
			for _, page := range activePorts[p] {
				if page.Type != "page" {
					continue
				}
				if strings.Contains(strings.ToLower(page.Title), appLower) ||
					strings.Contains(strings.ToLower(page.URL), appLower) {
					return connectToApp(AppEntry{Title: page.Title, URL: page.URL, Port: p, WsURL: page.WebSocketDebuggerURL})
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	return mcp.TextResult(fmt.Sprintf(
		"Relaunched %s with CDP on port %d but couldn't connect yet.\n"+
			"The app may still be loading. Try: connect {\"target\": %q} in a few seconds.",
		appName, port, appName))
}

func findFreePort() int {
	// Find a port that's not in use
	for port := 9340; port < 9400; port++ {
		if !cdp.IsPortAlive(port) {
			return port
		}
	}
	return 9350 // fallback
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
