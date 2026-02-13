package uiautomator2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// writeJSON encodes data as JSON to the response writer.
func writeJSON(w http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func newTestClientWithSession(handler http.HandlerFunc) (*Client, *httptest.Server) {
	client, server := newTestClient(handler)
	client.sessionID = "test-session"
	return client, server
}

func TestFindElement(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/element") {
			t.Errorf("expected /element suffix, got %s", r.URL.Path)
		}

		var req FindElementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Strategy != "id" {
			t.Errorf("expected id strategy, got %s", req.Strategy)
		}
		if req.Selector != "com.example:id/button" {
			t.Errorf("expected com.example:id/button, got %s", req.Selector)
		}

		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{
				"ELEMENT": "element-123",
			},
		})
	})
	defer server.Close()

	elem, err := client.FindElement("id", "com.example:id/button")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elem.ID() != "element-123" {
		t.Errorf("expected element-123, got %s", elem.ID())
	}
}

func TestFindElementNotFound(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{},
		})
	})
	defer server.Close()

	_, err := client.FindElement("id", "not-found")
	if err == nil {
		t.Error("expected error for element not found")
	}
}

func TestFindElementWithContext(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req FindElementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Context != "parent-123" {
			t.Errorf("expected parent-123 context, got %s", req.Context)
		}

		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{
				"ELEMENT": "child-456",
			},
		})
	})
	defer server.Close()

	elem, err := client.FindElementWithContext("id", "child", "parent-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elem.ID() != "child-456" {
		t.Errorf("expected child-456, got %s", elem.ID())
	}
}

func TestFindElements(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/elements") {
			t.Errorf("expected /elements suffix, got %s", r.URL.Path)
		}

		writeJSON(w, map[string]interface{}{
			"value": []map[string]interface{}{
				{"ELEMENT": "elem-1"},
				{"ELEMENT": "elem-2"},
				{"ELEMENT": "elem-3"},
			},
		})
	})
	defer server.Close()

	elems, err := client.FindElements("className", "android.widget.Button")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elems))
	}
	if elems[0].ID() != "elem-1" {
		t.Errorf("expected elem-1, got %s", elems[0].ID())
	}
}

func TestFindElementsEmpty(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": []map[string]interface{}{},
		})
	})
	defer server.Close()

	elems, err := client.FindElements("id", "not-found")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(elems))
	}
}

func TestActiveElement(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/element/active") {
			t.Errorf("expected /element/active suffix, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{
				"ELEMENT": "active-elem",
			},
		})
	})
	defer server.Close()

	elem, err := client.ActiveElement()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elem.ID() != "active-elem" {
		t.Errorf("expected active-elem, got %s", elem.ID())
	}
}

func TestActiveElementNone(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{},
		})
	})
	defer server.Close()

	_, err := client.ActiveElement()
	if err == nil {
		t.Error("expected error for no active element")
	}
}

func TestElementClick(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/click") {
			t.Errorf("expected /element/elem-123/click, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		writeJSON(w, map[string]interface{}{})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	err := elem.Click()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementClear(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/clear") {
			t.Errorf("expected /element/elem-123/clear, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]interface{}{})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	err := elem.Clear()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementSendKeys(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/value") {
			t.Errorf("expected /element/elem-123/value, got %s", r.URL.Path)
		}

		var req InputTextRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Text != "hello world" {
			t.Errorf("expected 'hello world', got %s", req.Text)
		}
		writeJSON(w, map[string]interface{}{})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	err := elem.SendKeys("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementText(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/text") {
			t.Errorf("expected /element/elem-123/text, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		writeJSON(w, map[string]interface{}{
			"value": "Button Text",
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	text, err := elem.Text()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Button Text" {
		t.Errorf("expected 'Button Text', got %s", text)
	}
}

func TestElementAttribute(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/attribute/enabled") {
			t.Errorf("expected attribute/enabled, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]interface{}{
			"value": "true",
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	attr, err := elem.Attribute("enabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attr != "true" {
		t.Errorf("expected 'true', got %s", attr)
	}
}

func TestElementRect(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/rect") {
			t.Errorf("expected /element/elem-123/rect, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{
				"x":      100,
				"y":      200,
				"width":  50,
				"height": 30,
			},
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	rect, err := elem.Rect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rect.X != 100 || rect.Y != 200 || rect.Width != 50 || rect.Height != 30 {
		t.Errorf("unexpected rect: %+v", rect)
	}
}

func TestElementIsDisplayed(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": "true",
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	displayed, err := elem.IsDisplayed()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !displayed {
		t.Error("expected displayed to be true")
	}
}

func TestElementIsEnabled(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": "false",
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	enabled, err := elem.IsEnabled()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Error("expected enabled to be false")
	}
}

func TestElementIsSelected(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": "true",
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	selected, err := elem.IsSelected()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !selected {
		t.Error("expected selected to be true")
	}
}

func TestElementScreenshot(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem-123/screenshot") {
			t.Errorf("expected /element/elem-123/screenshot, got %s", r.URL.Path)
		}
		// Base64 encoded "PNG"
		writeJSON(w, map[string]interface{}{
			"value": "UE5H",
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	data, err := elem.Screenshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "PNG" {
		t.Errorf("expected 'PNG', got %s", string(data))
	}
}

func TestElementScreenshotInvalidResponse(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"value": 12345,
		})
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Screenshot()
	if err == nil {
		t.Error("expected error for invalid response")
	}
}

func TestElementScreenshotUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Screenshot()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFindElementUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.FindElement("id", "test")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFindElementsUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.FindElements("id", "test")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestActiveElementUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.ActiveElement()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestElementTextUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Text()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestElementAttributeUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Attribute("enabled")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestElementRectUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Rect()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestIsDisplayedError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("error")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.IsDisplayed()
	if err == nil {
		t.Error("expected error")
	}
}

func TestIsEnabledError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("error")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.IsEnabled()
	if err == nil {
		t.Error("expected error")
	}
}

func TestIsSelectedError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("error")); err != nil {
			return
		}
	})
	defer server.Close()

	elem := &Element{id: "elem-123", client: client}
	_, err := elem.IsSelected()
	if err == nil {
		t.Error("expected error")
	}
}

func TestFindElementRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.FindElement("id", "test")
	if err == nil {
		t.Error("expected error")
	}
}

func TestFindElementsRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.FindElements("id", "test")
	if err == nil {
		t.Error("expected error")
	}
}

func TestActiveElementRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.ActiveElement()
	if err == nil {
		t.Error("expected error")
	}
}

func TestElementTextRequestError(t *testing.T) {
	client := newErrorTestClient()
	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Text()
	if err == nil {
		t.Error("expected error")
	}
}

func TestElementRectRequestError(t *testing.T) {
	client := newErrorTestClient()
	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Rect()
	if err == nil {
		t.Error("expected error")
	}
}

func TestElementScreenshotRequestError(t *testing.T) {
	client := newErrorTestClient()
	elem := &Element{id: "elem-123", client: client}
	_, err := elem.Screenshot()
	if err == nil {
		t.Error("expected error")
	}
}
