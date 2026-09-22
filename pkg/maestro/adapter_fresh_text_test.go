package maestro

import (
	"strings"
	"sync"
	"testing"
)

// fieldAt is where the found EditText sits; the focused-element reply is
// compared against it.
var fieldAt = BoundsResult{X: 40, Y: 300, Width: 600, Height: 120}

// fakeField simulates one EditText on the device: find returns its hint, writes
// change its value, and UI.activeElement reports whatever `focused` returns.
type fakeField struct {
	mu      sync.Mutex
	value   string
	methods []string
	focused func(value string) interface{}
}

func (f *fakeField) handle(req Request) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methods = append(f.methods, req.Method)
	switch req.Method {
	case "UI.findElement":
		return ElementResult{ElementID: "e7", Text: "Username", Bounds: fieldAt}
	case "Input.sendKeys":
		p, _ := req.Params.(map[string]interface{})
		text, _ := p["text"].(string)
		f.value += text
	case "Input.clearElement":
		f.value = ""
	case "UI.activeElement":
		return f.focused(f.value)
	}
	return map[string]interface{}{}
}

func (f *fakeField) calls(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.methods {
		if m == method {
			n++
		}
	}
	return n
}

func focusedAt(b BoundsResult) func(string) interface{} {
	return func(v string) interface{} { return ElementResult{ElementID: "e9", Text: v, Bounds: b} }
}

func TestAdapterElementTextAfterWrite(t *testing.T) {
	grown := fieldAt
	grown.Height = 240
	moved := fieldAt
	moved.Y = 600

	tests := []struct {
		name     string
		focused  func(string) interface{}
		wantText string
		wantErr  string
	}{
		{"same field focused", focusedAt(fieldAt), "bo", ""},
		{"multi-line field grew", focusedAt(grown), "bo", ""},
		{"another field focused", focusedAt(moved), "", "different view"},
		{"no focused element", func(string) interface{} {
			return &ErrorPayload{Code: "error", Message: "No focused element"}
		}, "", "No focused element"},
		{"malformed reply", func(string) interface{} { return "garbage" }, "", "parse activeElement"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := &fakeField{focused: tt.focused}
			adapter, cleanup := adapterWithMock(t, field.handle)
			defer cleanup()

			elem, err := adapter.FindElement("id", "username")
			if err != nil {
				t.Fatal(err)
			}
			if before, _ := elem.Text(); before != "Username" {
				t.Fatalf("first read = %q, want the cached find text", before)
			}
			if field.calls("UI.activeElement") != 0 {
				t.Fatal("first read after find must come from the cache")
			}
			if err := elem.SendKeys("bo"); err != nil {
				t.Fatal(err)
			}

			got, err := elem.Text()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Text() = %q, %v; want error containing %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.wantText {
				t.Fatalf("Text() = %q, %v; want %q", got, err, tt.wantText)
			}
		})
	}
}

func TestAdapterElementTextAfterClear(t *testing.T) {
	field := &fakeField{value: "bob", focused: focusedAt(fieldAt)}
	adapter, cleanup := adapterWithMock(t, field.handle)
	defer cleanup()

	elem, err := adapter.FindElement("id", "username")
	if err != nil {
		t.Fatal(err)
	}
	if err := elem.Clear(); err != nil {
		t.Fatal(err)
	}
	if got, err := elem.Text(); err != nil || got != "" {
		t.Fatalf("Text() after clear = %q, %v; want empty", got, err)
	}
	if n := field.calls("UI.activeElement"); n != 1 {
		t.Errorf("UI.activeElement calls = %d, want 1", n)
	}
}

// writable is the slice of an element these tests write to and read back.
type writable interface {
	SendKeys(string) error
	Text() (string, error)
}

// Every constructor must produce an element that refreshes, not just FindElement.
func TestAdapterAllElementSourcesRefresh(t *testing.T) {
	sources := map[string]func(a *Adapter) (writable, error){
		"ActiveElement": func(a *Adapter) (writable, error) {
			return a.ActiveElement()
		},
		"FindFirstOf": func(a *Adapter) (writable, error) {
			return a.FindFirstOf([]string{"id", "username"})
		},
		"FindAndClick": func(a *Adapter) (writable, error) {
			return a.FindAndClick("id", "username")
		},
	}
	for name, find := range sources {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			value := ""
			adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
				mu.Lock()
				defer mu.Unlock()
				if req.Method == "Input.sendKeys" {
					value = "typed"
					return map[string]interface{}{}
				}
				return ElementResult{ElementID: "e1", Text: value, Bounds: fieldAt}
			})
			defer cleanup()

			elem, err := find(adapter)
			if err != nil {
				t.Fatal(err)
			}
			if err := elem.SendKeys("typed"); err != nil {
				t.Fatal(err)
			}
			if got, err := elem.Text(); err != nil || got != "typed" {
				t.Errorf("Text() = %q, %v; want typed", got, err)
			}
		})
	}
}

// A click does not change a field's text, so it must not cost a re-read.
func TestAdapterElementClickKeepsCache(t *testing.T) {
	field := &fakeField{focused: focusedAt(fieldAt)}
	adapter, cleanup := adapterWithMock(t, field.handle)
	defer cleanup()

	elem, err := adapter.FindElement("id", "username")
	if err != nil {
		t.Fatal(err)
	}
	if err := elem.Click(); err != nil {
		t.Fatal(err)
	}
	if got, _ := elem.Text(); got != "Username" {
		t.Errorf("Text() = %q, want cached %q", got, "Username")
	}
	if field.calls("Gesture.click") != 1 || field.calls("UI.activeElement") != 0 {
		t.Errorf("calls = %v, want one Gesture.click and no re-read", field.methods)
	}
}
