package tools

import (
	"sync"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
)

// AppEntry represents a discovered CDP-capable app/page.
type AppEntry struct {
	Index int
	Title string
	URL   string
	Port  int
	WsURL string
}

// State holds the shared state for all tools.
type State struct {
	mu             sync.RWMutex
	conn           *cdp.Connection
	connTitle      string
	connURL        string
	connPort       int
	lastApps       []AppEntry
	recordedEvents []cdp.RecordEvent
	recording      bool
}

// Global state instance
var state = &State{}

// GetConn returns the current CDP connection, or nil if not connected.
func GetConn() *cdp.Connection {
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.conn != nil && state.conn.IsClosed() {
		return nil
	}
	return state.conn
}

// SetConn sets the active CDP connection.
func SetConn(conn *cdp.Connection, title, url string, port int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	// Close previous connection if any
	if state.conn != nil && !state.conn.IsClosed() {
		state.conn.Close()
	}
	state.conn = conn
	state.connTitle = title
	state.connURL = url
	state.connPort = port
}

// GetConnInfo returns connection metadata.
func GetConnInfo() (title, url string, port int, connected bool) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.conn == nil || state.conn.IsClosed() {
		return "", "", 0, false
	}
	return state.connTitle, state.connURL, state.connPort, true
}

// SetLastApps stores the last scanned apps list.
func SetLastApps(apps []AppEntry) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.lastApps = apps
}

// GetLastApps returns the last scanned apps list.
func GetLastApps() []AppEntry {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.lastApps
}

// AppendRecordEvent appends a recording event.
func AppendRecordEvent(evt cdp.RecordEvent) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.recordedEvents = append(state.recordedEvents, evt)
}

// GetRecordedEvents returns and clears recorded events.
func GetRecordedEvents() []cdp.RecordEvent {
	state.mu.Lock()
	defer state.mu.Unlock()
	events := state.recordedEvents
	state.recordedEvents = nil
	return events
}

// SetRecording sets the recording state flag.
func SetRecording(val bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.recording = val
}

// IsRecording returns whether recording is active.
func IsRecording() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.recording
}
