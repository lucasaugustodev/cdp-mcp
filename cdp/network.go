package cdp

import (
	"encoding/json"
	"sync"
	"time"
)

// NetworkRequest stores a captured network request.
type NetworkRequest struct {
	URL       string    `json:"url"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"requestId"`
}

// networkCapture holds captured network data.
type networkCapture struct {
	mu       sync.Mutex
	requests []NetworkRequest
	reqMap   map[string]*NetworkRequest // requestId -> request (for filling in status)
	enabled  bool
	maxSize  int
}

// EnableNetworkCapture enables the Network domain and starts collecting requests.
func (c *Connection) EnableNetworkCapture() error {
	_, err := c.Send("Network.enable", map[string]interface{}{})
	if err != nil {
		return err
	}

	nc := &networkCapture{
		reqMap:  make(map[string]*NetworkRequest),
		maxSize: 200,
	}
	nc.enabled = true

	// Subscribe to request events
	c.Subscribe("Network.requestWillBeSent", func(params json.RawMessage) {
		var data struct {
			RequestID string `json:"requestId"`
			Request   struct {
				URL    string `json:"url"`
				Method string `json:"method"`
			} `json:"request"`
			Timestamp float64 `json:"timestamp"`
		}
		if json.Unmarshal(params, &data) != nil {
			return
		}
		req := &NetworkRequest{
			URL:       data.Request.URL,
			Method:    data.Request.Method,
			Timestamp: time.Now(),
			RequestID: data.RequestID,
		}
		nc.mu.Lock()
		nc.reqMap[data.RequestID] = req
		nc.requests = append(nc.requests, *req)
		if len(nc.requests) > nc.maxSize {
			nc.requests = nc.requests[len(nc.requests)-nc.maxSize:]
		}
		nc.mu.Unlock()
	})

	// Subscribe to response events to fill in status codes
	c.Subscribe("Network.responseReceived", func(params json.RawMessage) {
		var data struct {
			RequestID string `json:"requestId"`
			Response  struct {
				Status int `json:"status"`
			} `json:"response"`
		}
		if json.Unmarshal(params, &data) != nil {
			return
		}
		nc.mu.Lock()
		if req, ok := nc.reqMap[data.RequestID]; ok {
			req.Status = data.Response.Status
		}
		for i := len(nc.requests) - 1; i >= 0; i-- {
			if nc.requests[i].RequestID == data.RequestID {
				nc.requests[i].Status = data.Response.Status
				break
			}
		}
		nc.mu.Unlock()
	})

	c.netCapture = nc
	return nil
}

// GetRecentRequests returns the last N captured network requests.
func (c *Connection) GetRecentRequests(n int) []NetworkRequest {
	if c.netCapture == nil {
		return nil
	}
	c.netCapture.mu.Lock()
	defer c.netCapture.mu.Unlock()

	total := len(c.netCapture.requests)
	if n > total {
		n = total
	}
	result := make([]NetworkRequest, n)
	copy(result, c.netCapture.requests[total-n:])
	return result
}

// DisableNetworkCapture disables network capture.
func (c *Connection) DisableNetworkCapture() error {
	_, err := c.Send("Network.disable", map[string]interface{}{})
	c.netCapture = nil
	return err
}
