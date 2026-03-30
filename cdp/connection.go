package cdp

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Connection manages a CDP WebSocket connection.
type Connection struct {
	ws         *websocket.Conn
	nextID     atomic.Int64
	mu         sync.Mutex
	closed     bool
	closeCh    chan struct{}
	pending    map[int64]chan json.RawMessage // waiting for response by ID
	pendMu     sync.Mutex
	subs       map[string][]func(json.RawMessage) // event subscriptions by method
	subsMu     sync.RWMutex
	netCapture *networkCapture // network capture state
	recording  *recordingState // recording state
	WsURL      string          // the WebSocket URL we connected to
	PageTitle  string          // title of the connected page
	PageURL    string          // URL of the connected page
}

type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Dial connects to a CDP WebSocket endpoint.
func Dial(wsURL string) (*Connection, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp dial %s: %w", wsURL, err)
	}

	c := &Connection{
		ws:      ws,
		closeCh: make(chan struct{}),
		pending: make(map[int64]chan json.RawMessage),
		subs:    make(map[string][]func(json.RawMessage)),
		WsURL:   wsURL,
	}

	go c.readLoop()
	log.Printf("[CDP] Connected to %s", wsURL)
	return c, nil
}

// readLoop reads messages from the WebSocket and routes them.
func (c *Connection) readLoop() {
	defer func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		// Wake up all pending requests
		c.pendMu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.pendMu.Unlock()
	}()

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		_, data, err := c.ws.ReadMessage()
		if err != nil {
			select {
			case <-c.closeCh:
				return // clean close
			default:
				log.Printf("[CDP] Read error: %v", err)
				return
			}
		}

		var msg cdpMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.ID > 0 {
			// Response to a command
			c.pendMu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.pendMu.Unlock()
			if ok {
				if msg.Error != nil {
					errJSON, _ := json.Marshal(msg.Error)
					ch <- errJSON
				} else {
					ch <- msg.Result
				}
			}
		} else if msg.Method != "" {
			// Event
			c.subsMu.RLock()
			handlers := c.subs[msg.Method]
			c.subsMu.RUnlock()
			for _, h := range handlers {
				go h(msg.Params)
			}
		}
	}
}

// Send sends a CDP command and waits for the response.
func (c *Connection) Send(method string, params map[string]interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp connection closed")
	}
	c.mu.Unlock()

	id := c.nextID.Add(1)

	ch := make(chan json.RawMessage, 1)
	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	msg := map[string]interface{}{
		"id":     id,
		"method": method,
	}
	if params != nil {
		msg["params"] = params
	}

	data, err := json.Marshal(msg)
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	err = c.ws.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("cdp write: %w", err)
	}

	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("cdp connection closed while waiting for response")
		}
		return result, nil
	case <-time.After(30 * time.Second):
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("cdp timeout waiting for %s (id=%d)", method, id)
	}
}

// Subscribe registers a handler for a CDP event method.
func (c *Connection) Subscribe(method string, handler func(json.RawMessage)) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	c.subs[method] = append(c.subs[method], handler)
}

// Close shuts down the CDP connection.
func (c *Connection) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()

	close(c.closeCh)
	c.ws.Close()
	log.Printf("[CDP] Connection closed")
}

// IsClosed returns whether the connection has been closed.
func (c *Connection) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
