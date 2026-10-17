package wda

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// focusedFieldServer is a WDA whose focused element is one text field. Keys sent
// through /wda/keys (typing into whatever has focus) pass through garble first,
// as a janky keyboard does; keys sent to the element itself, the retype path,
// land as sent.
type focusedFieldServer struct {
	mu      sync.Mutex
	value   string
	garble  func(string) string
	clears  int
	retyped int
}

func (f *focusedFieldServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		keys := func() string {
			var body struct {
				Value []string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			return strings.Join(body.Value, "")
		}
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/element/active"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"ELEMENT": "field-1"}})
		case strings.HasSuffix(path, "/element/field-1/text"):
			jsonResponse(w, map[string]interface{}{"value": f.value})
		case strings.HasSuffix(path, "/wda/keys"):
			f.value += f.garble(keys())
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(path, "/element/field-1/clear"):
			f.clears++
			f.value = ""
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(path, "/element/field-1/value"):
			f.retyped++
			f.value += keys()
			jsonResponse(w, map[string]interface{}{"value": nil})
		default:
			jsonResponse(w, map[string]interface{}{"value": nil})
		}
	}
}

const focusedEmail = "qa+login-1790574970963@example.com"

func typeIntoFocusedField(t *testing.T, garble func(string) string) (*focusedFieldServer, string) {
	t.Helper()
	f := &focusedFieldServer{garble: garble}
	server := httptest.NewServer(f.handler(t))
	defer server.Close()
	result := createTestDriver(server).inputText(&flow.InputTextStep{Text: focusedEmail})
	if !result.Success {
		t.Fatalf("inputText failed: %s", result.Message)
	}
	return f, result.Message
}

// A typed character went missing, the case the element-scoped read-back
// already handles. Typing into the focused field is how a flow usually types
// (tapOn the field, then inputText), and it now gets the same check.
func TestInputTextFocusedFieldRetypesADroppedCharacter(t *testing.T) {
	// The third character goes missing, as "2e" did from "e2e+..." on the phone.
	f, msg := typeIntoFocusedField(t, func(s string) string { return s[:2] + s[3:] })
	if f.value != focusedEmail {
		t.Fatalf("field holds %q, want %q", f.value, focusedEmail)
	}
	if f.clears != 1 || f.retyped != 1 {
		t.Errorf("cleared %d and retyped %d times, want one of each", f.clears, f.retyped)
	}
	if !strings.Contains(msg, "retyped after dropped characters") {
		t.Errorf("message %q does not say it retyped", msg)
	}
}

// A field holding more than was typed, such as a second "@" after an email,
// reads as the app's own rewriting and is left alone.
func TestInputTextFocusedFieldExtraCharacter(t *testing.T) {
	f, _ := typeIntoFocusedField(t, func(s string) string { return s + "@" })
	if f.value != focusedEmail+"@" || f.clears != 0 {
		t.Fatalf("field %q after %d clears, want it untouched", f.value, f.clears)
	}
}

func TestInputTextFocusedFieldThatLandedIsNotRetyped(t *testing.T) {
	f, _ := typeIntoFocusedField(t, func(s string) string { return s })
	if f.value != focusedEmail || f.clears != 0 || f.retyped != 0 {
		t.Fatalf("field %q, %d clears, %d retypes: want the text as typed and no retype", f.value, f.clears, f.retyped)
	}
}
