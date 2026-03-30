package cdp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// screenshotResult is the shape of Page.captureScreenshot result.
type screenshotResult struct {
	Data string `json:"data"` // base64-encoded image
}

// CaptureScreenshot takes a JPEG screenshot via CDP Page.captureScreenshot.
// Returns raw JPEG bytes.
func (c *Connection) CaptureScreenshot() ([]byte, error) {
	raw, err := c.Send("Page.captureScreenshot", map[string]interface{}{
		"format":  "jpeg",
		"quality": 80,
	})
	if err != nil {
		return nil, fmt.Errorf("cdp screenshot: %w", err)
	}

	var res screenshotResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("cdp screenshot unmarshal: %w", err)
	}
	if res.Data == "" {
		return nil, fmt.Errorf("cdp screenshot: empty data")
	}

	jpegBytes, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return nil, fmt.Errorf("cdp screenshot base64 decode: %w", err)
	}

	return jpegBytes, nil
}

// CaptureScreenshotBase64 takes a JPEG screenshot and returns base64-encoded string.
func (c *Connection) CaptureScreenshotBase64() (string, error) {
	raw, err := c.Send("Page.captureScreenshot", map[string]interface{}{
		"format":  "jpeg",
		"quality": 80,
	})
	if err != nil {
		return "", fmt.Errorf("cdp screenshot: %w", err)
	}

	var res screenshotResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("cdp screenshot unmarshal: %w", err)
	}
	if res.Data == "" {
		return "", fmt.Errorf("cdp screenshot: empty data")
	}

	return res.Data, nil
}

// SetViewport forces the page viewport to a specific size.
func (c *Connection) SetViewport(width, height int) error {
	_, err := c.Send("Emulation.setDeviceMetricsOverride", map[string]interface{}{
		"width":             width,
		"height":            height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	})
	return err
}

// DispatchMouseClick sends a real mouse click at viewport coordinates via CDP.
func (c *Connection) DispatchMouseClick(x, y int) error {
	// mousePressed
	_, err := c.Send("Input.dispatchMouseEvent", map[string]interface{}{
		"type":       "mousePressed",
		"x":          x,
		"y":          y,
		"button":     "left",
		"clickCount": 1,
	})
	if err != nil {
		return err
	}
	// mouseReleased
	_, err = c.Send("Input.dispatchMouseEvent", map[string]interface{}{
		"type":       "mouseReleased",
		"x":          x,
		"y":          y,
		"button":     "left",
		"clickCount": 1,
	})
	return err
}
