package tools

import (
	"strings"
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

// AppState holds the state for a single connected CDP app.
type AppState struct {
	Conn      *cdp.Connection
	AppID     string
	Title     string
	URL       string
	Port      int
	Recording bool
	Events    []cdp.RecordEvent
}

// State holds the shared state for all tools.
type State struct {
	mu          sync.RWMutex
	apps        map[string]*AppState // appId → state
	activeAppID string               // currently selected app
	lastScan    []AppEntry
	activityLog []ActivityEntry
}

// Global state instance
var state = &State{
	apps: make(map[string]*AppState),
}

// deriveAppID creates an app ID from a title (lowercase, no spaces).
func deriveAppID(title string) string {
	id := strings.ToLower(title)
	// Remove leading numbers/parens: "(2) Mensagens | LinkedIn" → "mensagens | linkedin"
	for len(id) > 0 && (id[0] == '(' || id[0] == ')' || id[0] == ' ' || (id[0] >= '0' && id[0] <= '9')) {
		id = id[1:]
	}
	// Take first meaningful word before separators
	for _, sep := range []string{" | ", " - ", " — "} {
		if idx := strings.Index(id, sep); idx > 0 {
			id = id[:idx]
		}
	}
	id = strings.TrimSpace(id)
	// Only keep alphanumeric + hyphens
	var clean []byte
	for _, c := range []byte(id) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			clean = append(clean, c)
		} else if c == ' ' || c == '-' || c == '_' {
			if len(clean) > 0 && clean[len(clean)-1] != '-' {
				clean = append(clean, '-')
			}
		}
	}
	result := strings.Trim(string(clean), "-")
	if result == "" {
		result = "default"
	}
	return result
}

// getActiveAppState returns the active app state, falling back to the first
// connected app if activeAppID is empty or stale. Caller must hold at least RLock.
func (s *State) getActiveAppState() *AppState {
	// Try the explicitly set active app
	if s.activeAppID != "" {
		if app, ok := s.apps[s.activeAppID]; ok {
			return app
		}
	}
	// Fallback: pick the first connected (non-closed) app
	for _, app := range s.apps {
		if app.Conn != nil && !app.Conn.IsClosed() {
			s.activeAppID = app.AppID
			return app
		}
	}
	return nil
}

// GetConn returns the active app's CDP connection, or nil if not connected.
func GetConn() *cdp.Connection {
	state.mu.RLock()
	defer state.mu.RUnlock()
	app := state.getActiveAppState()
	if app == nil || app.Conn == nil || app.Conn.IsClosed() {
		return nil
	}
	return app.Conn
}

// SetConn sets the active CDP connection (backwards compatible).
// Derives appId from the title.
func SetConn(conn *cdp.Connection, title, url string, port int) {
	appID := DeriveAppIDFromURL(title, url)
	SetAppConn(appID, conn, title, url, port)
}

// DeriveAppIDFromURL tries URL domain first, then falls back to title
func DeriveAppIDFromURL(title, url string) string {
	urlLower := strings.ToLower(url)
	domainMap := map[string]string{
		"whatsapp.com": "whatsapp", "linkedin.com": "linkedin", "instagram.com": "instagram",
		"facebook.com": "facebook", "twitter.com": "twitter", "x.com": "twitter",
		"youtube.com": "youtube", "spotify.com": "spotify", "github.com": "github",
		"slack.com": "slack", "discord.com": "discord", "reddit.com": "reddit",
		"notion.so": "notion", "figma.com": "figma", "mail.google.com": "gmail",
	}
	for domain, id := range domainMap {
		if strings.Contains(urlLower, domain) {
			return id
		}
	}
	return deriveAppID(title)
}

// SetAppConn sets a CDP connection for an explicit appId.
func SetAppConn(appID string, conn *cdp.Connection, title, url string, port int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	// Close previous connection for this appID if any
	if existing, ok := state.apps[appID]; ok {
		if existing.Conn != nil && !existing.Conn.IsClosed() {
			existing.Conn.Close()
		}
	}
	state.apps[appID] = &AppState{
		Conn:  conn,
		AppID: appID,
		Title: title,
		URL:   url,
		Port:  port,
	}
	// Set as active if no active app or this is the first connection
	if state.activeAppID == "" {
		state.activeAppID = appID
	}
}

// GetConnInfo returns connection metadata for the active app.
func GetConnInfo() (title, url string, port int, connected bool) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	app := state.getActiveAppState()
	if app == nil || app.Conn == nil || app.Conn.IsClosed() {
		return "", "", 0, false
	}
	return app.Title, app.URL, app.Port, true
}

// SetActiveApp switches the active app by appId.
func SetActiveApp(appID string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.apps[appID]; ok {
		state.activeAppID = appID
	}
}

// GetAppState returns the state for a specific appId, or nil.
func GetAppState(appID string) *AppState {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.apps[appID]
}

// ListConnectedApps returns all apps with live (non-closed) connections.
func ListConnectedApps() []AppState {
	state.mu.RLock()
	defer state.mu.RUnlock()
	var result []AppState
	for _, app := range state.apps {
		if app.Conn != nil && !app.Conn.IsClosed() {
			result = append(result, *app)
		}
	}
	return result
}

// SetLastApps stores the last scanned apps list.
func SetLastApps(apps []AppEntry) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.lastScan = apps
}

// GetLastApps returns the last scanned apps list.
func GetLastApps() []AppEntry {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.lastScan
}

// AppendRecordEvent appends a recording event to the active app.
func AppendRecordEvent(evt cdp.RecordEvent) {
	state.mu.Lock()
	defer state.mu.Unlock()
	app := state.getActiveAppState()
	if app != nil {
		app.Events = append(app.Events, evt)
	}
}

// GetRecordedEvents returns and clears recorded events from the active app.
func GetRecordedEvents() []cdp.RecordEvent {
	state.mu.Lock()
	defer state.mu.Unlock()
	app := state.getActiveAppState()
	if app == nil {
		return nil
	}
	events := app.Events
	app.Events = nil
	return events
}

// SetRecording sets the recording state flag on the active app.
func SetRecording(val bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	app := state.getActiveAppState()
	if app != nil {
		app.Recording = val
	}
}

// IsRecording returns whether recording is active on the active app.
func IsRecording() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	app := state.getActiveAppState()
	if app == nil {
		return false
	}
	return app.Recording
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

// GetAllConnections returns info about all live CDP connections.
// Returns a map of appId -> connection for the dashboard.
func GetAllConnections() map[string]*cdp.Connection {
	state.mu.RLock()
	defer state.mu.RUnlock()
	result := make(map[string]*cdp.Connection)
	for id, app := range state.apps {
		if app.Conn != nil && !app.Conn.IsClosed() {
			result[id] = app.Conn
		}
	}
	return result
}

// GetConnDetails returns the connection details for the active app (dashboard).
func GetConnDetails() (title, url string, port int, conn *cdp.Connection) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	app := state.getActiveAppState()
	if app == nil || app.Conn == nil || app.Conn.IsClosed() {
		return "", "", 0, nil
	}
	return app.Title, app.URL, app.Port, app.Conn
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
