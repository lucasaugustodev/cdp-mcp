package cdp

import (
	"encoding/json"
	"log"
	"sync"
)

// RecordEvent represents a user interaction captured from the page.
type RecordEvent struct {
	Action    string `json:"action"`    // "click", "input", "change"
	Selector  string `json:"selector"`  // CSS selector
	XPath     string `json:"xpath"`     // XPath (best effort)
	Text      string `json:"text"`      // element text content
	AriaLabel string `json:"ariaLabel"`
	TagName   string `json:"tagName"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Value     string `json:"value,omitempty"` // for input events
}

type recordingState struct {
	mu      sync.Mutex
	active  bool
	handler func(RecordEvent)
}

// StartRecording injects click/input/change listeners that emit events via console.log.
func (c *Connection) StartRecording() error {
	if c.recording == nil {
		c.recording = &recordingState{}
	}

	// Enable console events
	_, err := c.Send("Runtime.enable", map[string]interface{}{})
	if err != nil {
		return err
	}

	// Inject recording script
	js := `
		(() => {
			if (window.__cdpMCPRecording) return 'already_recording';
			window.__cdpMCPRecording = true;

			function getSelector(el) {
				if (el.id) return '#' + CSS.escape(el.id);
				let sel = el.tagName.toLowerCase();
				const ariaLabel = el.getAttribute('aria-label');
				if (ariaLabel) return sel + '[aria-label="' + ariaLabel.replace(/"/g, '\\"') + '"]';
				const name = el.getAttribute('name');
				if (name) return sel + '[name="' + name.replace(/"/g, '\\"') + '"]';
				return sel;
			}

			function getXPath(el) {
				const parts = [];
				while (el && el.nodeType === 1) {
					let idx = 0;
					let sibling = el.previousSibling;
					while (sibling) {
						if (sibling.nodeType === 1 && sibling.tagName === el.tagName) idx++;
						sibling = sibling.previousSibling;
					}
					parts.unshift(el.tagName.toLowerCase() + '[' + (idx + 1) + ']');
					el = el.parentNode;
				}
				return '/' + parts.join('/');
			}

			function emitEvent(action, el, extra) {
				const rect = el.getBoundingClientRect();
				const evt = {
					__cdpMCPRecord: true,
					action: action,
					selector: getSelector(el),
					xpath: getXPath(el),
					text: (el.textContent || '').trim().substring(0, 100),
					ariaLabel: el.getAttribute('aria-label') || '',
					tagName: el.tagName,
					x: Math.round(rect.x + rect.width/2),
					y: Math.round(rect.y + rect.height/2),
					...extra
				};
				console.log('__CDPMCPRECORD__' + JSON.stringify(evt));
			}

			document.addEventListener('click', e => emitEvent('click', e.target, {}), true);
			document.addEventListener('input', e => emitEvent('input', e.target, {value: (e.target.value || '').substring(0, 200)}), true);
			document.addEventListener('change', e => emitEvent('change', e.target, {value: (e.target.value || '').substring(0, 200)}), true);

			return 'recording_started';
		})()
	`
	_, err = c.Send("Runtime.evaluate", map[string]interface{}{
		"expression": js,
	})
	if err != nil {
		return err
	}

	// Subscribe to console messages to capture recording events
	c.Subscribe("Runtime.consoleAPICalled", func(params json.RawMessage) {
		var data struct {
			Type string `json:"type"`
			Args []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"args"`
		}
		if json.Unmarshal(params, &data) != nil {
			return
		}
		if data.Type != "log" || len(data.Args) == 0 {
			return
		}
		val := data.Args[0].Value
		const prefix = "__CDPMCPRECORD__"
		if len(val) <= len(prefix) || val[:len(prefix)] != prefix {
			return
		}
		jsonStr := val[len(prefix):]
		var evt RecordEvent
		if json.Unmarshal([]byte(jsonStr), &evt) != nil {
			return
		}

		c.recording.mu.Lock()
		h := c.recording.handler
		c.recording.mu.Unlock()
		if h != nil {
			h(evt)
		} else {
			log.Printf("[CDP:Record] %s on %s %q at (%d,%d)", evt.Action, evt.TagName, evt.Text, evt.X, evt.Y)
		}
	})

	c.recording.mu.Lock()
	c.recording.active = true
	c.recording.mu.Unlock()

	return nil
}

// StopRecording removes the recording listeners from the page.
func (c *Connection) StopRecording() error {
	js := `
		(() => {
			window.__cdpMCPRecording = false;
			return 'recording_stopped';
		})()
	`
	_, err := c.Send("Runtime.evaluate", map[string]interface{}{
		"expression": js,
	})
	if c.recording != nil {
		c.recording.mu.Lock()
		c.recording.active = false
		c.recording.mu.Unlock()
	}
	return err
}

// OnRecordEvent sets the callback for recording events.
func (c *Connection) OnRecordEvent(handler func(RecordEvent)) {
	if c.recording == nil {
		c.recording = &recordingState{}
	}
	c.recording.mu.Lock()
	c.recording.handler = handler
	c.recording.mu.Unlock()
}

// IsRecording returns whether recording is currently active.
func (c *Connection) IsRecording() bool {
	if c.recording == nil {
		return false
	}
	c.recording.mu.Lock()
	defer c.recording.mu.Unlock()
	return c.recording.active
}
