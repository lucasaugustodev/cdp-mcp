package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/dashboard"
	"github.com/lucasaugustodev/cdp-mcp/engine"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

// autoLoadApps loads all configured apps and connects those with AutoStart=true.
// It scans CDP ports once and matches each auto-start app by name/URL.
// The first successfully connected app is set as the active app.
func autoLoadApps() {
	apps := config.LoadApps()
	if len(apps.Apps) == 0 {
		fmt.Fprintf(os.Stderr, "  No configured apps found, falling back to port scan\n")
		autoConnectFirstCDP()
		return
	}

	// Scan all CDP ports once (shared across all apps)
	activePorts := cdp.ScanCDPPorts()
	if len(activePorts) == 0 {
		fmt.Fprintf(os.Stderr, "  No CDP ports found\n")
		return
	}

	connected := 0
	for _, app := range apps.Apps {
		if !app.AutoStart {
			continue
		}

		found := false
		nameLower := strings.ToLower(app.Name)

		// If app has a specific CDP port configured, try that first
		if app.CDPPort > 0 {
			if pages, ok := activePorts[app.CDPPort]; ok {
				for _, page := range pages {
					if page.Type != "page" || page.WebSocketDebuggerURL == "" {
						continue
					}
					conn, err := cdp.Dial(page.WebSocketDebuggerURL)
					if err != nil {
						continue
					}
					conn.PageTitle = page.Title
					conn.PageURL = page.URL
					_ = conn.SetViewport(1440, 900)
					tools.SetAppConn(app.ID, conn, page.Title, page.URL, app.CDPPort)
					fmt.Fprintf(os.Stderr, "  Connected: %s → %s (port %d)\n", app.Name, page.Title, app.CDPPort)
					found = true
					connected++
					break
				}
			}
			if found {
				continue
			}
		}

		// Scan all ports to find a page matching the app name or URL
		for port, pages := range activePorts {
			if found {
				break
			}
			for _, page := range pages {
				if page.Type != "page" || page.WebSocketDebuggerURL == "" {
					continue
				}
				titleLower := strings.ToLower(page.Title)
				urlLower := strings.ToLower(page.URL)

				if strings.Contains(titleLower, nameLower) ||
					strings.Contains(urlLower, nameLower) ||
					(app.URL != "" && strings.Contains(urlLower, strings.ToLower(app.URL))) {
					conn, err := cdp.Dial(page.WebSocketDebuggerURL)
					if err != nil {
						continue
					}
					conn.PageTitle = page.Title
					conn.PageURL = page.URL
					_ = conn.SetViewport(1440, 900)
					tools.SetAppConn(app.ID, conn, page.Title, page.URL, port)
					fmt.Fprintf(os.Stderr, "  Connected: %s → %s (port %d)\n", app.Name, page.Title, port)
					found = true
					connected++
					break
				}
			}
		}

		if !found {
			fmt.Fprintf(os.Stderr, "  Warning: app %q (auto_start) not found on any CDP port\n", app.Name)
		}
	}

	if connected == 0 {
		fmt.Fprintf(os.Stderr, "  No auto-start apps connected, falling back to port scan\n")
		autoConnectFromPorts(activePorts)
	} else {
		fmt.Fprintf(os.Stderr, "  Auto-loaded %d app(s)\n", connected)
	}
}

// autoConnectFirstCDP connects to the first available CDP page (no configured apps).
func autoConnectFirstCDP() {
	activePorts := cdp.ScanCDPPorts()
	autoConnectFromPorts(activePorts)
}

// autoConnectFromPorts connects to ALL available CDP pages from a port scan result.
func autoConnectFromPorts(activePorts map[int][]cdp.Page) {
	connected := 0
	seen := make(map[string]bool) // avoid duplicate IDs
	for port, pages := range activePorts {
		for _, page := range pages {
			if page.Type != "page" || page.WebSocketDebuggerURL == "" {
				continue
			}
			// Skip empty/internal pages
			if page.Title == "" || strings.HasPrefix(page.URL, "edge://") || strings.HasPrefix(page.URL, "chrome://") {
				continue
			}
			conn, err := cdp.Dial(page.WebSocketDebuggerURL)
			if err != nil {
				continue
			}
			conn.PageTitle = page.Title
			conn.PageURL = page.URL
			_ = conn.SetViewport(1440, 900)
			// Derive ID from URL to avoid duplicates
			id := tools.DeriveAppIDFromURL(page.Title, page.URL)
			if seen[id] {
				conn.Close()
				continue
			}
			seen[id] = true
			tools.SetAppConn(id, conn, page.Title, page.URL, port)
			fmt.Fprintf(os.Stderr, "  Connected: %s [%s] port %d\n", id, page.Title, port)
			connected++
		}
	}
	if connected == 0 {
		fmt.Fprintf(os.Stderr, "  No CDP apps found\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Connected %d app(s)\n", connected)
	}
}

func main() {
	// --dashboard mode: run ONLY the dashboard web UI (no MCP stdio)
	if len(os.Args) > 1 && os.Args[1] == "--dashboard" {
		port := 9400
		if len(os.Args) > 2 {
			fmt.Sscanf(os.Args[2], "%d", &port)
		}
		fmt.Fprintf(os.Stderr, "Dashboard mode: http://localhost:%d\n", port)
		fmt.Fprintf(os.Stderr, "Auto-loading configured apps...\n")
		go engine.Start()
		go autoLoadApps()
		dashboard.Start(port) // blocks forever
		return
	}

	// Normal MCP mode: stdio JSON-RPC + dashboard on port 9400
	server := mcp.NewServer()

	tools.RegisterDiscovery(server)
	tools.RegisterInteraction(server)
	tools.RegisterRecording(server)
	tools.RegisterAdvanced(server)
	tools.RegisterApps(server)
	tools.RegisterTasks(server)

	go dashboard.Start(9400)
	go engine.Start()
	go autoLoadApps()

	server.Run()
}
