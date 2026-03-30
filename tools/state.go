package tools

import (
	"sync"
	"time"

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

// ActivityEntry represents a logged tool call for the dashboard.
type ActivityEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Tool      string    `json:"tool"`
	Args      string    `json:"args"`
	Result    string    `json:"result"`
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
	activityLog    []ActivityEntry
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

// LogActivity logs a tool call for the dashboard activity feed.
func LogActivity(tool, args, result string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	entry := ActivityEntry{
		Timestamp: time.Now(),
		Tool:      tool,
		Args:      args,
		Result:    result,
	}
	state.activityLog = append(state.activityLog, entry)
	// Keep last 200 entries
	if len(state.activityLog) > 200 {
		state.activityLog = state.activityLog[len(state.activityLog)-200:]
	}
	// Notify dashboard listeners
	notifyActivity(entry)
}

// GetRecentActivity returns the last n activity entries.
func GetRecentActivity(n int) []ActivityEntry {
	state.mu.RLock()
	defer state.mu.RUnlock()
	total := len(state.activityLog)
	if n > total {
		n = total
	}
	result := make([]ActivityEntry, n)
	copy(result, state.activityLog[total-n:])
	return result
}

// GetAllConnections returns info about the active CDP connection.
// Returns a map of id -> connection for the dashboard.
func GetAllConnections() map[string]*cdp.Connection {
	state.mu.RLock()
	defer state.mu.RUnlock()
	result := make(map[string]*cdp.Connection)
	if state.conn != nil && !state.conn.IsClosed() {
		result[state.connTitle] = state.conn
	}
	return result
}

// GetConnDetails returns the connection details for the dashboard.
func GetConnDetails() (title, url string, port int, conn *cdp.Connection) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.conn == nil || state.conn.IsClosed() {
		return "", "", 0, nil
	}
	return state.connTitle, state.connURL, state.connPort, state.conn
}

// activityListeners holds WebSocket notification channels.
var activityMu sync.RWMutex
var activityListeners []chan ActivityEntry

// SubscribeActivity registers a channel to receive activity updates.
func SubscribeActivity() chan ActivityEntry {
	ch := make(chan ActivityEntry, 50)
	activityMu.Lock()
	activityListeners = append(activityListeners, ch)
	activityMu.Unlock()
	return ch
}

// UnsubscribeActivity removes a channel from activity updates.
func UnsubscribeActivity(ch chan ActivityEntry) {
	activityMu.Lock()
	defer activityMu.Unlock()
	for i, c := range activityListeners {
		if c == ch {
			activityListeners = append(activityListeners[:i], activityListeners[i+1:]...)
			close(ch)
			return
		}
	}
}

// notifyActivity sends an activity entry to all listeners.
func notifyActivity(entry ActivityEntry) {
	activityMu.RLock()
	defer activityMu.RUnlock()
	for _, ch := range activityListeners {
		select {
		case ch <- entry:
		default:
			// Drop if buffer full
		}
	}
}
