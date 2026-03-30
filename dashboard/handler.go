package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lucasaugustodev/cdp-mcp/cdp"
	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/recipes"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// appInfo is the JSON shape for the apps list API.
type appInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	Port      int    `json:"port,omitempty"`
	Connected bool   `json:"connected"`
}

// inventoryItem is the JSON shape for the inventory scan API.
type inventoryItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Port  int    `json:"port"`
	CDP   bool   `json:"cdp"`
}

// --- Handlers ---

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

// GET /api/apps — list configured apps with connection status
func handleApps(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	configured := config.ListApps()
	conns := tools.GetAllConnections()

	var result []appInfo
	for _, app := range configured {
		_, connected := conns[app.ID]
		result = append(result, appInfo{
			ID:        app.ID,
			Name:      app.Name,
			Type:      app.Type,
			URL:       app.URL,
			Port:      app.CDPPort,
			Connected: connected,
		})
	}

	// Also include any connected apps not in config (connected via MCP tools)
	connectedApps := tools.ListConnectedApps()
	configIDs := make(map[string]bool)
	for _, app := range configured {
		configIDs[app.ID] = true
	}
	for _, app := range connectedApps {
		if !configIDs[app.AppID] {
			result = append(result, appInfo{
				ID:        app.AppID,
				Name:      app.Title,
				Type:      "detected",
				URL:       app.URL,
				Port:      app.Port,
				Connected: true,
			})
		}
	}

	if result == nil {
		result = []appInfo{}
	}
	json.NewEncoder(w).Encode(result)
}

// extractAppID extracts the app ID from paths like /api/apps/:id/screenshot
// Expected format: /api/apps/{id}/{action}
func extractAppID(path string) string {
	// Remove /api/apps/ prefix
	trimmed := strings.TrimPrefix(path, "/api/apps/")
	// Get the ID part (everything before the next /)
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}

// getAppConn returns the CDP connection for a given app ID.
// It first checks tools state, then tries to find it by matching config.
func getAppConn(appID string) *cdp.Connection {
	conns := tools.GetAllConnections()
	if conn, ok := conns[appID]; ok {
		return conn
	}
	return nil
}

// GET /api/apps/:id/screenshot
func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	conn := getAppConn(appID)
	if conn == nil {
		http.Error(w, "app not connected", http.StatusNotFound)
		return
	}

	jpegBytes, err := conn.CaptureScreenshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write(jpegBytes)
}

// POST /api/apps/:id/click — body {x, y}
func handleClick(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	conn := getAppConn(appID)
	if conn == nil {
		http.Error(w, "app not connected", http.StatusNotFound)
		return
	}

	var body struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if err := conn.DispatchMouseClick(body.X, body.Y); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tools.LogActivity("dashboard:click", fmt.Sprintf("app=%s x=%d y=%d", appID, body.X, body.Y), "ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/apps/:id/type — body {text}
func handleType(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	conn := getAppConn(appID)
	if conn == nil {
		http.Error(w, "app not connected", http.StatusNotFound)
		return
	}

	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	for _, ch := range body.Text {
		conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyDown",
			"text": string(ch),
		})
		conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyUp",
		})
		time.Sleep(30 * time.Millisecond)
	}

	tools.LogActivity("dashboard:type", fmt.Sprintf("app=%s text=%q", appID, body.Text), "ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/apps/:id/select — set as active app
func handleSelect(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	tools.SetActiveApp(appID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "active": appID})
}

// POST /api/apps/add — add new app
func handleAddApp(w http.ResponseWriter, r *http.Request) {
	var body config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		body.Name = body.ID
	}

	if err := config.AddApp(body); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	tools.LogActivity("dashboard:add-app", fmt.Sprintf("id=%s name=%s", body.ID, body.Name), "ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": body.ID})
}

// DELETE /api/apps/:id — remove app
func handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	if err := config.RemoveApp(appID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	tools.LogActivity("dashboard:remove-app", fmt.Sprintf("id=%s", appID), "ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /api/apps/:id/tasks — tasks for this app
func handleTasks(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	tasks := config.ListTasksByApp(appID)
	if tasks == nil {
		tasks = []config.Task{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// GET /api/apps/:id/recipes — recipes for this app
func handleRecipes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	names := recipes.ListRecipes()
	if names == nil {
		names = []string{}
	}
	json.NewEncoder(w).Encode(names)
}

// POST /api/apps/:id/teach/start — start CDP recording
func handleTeachStart(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	conn := getAppConn(appID)
	if conn == nil {
		http.Error(w, "app not connected", http.StatusNotFound)
		return
	}

	if tools.IsRecording() {
		http.Error(w, "already recording", http.StatusConflict)
		return
	}

	// Clear previous events
	tools.GetRecordedEvents()

	// Set up event handler
	conn.OnRecordEvent(func(evt cdp.RecordEvent) {
		tools.AppendRecordEvent(evt)
		tools.LogActivity("teach:event", fmt.Sprintf("%s %q", evt.Action, evt.Text), evt.Selector)
	})

	if err := conn.StartRecording(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tools.SetRecording(true)
	tools.LogActivity("teach:start", fmt.Sprintf("app=%s", appID), "recording")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "recording"})
}

// POST /api/apps/:id/teach/stop — stop recording, save recipe
func handleTeachStop(w http.ResponseWriter, r *http.Request) {
	appID := extractAppID(r.URL.Path)
	conn := getAppConn(appID)
	if conn == nil {
		http.Error(w, "app not connected", http.StatusNotFound)
		return
	}

	if !tools.IsRecording() {
		http.Error(w, "not recording", http.StatusConflict)
		return
	}

	conn.StopRecording()
	tools.SetRecording(false)
	events := tools.GetRecordedEvents()

	// Optionally save if name provided in query
	name := r.URL.Query().Get("name")
	saved := false
	if name != "" && len(events) > 0 {
		if err := recipes.SaveRecipe(name, events); err == nil {
			saved = true
		}
	}

	tools.LogActivity("teach:stop", fmt.Sprintf("app=%s steps=%d saved=%v", appID, len(events), saved), "done")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "stopped",
		"steps":  len(events),
		"saved":  saved,
	})
}

// GET /api/inventory — scan for all available apps
func handleInventory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	activePorts := cdp.ScanCDPPorts()
	var items []inventoryItem

	for port, pages := range activePorts {
		for _, page := range pages {
			if page.Type != "page" {
				continue
			}
			id := strings.ToLower(page.Title)
			id = strings.ReplaceAll(id, " ", "-")
			if id == "" {
				id = fmt.Sprintf("page-%d", port)
			}
			// Limit ID length
			if len(id) > 40 {
				id = id[:40]
			}
			items = append(items, inventoryItem{
				ID:    id,
				Title: page.Title,
				URL:   page.URL,
				Port:  port,
				CDP:   true,
			})
		}
	}

	if items == nil {
		items = []inventoryItem{}
	}
	json.NewEncoder(w).Encode(items)
}

// GET /api/activity — recent activity log
func handleActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	entries := tools.GetRecentActivity(50)
	json.NewEncoder(w).Encode(entries)
}

// WS /ws — WebSocket for live activity updates
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Dashboard] WebSocket upgrade failed: %v", err)
		return
	}
	defer wsConn.Close()

	ch := tools.SubscribeActivity()
	defer tools.UnsubscribeActivity(ch)

	// Read goroutine to detect close
	go func() {
		for {
			if _, _, err := wsConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for entry := range ch {
		msg := map[string]interface{}{
			"type": "activity",
			"data": entry,
		}
		data, _ := json.Marshal(msg)
		if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// route registers all dashboard routes on the given ServeMux.
func route(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// Static
		case path == "/" || path == "/index.html":
			handleIndex(w, r)

		// WebSocket
		case path == "/ws":
			handleWebSocket(w, r)

		// Activity
		case path == "/api/activity":
			handleActivity(w, r)

		// Inventory
		case path == "/api/inventory":
			handleInventory(w, r)

		// Apps list
		case path == "/api/apps":
			if r.Method == "GET" {
				handleApps(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		// Add app
		case path == "/api/apps/add" && r.Method == "POST":
			handleAddApp(w, r)

		// Teach start
		case strings.HasSuffix(path, "/teach/start") && r.Method == "POST":
			handleTeachStart(w, r)

		// Teach stop
		case strings.HasSuffix(path, "/teach/stop") && r.Method == "POST":
			handleTeachStop(w, r)

		// Screenshot
		case strings.HasSuffix(path, "/screenshot"):
			handleScreenshot(w, r)

		// Click
		case strings.HasSuffix(path, "/click") && r.Method == "POST":
			handleClick(w, r)

		// Type
		case strings.HasSuffix(path, "/type") && r.Method == "POST":
			handleType(w, r)

		// Select
		case strings.HasSuffix(path, "/select") && r.Method == "POST":
			handleSelect(w, r)

		// Tasks
		case strings.HasSuffix(path, "/tasks"):
			handleTasks(w, r)

		// Recipes
		case strings.HasSuffix(path, "/recipes"):
			handleRecipes(w, r)

		// Delete app
		case strings.HasPrefix(path, "/api/apps/") && r.Method == "DELETE":
			handleDeleteApp(w, r)

		default:
			http.NotFound(w, r)
		}
	})
}
