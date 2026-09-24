// SPDX-FileCopyrightText: 2025–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package uiautomator2

import (
	"encoding/json"
	"fmt"
)

// Element represents a UI element on the device.
type Element struct {
	id     string
	client *Client

	// Cached data — when set, methods return these without HTTP calls.
	// Used by the Maestro WebSocket adapter to pre-populate element data
	// from a single round-trip, avoiding extra HTTP calls for text/rect.
	cachedText *string
	cachedRect *ElementRect

	// Action callbacks — when set, used instead of HTTP calls.
	// Used by the Maestro WebSocket adapter to route actions through WebSocket.
	clickFunc    func() error
	clearFunc    func() error
	sendKeysFunc func(text string) error

	// textFunc reads the element's current text from the device. The cached
	// text is only true until something writes to the field, so a Clear or
	// SendKeys through the callbacks above marks it stale and the next Text()
	// asks the device instead. Without it a caller that types and then reads
	// back — the dropped-character check — sees the value from before typing.
	textFunc  func() (string, error)
	textStale bool
}

// NewCachedElement creates an element with pre-populated text and bounds.
// Used by the Maestro WebSocket adapter to avoid extra round-trips.
// Call SetClickFunc/SetClearFunc/SetSendKeysFunc to wire action callbacks.
func NewCachedElement(id string, text string, rect ElementRect) *Element {
	return &Element{
		id:         id,
		cachedText: &text,
		cachedRect: &rect,
	}
}

// SetClickFunc sets the callback for Click().
func (e *Element) SetClickFunc(f func() error) { e.clickFunc = f }

// SetClearFunc sets the callback for Clear().
func (e *Element) SetClearFunc(f func() error) { e.clearFunc = f }

// SetSendKeysFunc sets the callback for SendKeys().
func (e *Element) SetSendKeysFunc(f func(text string) error) { e.sendKeysFunc = f }

// SetTextFunc sets how a cached element re-reads its text once a write has
// made the cached value stale. The first read after find still comes from the
// cache, so an element that is never written to costs no extra round trip.
func (e *Element) SetTextFunc(f func() (string, error)) { e.textFunc = f }

// InvalidateText marks the cached text stale so the next Text() re-reads it
// through the text func. Callers use it when they changed the field by a route
// the element did not see, such as raw key events to whatever holds focus. It
// is a no-op without a text func: dropping the cache would send Text() to the
// HTTP client, which a cached element does not have.
func (e *Element) InvalidateText() {
	if e.textFunc != nil {
		e.textStale = true
	}
}

// ID returns the element ID.
func (e *Element) ID() string {
	return e.id
}

// FindElement finds a single element.
func (c *Client) FindElement(strategy, selector string) (*Element, error) {
	return c.FindElementWithContext(strategy, selector, "")
}

// FindElementWithContext finds an element within a parent element.
func (c *Client) FindElementWithContext(strategy, selector, contextID string) (*Element, error) {
	req := FindElementRequest{
		Strategy: strategy,
		Selector: selector,
		Context:  contextID,
	}

	data, err := c.request("POST", c.sessionPath("/element"), req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value struct {
			ELEMENT string `json:"ELEMENT"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse element response: %w", err)
	}

	if resp.Value.ELEMENT == "" {
		return nil, fmt.Errorf("element not found: %s=%s", strategy, selector)
	}

	return &Element{
		id:     resp.Value.ELEMENT,
		client: c,
	}, nil
}

// FindElements finds multiple elements.
func (c *Client) FindElements(strategy, selector string) ([]*Element, error) {
	req := FindElementRequest{
		Strategy: strategy,
		Selector: selector,
	}

	data, err := c.request("POST", c.sessionPath("/elements"), req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value []struct {
			ELEMENT string `json:"ELEMENT"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse elements response: %w", err)
	}

	elements := make([]*Element, len(resp.Value))
	for i, v := range resp.Value {
		elements[i] = &Element{id: v.ELEMENT, client: c}
	}
	return elements, nil
}

// FindAndClick finds an element and clicks it (2 HTTP calls for native client).
func (c *Client) FindAndClick(strategy, selector string) (*Element, error) {
	elem, err := c.FindElement(strategy, selector)
	if err != nil {
		return nil, err
	}
	if err := elem.Click(); err != nil {
		return nil, err
	}
	return elem, nil
}

// SendKeysToActive sends text to the currently focused element (2 HTTP calls for native client).
func (c *Client) SendKeysToActive(text string) error {
	elem, err := c.ActiveElement()
	if err != nil {
		return err
	}
	return elem.SendKeys(text)
}

// ActiveElement returns the currently focused element.
func (c *Client) ActiveElement() (*Element, error) {
	data, err := c.request("GET", c.sessionPath("/element/active"), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value struct {
			ELEMENT string `json:"ELEMENT"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	if resp.Value.ELEMENT == "" {
		return nil, fmt.Errorf("no active element")
	}

	return &Element{id: resp.Value.ELEMENT, client: c}, nil
}

// Click taps the element.
func (e *Element) Click() error {
	if e.clickFunc != nil {
		return e.clickFunc()
	}
	_, err := e.client.request("POST", e.client.sessionPath("/element/"+e.id+"/click"), nil)
	return err
}

// Clear clears the element's text.
func (e *Element) Clear() error {
	if e.clearFunc != nil {
		// Stale even on error: a failed write may still have changed the field.
		defer e.InvalidateText()
		return e.clearFunc()
	}
	_, err := e.client.request("POST", e.client.sessionPath("/element/"+e.id+"/clear"), nil)
	return err
}

// SendKeys types text into the element.
func (e *Element) SendKeys(text string) error {
	if e.sendKeysFunc != nil {
		// Stale even on error: a failed write may still have changed the
		// field, and callers fall back to key events that certainly do.
		defer e.InvalidateText()
		return e.sendKeysFunc(text)
	}
	req := InputTextRequest{Text: text}
	_, err := e.client.request("POST", e.client.sessionPath("/element/"+e.id+"/value"), req)
	return err
}

// Text returns the element's text content.
func (e *Element) Text() (string, error) {
	if e.textStale {
		return e.refreshText()
	}
	if e.cachedText != nil {
		return *e.cachedText, nil
	}

	data, err := e.client.request("GET", e.client.sessionPath("/element/"+e.id+"/text"), nil)
	if err != nil {
		return "", err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	text, _ := resp.Value.(string)
	return text, nil
}

// refreshText re-reads a stale cached text through the text func. A failed
// read is returned as an error rather than falling back to the stale cache —
// reporting the pre-write value as current is exactly the bug this avoids —
// and the element stays stale so the next read tries again.
func (e *Element) refreshText() (string, error) {
	text, err := e.textFunc()
	if err != nil {
		return "", err
	}
	e.cachedText = &text
	e.textStale = false
	return text, nil
}

// Attribute returns an element attribute.
//
// Cached elements (NewCachedElement, used by the DeviceLab WebSocket adapter)
// carry no HTTP client — every other method is cache- or callback-aware, so
// only this one could reach a nil client and crash the runner. Report it
// instead: callers already treat an attribute error as "not available".
func (e *Element) Attribute(name string) (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("attribute %q unavailable: element %s is not backed by a session", name, e.id)
	}

	data, err := e.client.request("GET", e.client.sessionPath("/element/"+e.id+"/attribute/"+name), nil)
	if err != nil {
		return "", err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	attr, _ := resp.Value.(string)
	return attr, nil
}

// Rect returns the element's bounds.
func (e *Element) Rect() (ElementRect, error) {
	if e.cachedRect != nil {
		return *e.cachedRect, nil
	}

	data, err := e.client.request("GET", e.client.sessionPath("/element/"+e.id+"/rect"), nil)
	if err != nil {
		return ElementRect{}, err
	}

	var resp struct {
		Value ElementRect `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ElementRect{}, err
	}

	return resp.Value, nil
}

// IsDisplayed checks if the element is visible.
func (e *Element) IsDisplayed() (bool, error) {
	attr, err := e.Attribute("displayed")
	if err != nil {
		return false, err
	}
	return attr == "true", nil
}

// IsEnabled checks if the element is enabled.
func (e *Element) IsEnabled() (bool, error) {
	attr, err := e.Attribute("enabled")
	if err != nil {
		return false, err
	}
	return attr == "true", nil
}

// IsSelected checks if the element is selected.
func (e *Element) IsSelected() (bool, error) {
	attr, err := e.Attribute("selected")
	if err != nil {
		return false, err
	}
	return attr == "true", nil
}

// Screenshot captures just this element.
func (e *Element) Screenshot() ([]byte, error) {
	data, err := e.client.request("GET", e.client.sessionPath("/element/"+e.id+"/screenshot"), nil)
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	b64, ok := resp.Value.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected screenshot response")
	}

	return decodeBase64(b64)
}
