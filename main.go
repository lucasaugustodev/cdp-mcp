package main

import (
	"github.com/lucasaugustodev/cdp-mcp/dashboard"
	"github.com/lucasaugustodev/cdp-mcp/mcp"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

func main() {
	server := mcp.NewServer()

	// Register all tool groups
	tools.RegisterDiscovery(server)
	tools.RegisterInteraction(server)
	tools.RegisterRecording(server)
	tools.RegisterAdvanced(server)

	// Start the web dashboard on a separate goroutine
	go dashboard.Start(9400)

	// Run the MCP stdio server (blocks)
	server.Run()
}
