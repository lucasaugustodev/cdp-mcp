package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lucasaugustodev/cdp-mcp/mcp"
)

// RegisterInteraction registers all core interaction tools.
func RegisterInteraction(server *mcp.Server) {
	server.RegisterTool(mcp.Tool{
		Name:        "find",
		Description: "Search for elements by text, title, or aria-label. Optionally click the first match. Returns element info with selectors and coordinates.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Text to search for in element text, title, aria-label, placeholder, or value",
				},
				"click": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, click the first matching element",
				},
			},
			"required": []string{"query"},
		},
	}, handleFind)

	server.RegisterTool(mcp.Tool{
		Name:        "js",
		Description: "Execute arbitrary JavaScript in the connected page. Returns the evaluation result.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"code": map[string]interface{}{
					"type":        "string",
					"description": "JavaScript code to evaluate",
				},
			},
			"required": []string{"code"},
		},
	}, handleJS)

	server.RegisterTool(mcp.Tool{
		Name:        "screenshot",
		Description: "Capture a JPEG screenshot of the connected page. Returns the image.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleScreenshot)

	server.RegisterTool(mcp.Tool{
		Name:        "type_text",
		Description: "Type text character by character via CDP Input.dispatchKeyEvent. Simulates real keyboard input.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{
					"type":        "string",
					"description": "Text to type",
				},
			},
			"required": []string{"text"},
		},
	}, handleTypeText)

	server.RegisterTool(mcp.Tool{
		Name:        "press_key",
		Description: "Press a special key via CDP Input.dispatchKeyEvent. Supports: Enter, Tab, Escape, Backspace, Space, Delete, ArrowUp, ArrowDown, ArrowLeft, ArrowRight.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "Key name: Enter, Tab, Escape, Backspace, Space, Delete, ArrowUp, ArrowDown, ArrowLeft, ArrowRight",
				},
			},
			"required": []string{"key"},
		},
	}, handlePressKey)

	server.RegisterTool(mcp.Tool{
		Name:        "scroll",
		Description: "Scroll the page by a given amount of pixels.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"x": map[string]interface{}{
					"type":        "number",
					"description": "Horizontal scroll amount in pixels (default 0)",
				},
				"y": map[string]interface{}{
					"type":        "number",
					"description": "Vertical scroll amount in pixels (positive = down, negative = up). Default 500.",
				},
			},
		},
	}, handleScroll)

	server.RegisterTool(mcp.Tool{
		Name:        "navigate",
		Description: "Navigate the connected page to a URL.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "URL to navigate to",
				},
			},
			"required": []string{"url"},
		},
	}, handleNavigate)

	server.RegisterTool(mcp.Tool{
		Name:        "elements",
		Description: "List all interactive elements on the page (buttons, links, inputs, etc.) with their selectors and coordinates.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, handleElements)
}

func handleFind(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	query, _ := args["query"].(string)
	if query == "" {
		return mcp.ErrorResult("Missing 'query' argument.")
	}
	clickFirst, _ := args["click"].(bool)

	queryJSON, _ := json.Marshal(query)

	js := fmt.Sprintf(`
		(() => {
			const query = %s.toLowerCase();
			const allEls = document.querySelectorAll('*');
			const results = [];
			const seen = new Set();

			for (const el of allEls) {
				if (seen.has(el)) continue;
				const rect = el.getBoundingClientRect();
				if (rect.width < 2 || rect.height < 2) continue;

				const text = (el.textContent || '').trim().substring(0, 200).toLowerCase();
				const ariaLabel = (el.getAttribute('aria-label') || '').toLowerCase();
				const title = (el.getAttribute('title') || '').toLowerCase();
				const placeholder = (el.getAttribute('placeholder') || '').toLowerCase();
				const value = (el.value || '').toLowerCase();
				const alt = (el.getAttribute('alt') || '').toLowerCase();

				const matches = text.includes(query) || ariaLabel.includes(query) ||
					title.includes(query) || placeholder.includes(query) ||
					value.includes(query) || alt.includes(query);

				if (!matches) continue;
				seen.add(el);

				// Build selector
				let sel = el.tagName.toLowerCase();
				if (el.id) sel = '#' + CSS.escape(el.id);
				else if (ariaLabel) sel += '[aria-label="' + el.getAttribute('aria-label').replace(/"/g, '\\"') + '"]';
				else if (el.getAttribute('name')) sel += '[name="' + el.getAttribute('name').replace(/"/g, '\\"') + '"]';

				const x = Math.round(rect.x + rect.width/2);
				const y = Math.round(rect.y + rect.height/2);

				results.push({
					tagName: el.tagName,
					text: (el.textContent || '').trim().substring(0, 80),
					ariaLabel: el.getAttribute('aria-label') || '',
					selector: sel,
					x: x,
					y: y,
					width: Math.round(rect.width),
					height: Math.round(rect.height)
				});

				if (results.length >= 20) break;
			}

			return JSON.stringify(results);
		})()
	`, string(queryJSON))

	resultStr, err := conn.EvaluateString(js)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Find failed: %v", err))
	}

	var elements []struct {
		TagName   string `json:"tagName"`
		Text      string `json:"text"`
		AriaLabel string `json:"ariaLabel"`
		Selector  string `json:"selector"`
		X         int    `json:"x"`
		Y         int    `json:"y"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(resultStr), &elements); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Failed to parse find results: %v", err))
	}

	if len(elements) == 0 {
		return mcp.TextResult(fmt.Sprintf("No elements found matching %q", query))
	}

	var lines []string
	for i, el := range elements {
		display := el.AriaLabel
		if display == "" {
			display = el.Text
		}
		if len(display) > 60 {
			display = display[:57] + "..."
		}
		lines = append(lines, fmt.Sprintf("[%d] %s %q at (%d,%d) %dx%d selector=%q",
			i, el.TagName, display, el.X, el.Y, el.Width, el.Height, el.Selector))
	}
	lines = append(lines, fmt.Sprintf("\nFound %d elements matching %q", len(elements), query))

	if clickFirst && len(elements) > 0 {
		el := elements[0]
		err := conn.DispatchMouseClick(el.X, el.Y)
		if err != nil {
			lines = append(lines, fmt.Sprintf("\nClick failed: %v", err))
		} else {
			display := el.AriaLabel
			if display == "" {
				display = el.Text
			}
			lines = append(lines, fmt.Sprintf("\nClicked: %s %q at (%d,%d)", el.TagName, display, el.X, el.Y))
		}
	}

	return mcp.TextResult(strings.Join(lines, "\n"))
}

func handleJS(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	code, _ := args["code"].(string)
	if code == "" {
		return mcp.ErrorResult("Missing 'code' argument.")
	}

	result, err := conn.EvaluateJS(code)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("JS execution failed: %v", err))
	}

	if result == nil {
		return mcp.TextResult("undefined")
	}

	switch v := result.(type) {
	case string:
		return mcp.TextResult(v)
	default:
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.TextResult(string(data))
	}
}

func handleScreenshot(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	b64, err := conn.CaptureScreenshotBase64()
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Screenshot failed: %v", err))
	}

	return mcp.ImageResult(b64, "image/jpeg")
}

func handleTypeText(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	text, _ := args["text"].(string)
	if text == "" {
		return mcp.ErrorResult("Missing 'text' argument.")
	}

	for _, ch := range text {
		charStr := string(ch)
		// Send keyDown with char
		_, err := conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyDown",
			"text": charStr,
		})
		if err != nil {
			return mcp.ErrorResult(fmt.Sprintf("Type failed at char %q: %v", charStr, err))
		}
		// Send keyUp
		_, err = conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyUp",
		})
		if err != nil {
			return mcp.ErrorResult(fmt.Sprintf("Type keyUp failed: %v", err))
		}
		time.Sleep(30 * time.Millisecond)
	}

	return mcp.TextResult(fmt.Sprintf("Typed %d characters", len([]rune(text))))
}

// keyMap maps key names to their CDP key definitions.
var keyMap = map[string]struct {
	Key            string
	Code           string
	WindowsKeyCode int
	NativeKeyCode  int
}{
	"enter":      {"Enter", "Enter", 13, 13},
	"tab":        {"Tab", "Tab", 9, 9},
	"escape":     {"Escape", "Escape", 27, 27},
	"backspace":  {"Backspace", "Backspace", 8, 8},
	"space":      {" ", "Space", 32, 32},
	"delete":     {"Delete", "Delete", 46, 46},
	"arrowup":    {"ArrowUp", "ArrowUp", 38, 38},
	"arrowdown":  {"ArrowDown", "ArrowDown", 40, 40},
	"arrowleft":  {"ArrowLeft", "ArrowLeft", 37, 37},
	"arrowright": {"ArrowRight", "ArrowRight", 39, 39},
}

func handlePressKey(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	keyName, _ := args["key"].(string)
	if keyName == "" {
		return mcp.ErrorResult("Missing 'key' argument.")
	}

	keyDef, ok := keyMap[strings.ToLower(keyName)]
	if !ok {
		supported := make([]string, 0, len(keyMap))
		for k := range keyMap {
			supported = append(supported, k)
		}
		return mcp.ErrorResult(fmt.Sprintf("Unknown key %q. Supported: %s", keyName, strings.Join(supported, ", ")))
	}

	// keyDown
	_, err := conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
		"type":                  "keyDown",
		"key":                   keyDef.Key,
		"code":                  keyDef.Code,
		"windowsVirtualKeyCode": keyDef.WindowsKeyCode,
		"nativeVirtualKeyCode":  keyDef.NativeKeyCode,
	})
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Key press failed: %v", err))
	}

	// keyUp
	_, err = conn.Send("Input.dispatchKeyEvent", map[string]interface{}{
		"type":                  "keyUp",
		"key":                   keyDef.Key,
		"code":                  keyDef.Code,
		"windowsVirtualKeyCode": keyDef.WindowsKeyCode,
		"nativeVirtualKeyCode":  keyDef.NativeKeyCode,
	})
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Key release failed: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Pressed key: %s", keyName))
}

func handleScroll(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	scrollX := 0.0
	scrollY := 500.0

	if v, ok := args["x"].(float64); ok {
		scrollX = v
	}
	if v, ok := args["y"].(float64); ok {
		scrollY = v
	}

	js := fmt.Sprintf("window.scrollBy(%d, %d); JSON.stringify({scrollX: window.scrollX, scrollY: window.scrollY})", int(scrollX), int(scrollY))
	result, err := conn.EvaluateString(js)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Scroll failed: %v", err))
	}

	return mcp.TextResult(fmt.Sprintf("Scrolled by (%d, %d). Current position: %s", int(scrollX), int(scrollY), result))
}

func handleNavigate(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	url, _ := args["url"].(string)
	if url == "" {
		return mcp.ErrorResult("Missing 'url' argument.")
	}

	_, err := conn.Send("Page.navigate", map[string]interface{}{
		"url": url,
	})
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Navigate failed: %v", err))
	}

	// Wait a moment for navigation to start
	time.Sleep(1 * time.Second)

	// Get the new URL
	newURL, _ := conn.EvaluateString("window.location.href")
	newTitle, _ := conn.EvaluateString("document.title")

	// Update stored info without closing connection
	state.mu.Lock()
	if newTitle != "" {
		state.connTitle = newTitle
	}
	if newURL != "" {
		state.connURL = newURL
	}
	state.mu.Unlock()

	return mcp.TextResult(fmt.Sprintf("Navigated to: %s\nTitle: %s", newURL, newTitle))
}

func handleElements(args map[string]interface{}) mcp.ToolResult {
	conn := GetConn()
	if conn == nil {
		return mcp.ErrorResult("Not connected to any app. Use list_apps and connect first.")
	}

	result, err := conn.GetInteractiveElements()
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("Elements listing failed: %v", err))
	}

	return mcp.TextResult(result)
}
