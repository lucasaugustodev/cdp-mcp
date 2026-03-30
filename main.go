package main

import (
	"fmt"
	"os"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/dashboard"
	"github.com/lucasaugustodev/cdp-mcp/engine"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

func autoConnectCDP() {
	activePorts := cdp.ScanCDPPorts()
	for port, pages := range activePorts {
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
			tools.SetConn(conn, page.Title, page.URL, port)
			fmt.Fprintf(os.Stderr, "  Connected: %s (%s) port %d\n", page.Title, page.URL, port)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "  No CDP apps found\n")
}

func main() {
	// --dashboard mode: run ONLY the dashboard web UI (no MCP stdio)
	if len(os.Args) > 1 && os.Args[1] == "--dashboard" {
		port := 9400
		if len(os.Args) > 2 {
			fmt.Sscanf(os.Args[2], "%d", &port)
		}
		fmt.Fprintf(os.Stderr, "Dashboard mode: http://localhost:%d\n", port)
		fmt.Fprintf(os.Stderr, "Auto-connecting to CDP apps...\n")
		go autoConnectCDP()
		go engine.Start()
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

	server.Run()
}
