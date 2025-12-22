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
