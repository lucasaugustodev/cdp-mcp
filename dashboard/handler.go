package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lucasaugustodev/cdp-mcp/tools"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// sessionInfo is the JSON shape for the sessions API.
type sessionInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Port  int    `json:"port"`
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	title, url, port, conn := tools.GetConnDetails()
	var sessions []sessionInfo
	if conn != nil {
		sessions = append(sessions, sessionInfo{
			ID:    "main",
			Title: title,
			URL:   url,
			Port:  port,
		})
	}

	json.NewEncoder(w).Encode(sessions)
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /api/sessions/:id/screenshot
	_, _, _, conn := tools.GetConnDetails()
	if conn == nil {
		http.Error(w, "no active connection", http.StatusNotFound)
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

func handleClick(w http.ResponseWriter, r *http.Request) {
	_, _, _, conn := tools.GetConnDetails()
	if conn == nil {
		http.Error(w, "no active connection", http.StatusNotFound)
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

	err := conn.DispatchMouseClick(body.X, body.Y)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tools.LogActivity("dashboard:click", fmt.Sprintf("x=%d y=%d", body.X, body.Y), "ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleType(w http.ResponseWriter, r *http.Request) {
	_, _, _, conn := tools.GetConnDetails()
	if conn == nil {
		http.Error(w, "no active connection", http.StatusNotFound)
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

	tools.LogActivity("dashboard:type", body.Text, "ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	entries := tools.GetRecentActivity(50)
	json.NewEncoder(w).Encode(entries)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Dashboard] WebSocket upgrade failed: %v", err)
		return
	}
	defer wsConn.Close()

	// Subscribe to activity updates
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

// route is a simple path-based router helper.
func route(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case path == "/" || path == "/index.html":
			handleIndex(w, r)
		case path == "/api/sessions":
			handleSessions(w, r)
		case path == "/api/activity":
			handleActivity(w, r)
		case path == "/ws":
			handleWebSocket(w, r)
		case strings.HasSuffix(path, "/screenshot"):
			handleScreenshot(w, r)
		case strings.HasSuffix(path, "/click") && r.Method == "POST":
			handleClick(w, r)
		case strings.HasSuffix(path, "/type") && r.Method == "POST":
			handleType(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
