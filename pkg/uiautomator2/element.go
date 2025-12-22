package uiautomator2

import (
	"encoding/json"
	"fmt"
)

// Element represents a UI element on the device.
type Element struct {
	id     string
	client *Client
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
	_, err := e.client.request("POST", e.client.sessionPath("/element/"+e.id+"/click"), nil)
	return err
}

// Clear clears the element's text.
func (e *Element) Clear() error {
	_, err := e.client.request("POST", e.client.sessionPath("/element/"+e.id+"/clear"), nil)
	return err
}

// SendKeys types text into the element.
func (e *Element) SendKeys(text string) error {
	req := InputTextRequest{Text: text}
	_, err := e.client.request("POST", e.client.sessionPath("/element/"+e.id+"/value"), req)
	return err
}

// Text returns the element's text content.
func (e *Element) Text() (string, error) {
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

// Attribute returns an element attribute.
func (e *Element) Attribute(name string) (string, error) {
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
func (e *Element) Rect() (RectModel, error) {
	data, err := e.client.request("GET", e.client.sessionPath("/element/"+e.id+"/rect"), nil)
	if err != nil {
		return RectModel{}, err
	}

	var resp struct {
		Value RectModel `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return RectModel{}, err
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
