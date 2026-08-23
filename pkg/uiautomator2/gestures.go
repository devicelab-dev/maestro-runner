package uiautomator2

// Click performs a tap at coordinates or on an element.
func (c *Client) Click(x, y int) error {
	req := ClickRequest{
		Offset: &PointModel{X: x, Y: y},
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/click"), req)
	return err
}

// ClickElement performs a tap on an element.
func (c *Client) ClickElement(elementID string) error {
	req := ClickRequest{
		Origin: &ElementModel{ELEMENT: elementID},
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/click"), req)
	return err
}

// LongClick performs a long press at coordinates.
func (c *Client) LongClick(x, y, durationMs int) error {
	req := LongClickRequest{
		Offset:   &PointModel{X: x, Y: y},
		Duration: durationMs,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/long_click"), req)
	return err
}

// LongClickElement performs a long press on an element.
func (c *Client) LongClickElement(elementID string, durationMs int) error {
	req := LongClickRequest{
		Origin:   &ElementModel{ELEMENT: elementID},
		Duration: durationMs,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/long_click"), req)
	return err
}

// DoubleClick performs a double tap at coordinates.
func (c *Client) DoubleClick(x, y int) error {
	req := ClickRequest{
		Offset: &PointModel{X: x, Y: y},
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/double_click"), req)
	return err
}

// DoubleClickElement performs a double tap on an element.
func (c *Client) DoubleClickElement(elementID string) error {
	req := ClickRequest{
		Origin: &ElementModel{ELEMENT: elementID},
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/double_click"), req)
	return err
}

// Swipe performs a swipe gesture on an element.
func (c *Client) Swipe(elementID, direction string, percent float64, speed int) error {
	req := SwipeRequest{
		Origin:    &ElementModel{ELEMENT: elementID},
		Direction: direction,
		Percent:   percent,
		Speed:     speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/swipe"), req)
	return err
}

// SwipeInArea performs a swipe gesture in a rectangular area.
func (c *Client) SwipeInArea(area RectModel, direction string, percent float64, speed int) error {
	req := SwipeRequest{
		Area:      &area,
		Direction: direction,
		Percent:   percent,
		Speed:     speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/swipe"), req)
	return err
}

// Scroll performs a scroll gesture on an element.
func (c *Client) Scroll(elementID, direction string, percent float64, speed int) error {
	req := ScrollRequest{
		Origin:    &ElementModel{ELEMENT: elementID},
		Direction: direction,
		Percent:   percent,
		Speed:     speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/scroll"), req)
	return err
}

// ScrollInArea performs a scroll gesture in a rectangular area.
func (c *Client) ScrollInArea(area RectModel, direction string, percent float64, speed int) error {
	req := ScrollRequest{
		Area:      &area,
		Direction: direction,
		Percent:   percent,
		Speed:     speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/scroll"), req)
	return err
}

// Drag performs a drag gesture from an element to coordinates.
func (c *Client) Drag(elementID string, endX, endY, speed int) error {
	req := DragRequest{
		Origin: &ElementModel{ELEMENT: elementID},
		EndX:   endX,
		EndY:   endY,
		Speed:  speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/drag"), req)
	return err
}

// DragAndDrop presses at (fromX, fromY), holds for holdMs, drags to
// (toX, toY) over moveMs, settles briefly, then releases.
//
// Goes through the W3C actions endpoint rather than the gestures API: the
// server's /gestures/drag has no hold phase, and reorder UIs only lift the
// item after a long press. The move is split into small steps because drop
// targets track the finger — a single jump can skip every zone in between.
func (c *Client) DragAndDrop(fromX, fromY, toX, toY, holdMs, moveMs int) error {
	const moveSteps = 20

	actions := []map[string]interface{}{
		{"type": "pointerMove", "duration": 0, "x": fromX, "y": fromY},
		{"type": "pointerDown", "button": 0},
		{"type": "pause", "duration": holdMs},
	}
	stepDuration := moveMs / moveSteps
	for i := 1; i <= moveSteps; i++ {
		frac := float64(i) / float64(moveSteps)
		actions = append(actions, map[string]interface{}{
			"type":     "pointerMove",
			"duration": stepDuration,
			"x":        fromX + int(float64(toX-fromX)*frac),
			"y":        fromY + int(float64(toY-fromY)*frac),
		})
	}
	actions = append(actions,
		// Hold still before lifting so the drop registers as a deliberate
		// placement rather than a fling.
		map[string]interface{}{"type": "pause", "duration": 250},
		map[string]interface{}{"type": "pointerUp", "button": 0},
	)

	req := map[string]interface{}{
		"actions": []map[string]interface{}{
			{
				"type":       "pointer",
				"id":         "finger1",
				"parameters": map[string]interface{}{"pointerType": "touch"},
				"actions":    actions,
			},
		},
	}
	_, err := c.request("POST", c.sessionPath("/actions"), req)
	return err
}

// PinchOpen performs a pinch-open (zoom in) gesture.
func (c *Client) PinchOpen(elementID string, percent float64, speed int) error {
	req := PinchRequest{
		Origin:  &ElementModel{ELEMENT: elementID},
		Percent: percent,
		Speed:   speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/pinch_open"), req)
	return err
}

// PinchClose performs a pinch-close (zoom out) gesture.
func (c *Client) PinchClose(elementID string, percent float64, speed int) error {
	req := PinchRequest{
		Origin:  &ElementModel{ELEMENT: elementID},
		Percent: percent,
		Speed:   speed,
	}
	_, err := c.request("POST", c.sessionPath("/appium/gestures/pinch_close"), req)
	return err
}
