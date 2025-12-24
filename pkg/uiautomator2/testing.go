package uiautomator2

// NewTestElement creates an Element for testing purposes.
// This should only be used in tests.
func NewTestElement(id string, client *Client) *Element {
	return &Element{
		id:     id,
		client: client,
	}
}

// NewMockClient creates a Client with a nil HTTP client for testing.
// Methods will need to be mocked or will panic.
func NewMockClient() *Client {
	return &Client{
		baseURL:   "http://mock",
		sessionID: "mock-session",
	}
}

// SetSession sets the session ID for testing purposes.
// This should only be used in tests.
func (c *Client) SetSession(sessionID string) {
	c.sessionID = sessionID
}

// SetBaseURL sets the base URL for testing purposes.
// This should only be used in tests.
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}
