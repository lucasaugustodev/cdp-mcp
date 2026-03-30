package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lucasaugustodev/cdp-mcp/mcp"
)

// RegisterAdvanced registers all advanced tools.
func RegisterAdvanced(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "network_start",
		Description: "Start capturing network requests on the connected page.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleNetworkStart)

	server.RegisterTool(mcp.Tool{
		Name:        "network_stop",
		Description: "Stop network capture and return recent requests.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"count": map[string]interface{}{
					"type":        "number",
					"description": "Number of recent requests to return (default 50)",
				},
			},
		},
	}, handleNetworkStop)

	server.RegisterTool(mcp.Tool{
		Name:        "cookies_get",
		Description: "Get all cookies for the current page via document.cookie.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleCookiesGet)

	server.RegisterTool(mcp.Tool{
		Name:        "cookies_set",
		Description: "Set a cookie on the current page.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cookie": map[string]interface{}{
					"type":        "string",
					"description": "Cookie string to set (e.g. 'name=value; path=/; expires=...')",
				},
			},
			"required": []string{"cookie"},
		},
	}, handleCookiesSet)

	server.RegisterTool(mcp.Tool{
		Name:        "storage_get",
		Description: "Read from localStorage or sessionStorage. Returns all keys or a specific key's value.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Storage type: 'local' (default) or 'session'",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "Optional key to read. If empty, returns all keys and values.",
				},
			},
		},
	}, handleStorageGet)

	server.RegisterTool(mcp.Tool{
		Name:        "storage_set",
		Description: "Write to localStorage or sessionStorage.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Storage type: 'local' (default) or 'session'",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "Key to set",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "Value to set",
				},
			},
			"required": []string{"key", "value"},
		},
	}, handleStorageSet)
}

func handleNetworkStart(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	err := conn.EnableNetworkCapture()
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to enable network capture: %v", err))
	}

	return mcp.TextResult("Network capture started. Requests are being recorded. Use network_stop to get results.")
}

func handleNetworkStop(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	count := 50
	if v, ok := args["count"].(float64); ok && v > 0 {
		count = int(v)
	}

	reqs := conn.GetRecentRequests(count)

	err := conn.DisableNetworkCapture()
	if err != nil {
		// Non-fatal, still return what we have
	}

	if len(reqs) == 0 {
		return mcp.TextResult("Network capture stopped. No requests were captured.")
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Network capture stopped. %d requests captured:", len(reqs)))
	lines = append(lines, "")

	for i, req := range reqs {
		status := fmt.Sprintf("%d", req.Status)
		if req.Status == 0 {
			status = "pending"
		}
		// Truncate long URLs
		url := req.URL
		if len(url) > 120 {
			url = url[:117] + "..."
		}
		lines = append(lines, fmt.Sprintf("[%d] %s %s — %s", i, req.Method, status, url))
	}

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleCookiesGet(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	result, err := conn.EvaluateString("document.cookie")
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to get cookies: %v", err))
	}

	if result == "" {
		return mcp.TextResult("No cookies found (document.cookie is empty). Note: HttpOnly cookies are not visible via document.cookie.")
	}

	// Parse and format cookies
	cookies := strings.Split(result, ";")
	var lines []string
	lines = append(lines, fmt.Sprintf("Cookies (%d):", len(cookies)))
	for i, c := range cookies {
		c = strings.TrimSpace(c)
		lines = append(lines, fmt.Sprintf("  [%d] %s", i, c))
	}

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleCookiesSet(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	cookie, _ := args["cookie"].(string)
	if cookie == "" {
		return mcp.ErrorResult("Missing 'cookie' argument.")
	}

	cookieJSON, _ := json.Marshal(cookie)
	js := fmt.Sprintf("document.cookie = %s; 'cookie_set'", string(cookieJSON))

	result, err := conn.EvaluateString(js)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to set cookie: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Cookie set. Result: %s", result))
}

func handleStorageGet(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	storageType := "local"
	if t, ok := args["type"].(string); ok && t == "session" {
		storageType = "session"
	}

	storageObj := "localStorage"
	if storageType == "session" {
		storageObj = "sessionStorage"
	}

	key, hasKey := args["key"].(string)

	if hasKey && key != "" {
		// Get specific key
		keyJSON, _ := json.Marshal(key)
		js := fmt.Sprintf(`
			(() => {
				const val = %s.getItem(%s);
				if (val === null) return '__NULL__';
				return val;
			})()
		`, storageObj, string(keyJSON))

		result, err := conn.EvaluateString(js)
		if err != nil {
			return mcp.ErrorResult(fmt.Sprintf("Failed to read %s: %v", storageObj, err))
		}
		if result == "__NULL__" {
			return mcp.TextResult(fmt.Sprintf("Key %q not found in %s", key, storageObj))
		}
		return mcp.TextResult(fmt.Sprintf("%s[%q] = %s", storageObj, key, result))
	}

	// Get all keys
	js := fmt.Sprintf(`
		(() => {
			const storage = %s;
			const result = {};
			for (let i = 0; i < storage.length; i++) {
				const key = storage.key(i);
				const val = storage.getItem(key);
				result[key] = val && val.length > 200 ? val.substring(0, 200) + '...' : val;
			}
			return JSON.stringify(result);
		})()
	`, storageObj)

	result, err := conn.EvaluateString(js)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to read %s: %v", storageObj, err))
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return mcp.TextResult(fmt.Sprintf("%s contents: %s", storageObj, result))
	}

	if len(data) == 0 {
		return mcp.TextResult(fmt.Sprintf("%s is empty.", storageObj))
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s (%d keys):", storageObj, len(data)))
	for k, v := range data {
		valStr := fmt.Sprintf("%v", v)
		if len(valStr) > 100 {
			valStr = valStr[:97] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %s = %s", k, valStr))
	}

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleStorageSet(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	storageType := "local"
	if t, ok := args["type"].(string); ok && t == "session" {
		storageType = "session"
	}

	storageObj := "localStorage"
	if storageType == "session" {
		storageObj = "sessionStorage"
	}

	key, _ := args["key"].(string)
	value, _ := args["value"].(string)

	if key == "" {
		return mcp.ErrorResult("Missing 'key' argument.")
	}

	keyJSON, _ := json.Marshal(key)
	valueJSON, _ := json.Marshal(value)

	js := fmt.Sprintf(`%s.setItem(%s, %s); 'stored'`, storageObj, string(keyJSON), string(valueJSON))

	result, err := conn.EvaluateString(js)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to write to %s: %v", storageObj, err))
	}

	return mcp.TextResult(fmt.Sprintf("Set %s[%q] = %q. Result: %s", storageObj, key, value, result))
}
