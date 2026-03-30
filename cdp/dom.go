package cdp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// evaluateResult is the shape of Runtime.evaluate result.
type evaluateResult struct {
	Result struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	} `json:"result"`
	ExceptionDetails *json.RawMessage `json:"exceptionDetails,omitempty"`
}

// EvaluateJS evaluates a JavaScript expression and returns the result.
func (c *Connection) EvaluateJS(expr string) (interface{}, error) {
	raw, err := c.Send("Runtime.evaluate", map[string]interface{}{
		"expression":    expr,
		"returnByValue": true,
	})
	if err != nil {
		return nil, err
	}

	var res evaluateResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("cdp eval unmarshal: %w", err)
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("cdp JS exception: %s", string(*res.ExceptionDetails))
	}

	var val interface{}
	if res.Result.Type == "string" {
		var s string
		json.Unmarshal(res.Result.Value, &s)
		return s, nil
	}
	json.Unmarshal(res.Result.Value, &val)
	return val, nil
}

// EvaluateString is a helper that expects a string result from JS.
func (c *Connection) EvaluateString(expr string) (string, error) {
	val, err := c.EvaluateJS(expr)
	if err != nil {
		return "", err
	}
	if s, ok := val.(string); ok {
		return s, nil
	}
	// May be a number or null
	data, _ := json.Marshal(val)
	return string(data), nil
}

// QueryElements evaluates a JS expression that should return an element list description.
func (c *Connection) QueryElements(jsExpression string) (string, error) {
	return c.EvaluateString(jsExpression)
}

// ClickElement clicks an element found by CSS selector via JS click().
func (c *Connection) ClickElement(selector string) error {
	js := fmt.Sprintf(`
		(() => {
			const el = document.querySelector(%s);
			if (!el) return 'error: element not found for selector';
			el.scrollIntoView({block: 'center'});
			el.focus();
			el.click();
			return 'clicked';
		})()
	`, jsStringLiteral(selector))

	result, err := c.EvaluateString(js)
	if err != nil {
		return err
	}
	if strings.HasPrefix(result, "error:") {
		return fmt.Errorf("cdp click: %s", result)
	}
	return nil
}

// TypeInElement focuses an element and sets its value with input events.
func (c *Connection) TypeInElement(selector, text string) error {
	js := fmt.Sprintf(`
		(() => {
			const el = document.querySelector(%s);
			if (!el) return 'error: element not found for selector';
			el.scrollIntoView({block: 'center'});
			el.focus();
			el.value = %s;
			el.dispatchEvent(new Event('input', {bubbles: true}));
			el.dispatchEvent(new Event('change', {bubbles: true}));
			return 'typed';
		})()
	`, jsStringLiteral(selector), jsStringLiteral(text))

	result, err := c.EvaluateString(js)
	if err != nil {
		return err
	}
	if strings.HasPrefix(result, "error:") {
		return fmt.Errorf("cdp type: %s", result)
	}
	return nil
}

// GetElementAtPoint returns info about the element at the given viewport coordinates.
func (c *Connection) GetElementAtPoint(x, y int) (string, error) {
	js := fmt.Sprintf(`
		(() => {
			const el = document.elementFromPoint(%d, %d);
			if (!el) return JSON.stringify({error: 'no element at point'});
			const rect = el.getBoundingClientRect();
			return JSON.stringify({
				tagName: el.tagName,
				text: (el.textContent || '').trim().substring(0, 100),
				ariaLabel: el.getAttribute('aria-label') || '',
				id: el.id || '',
				className: (el.className || '').toString().substring(0, 100),
				x: Math.round(rect.x + rect.width/2),
				y: Math.round(rect.y + rect.height/2)
			});
		})()
	`, x, y)
	return c.EvaluateString(js)
}

// GetInteractiveElements returns a numbered list of interactive elements.
func (c *Connection) GetInteractiveElements() (string, error) {
	js := `
		(() => {
			const seen = new Set();
			const results = [];
			const selectors = [
				'button', 'a[href]', 'input', 'textarea', 'select',
				'[role="button"]', '[role="link"]', '[role="menuitem"]',
				'[role="tab"]', '[role="checkbox"]', '[role="radio"]',
				'[role="textbox"]', '[role="combobox"]', '[role="option"]',
				'[contenteditable="true"]', '[tabindex]'
			];
			const allEls = document.querySelectorAll(selectors.join(','));
			allEls.forEach(el => {
				if (seen.has(el)) return;
				seen.add(el);
				const rect = el.getBoundingClientRect();
				if (rect.width < 3 || rect.height < 3) return;
				if (rect.bottom < 0 || rect.top > window.innerHeight + 200) return;
				if (rect.right < 0 || rect.left > window.innerWidth + 200) return;
				const style = window.getComputedStyle(el);
				if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return;

				const tag = el.tagName;
				const text = (el.textContent || '').trim().replace(/\s+/g, ' ').substring(0, 60);
				const ariaLabel = el.getAttribute('aria-label') || '';
				const role = el.getAttribute('role') || '';
				const inputType = el.getAttribute('type') || '';
				const placeholder = el.getAttribute('placeholder') || '';
				const name = el.getAttribute('name') || '';
				const x = Math.round(rect.x + rect.width/2);
				const y = Math.round(rect.y + rect.height/2);

				let sel = tag.toLowerCase();
				if (el.id) {
					sel = '#' + CSS.escape(el.id);
				} else {
					if (ariaLabel) sel += '[aria-label="' + ariaLabel.replace(/"/g, '\\"') + '"]';
					else if (name) sel += '[name="' + name.replace(/"/g, '\\"') + '"]';
					else if (inputType) sel += '[type="' + inputType + '"]';
				}

				let elemType = 'Element';
				const tagLower = tag.toLowerCase();
				if (tagLower === 'button' || role === 'button') elemType = 'Button';
				else if (tagLower === 'a') elemType = 'Link';
				else if (tagLower === 'input') {
					if (inputType === 'checkbox' || role === 'checkbox') elemType = 'CheckBox';
					else if (inputType === 'radio' || role === 'radio') elemType = 'RadioButton';
					else elemType = 'Edit';
				}
				else if (tagLower === 'textarea' || role === 'textbox') elemType = 'Edit';
				else if (tagLower === 'select' || role === 'combobox') elemType = 'ComboBox';
				else if (role === 'menuitem') elemType = 'MenuItem';
				else if (role === 'tab') elemType = 'Tab';
				else if (role === 'link') elemType = 'Link';
				else if (role === 'option') elemType = 'ListItem';

				let display = ariaLabel || text || placeholder || name || '';
				if (display.length > 60) display = display.substring(0, 57) + '...';

				results.push({
					type: elemType,
					display: display,
					ariaLabel: ariaLabel,
					selector: sel,
					x: x,
					y: y
				});
			});

			const lines = results.map((r, i) =>
				'[' + i + '] ' + r.type + ' "' + r.display + '"' +
				(r.ariaLabel ? ' aria="' + r.ariaLabel.substring(0, 50) + '"' : '') +
				' at (' + r.x + ',' + r.y + ')' +
				' selector="' + r.selector + '"'
			);
			lines.push('Total: ' + results.length + ' elements (CDP)');
			return lines.join('\n');
		})()
	`
	return c.EvaluateString(js)
}

// ClickElementByIndex clicks the Nth interactive element.
func (c *Connection) ClickElementByIndex(index int) (string, error) {
	js := fmt.Sprintf(`
		(() => {
			const seen = new Set();
			const results = [];
			const selectors = [
				'button', 'a[href]', 'input', 'textarea', 'select',
				'[role="button"]', '[role="link"]', '[role="menuitem"]',
				'[role="tab"]', '[role="checkbox"]', '[role="radio"]',
				'[role="textbox"]', '[role="combobox"]', '[role="option"]',
				'[contenteditable="true"]', '[tabindex]'
			];
			const allEls = document.querySelectorAll(selectors.join(','));
			allEls.forEach(el => {
				if (seen.has(el)) return;
				seen.add(el);
				const rect = el.getBoundingClientRect();
				if (rect.width < 3 || rect.height < 3) return;
				if (rect.bottom < 0 || rect.top > window.innerHeight + 200) return;
				if (rect.right < 0 || rect.left > window.innerWidth + 200) return;
				const style = window.getComputedStyle(el);
				if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return;
				results.push(el);
			});
			if (%d >= results.length) return 'error: index %d out of range (total: ' + results.length + ')';
			const el = results[%d];
			el.scrollIntoView({block: 'center'});
			el.focus();
			const rect = el.getBoundingClientRect();
			const cx = rect.x + rect.width / 2;
			const cy = rect.y + rect.height / 2;
			const opts = {bubbles: true, cancelable: true, clientX: cx, clientY: cy, button: 0};
			el.dispatchEvent(new MouseEvent('mousedown', opts));
			el.dispatchEvent(new MouseEvent('mouseup', opts));
			el.dispatchEvent(new MouseEvent('click', opts));
			const text = (el.textContent || '').trim().substring(0, 50);
			const ariaLabel = el.getAttribute('aria-label') || '';
			return 'Clicked: ' + el.tagName + ' "' + (ariaLabel || text) + '"';
		})()
	`, index, index, index)

	result, err := c.EvaluateString(js)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(result, "error:") {
		return "", fmt.Errorf("cdp click by index: %s", result)
	}
	return result, nil
}

// SetValueByIndex sets the value on the Nth interactive element.
func (c *Connection) SetValueByIndex(index int, text string) (string, error) {
	js := fmt.Sprintf(`
		(() => {
			const seen = new Set();
			const results = [];
			const selectors = [
				'button', 'a[href]', 'input', 'textarea', 'select',
				'[role="button"]', '[role="link"]', '[role="menuitem"]',
				'[role="tab"]', '[role="checkbox"]', '[role="radio"]',
				'[role="textbox"]', '[role="combobox"]', '[role="option"]',
				'[contenteditable="true"]', '[tabindex]'
			];
			const allEls = document.querySelectorAll(selectors.join(','));
			allEls.forEach(el => {
				if (seen.has(el)) return;
				seen.add(el);
				const rect = el.getBoundingClientRect();
				if (rect.width < 3 || rect.height < 3) return;
				if (rect.bottom < 0 || rect.top > window.innerHeight + 200) return;
				if (rect.right < 0 || rect.left > window.innerWidth + 200) return;
				const style = window.getComputedStyle(el);
				if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return;
				results.push(el);
			});
			if (%d >= results.length) return 'error: index %d out of range (total: ' + results.length + ')';
			const el = results[%d];
			el.scrollIntoView({block: 'center'});
			el.focus();
			if (el.getAttribute('contenteditable') === 'true') {
				el.textContent = %s;
				el.dispatchEvent(new Event('input', {bubbles: true}));
			} else {
				el.value = %s;
				el.dispatchEvent(new Event('input', {bubbles: true}));
				el.dispatchEvent(new Event('change', {bubbles: true}));
			}
			return 'Set value on: ' + el.tagName + ' "' + (el.getAttribute('aria-label') || el.getAttribute('placeholder') || '') + '"';
		})()
	`, index, index, index, jsStringLiteral(text), jsStringLiteral(text))

	result, err := c.EvaluateString(js)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(result, "error:") {
		return "", fmt.Errorf("cdp set value by index: %s", result)
	}
	return result, nil
}

// jsStringLiteral returns a JSON-encoded JS string literal for safe interpolation.
func jsStringLiteral(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
