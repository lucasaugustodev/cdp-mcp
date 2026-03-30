package dashboard

import (
	"fmt"
	"log"
	"net/http"
)

// Start starts the dashboard HTTP server on the given port.
// This should be called in a goroutine as it blocks.
func Start(port int) {
	mux := http.NewServeMux()
	route(mux)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[Dashboard] Starting on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[Dashboard] Server error: %v", err)
	}
}
