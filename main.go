package main

import (
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

	// Run the MCP stdio server
	server.Run()
}
