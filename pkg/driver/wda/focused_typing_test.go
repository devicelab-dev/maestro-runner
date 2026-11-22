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
	// focusMoves hands focus to another element once keys are typed, as a
	// one-digit code box does.
	focusMoves bool
	typed      bool
	actives    int
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
			f.actives++
			id := "field-1"
			if f.focusMoves && f.typed {
				id = "field-2"
			}
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"ELEMENT": id}})
		case strings.HasSuffix(path, "/element/field-1/text"):
			jsonResponse(w, map[string]interface{}{"value": f.value})
		case strings.HasSuffix(path, "/wda/keys"):
			f.value += f.garble(keys())
			f.typed = true
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
	return typeTextIntoFocusedField(t, &focusedFieldServer{garble: garble}, focusedEmail)
}

func typeTextIntoFocusedField(t *testing.T, f *focusedFieldServer, text string) (*focusedFieldServer, string) {
	t.Helper()
	server := httptest.NewServer(f.handler(t))
	defer server.Close()
	result := createTestDriver(server).inputText(&flow.InputTextStep{Text: text})
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
	// One read of the active element before typing (the wait for a typing
	// target) and one after it.
	if f.actives != 2 {
		t.Errorf("read the active element %d times, want 2", f.actives)
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

// A newline submits a single-line field and a tab moves on, so the field holds
// less than was typed without anything going missing. Retyping would submit a
// second time.
func TestInputTextFocusedFieldWithNewlineOrTabIsNotRetyped(t *testing.T) {
	for _, text := range []string{"query\nmore", "first\tsecond"} {
		f := &focusedFieldServer{garble: func(s string) string {
			return strings.NewReplacer("\n", "", "\t", "").Replace(s)
		}}
		typeTextIntoFocusedField(t, f, text)
		if f.clears != 0 || f.retyped != 0 {
			t.Errorf("%q: cleared %d and retyped %d times, want neither", text, f.clears, f.retyped)
		}
	}
}

// A field that hands focus on as it fills holds only its own part of the text:
// a one-digit code box keeps "1" of "123456" and passes the rest to the boxes
// after it. It is not retyped.
func TestInputTextFocusedFieldThatHandsFocusOnIsNotRetyped(t *testing.T) {
	f := &focusedFieldServer{focusMoves: true, garble: func(s string) string { return s[:1] }}
	typeTextIntoFocusedField(t, f, "123456")
	if f.clears != 0 || f.retyped != 0 || f.value != "1" {
		t.Fatalf("box holds %q after %d clears and %d retypes, want %q untouched", f.value, f.clears, f.retyped, "1")
	}
}

// A mask that inserts a separator leaves the field holding a character that
// was not typed: "122025" in a MM/YY field holds "12/20". Retyping gives the
// same, so it is left alone.
func TestInputTextFocusedFieldWithAnInsertedCharacterIsNotRetyped(t *testing.T) {
	f := &focusedFieldServer{garble: func(s string) string { return s[:2] + "/" + s[2:4] }}
	typeTextIntoFocusedField(t, f, "122025")
	if f.clears != 0 || f.retyped != 0 || f.value != "12/20" {
		t.Fatalf("field holds %q after %d clears and %d retypes, want %q untouched", f.value, f.clears, f.retyped, "12/20")
	}
}

// A character dropped from the very end is as much a loss as one from the
// middle, and is retyped.
func TestInputTextFocusedFieldRetypesADroppedLastCharacter(t *testing.T) {
	f := &focusedFieldServer{garble: func(s string) string { return s[:len(s)-1] }}
	typeTextIntoFocusedField(t, f, "hello")
	if f.value != "hello" || f.clears != 1 || f.retyped != 1 {
		t.Fatalf("field holds %q after %d clears and %d retypes, want %q after one of each", f.value, f.clears, f.retyped, "hello")
	}
}

// What the field held before typing stays in front and is not counted as
// typed: "x" plus "hello world" with "lo" dropped is retyped, and "x" kept.
func TestInputTextFocusedFieldThatHeldTextRetypesADrop(t *testing.T) {
	f := &focusedFieldServer{value: "x", garble: func(s string) string { return strings.Replace(s, "lo", "", 1) }}
	typeTextIntoFocusedField(t, f, "hello world")
	if f.value != "xhello world" || f.clears != 1 {
		t.Fatalf("field holds %q after %d clears, want %q after one", f.value, f.clears, "xhello world")
	}
}

// `retype: false` leaves a field that cuts what it is given, a stake field
// keeping two decimals, as the app made it: retyping would change the app's
// state. The step's message notes the difference.
func TestInputTextRetypeFalseLeavesAShortenedField(t *testing.T) {
	f := &focusedFieldServer{garble: func(s string) string { return strings.TrimSuffix(s, "999") }}
	server := httptest.NewServer(f.handler(t))
	defer server.Close()
	retype := false
	result := createTestDriver(server).inputText(&flow.InputTextStep{Text: "3,23999", Retype: &retype})
	if !result.Success {
		t.Fatalf("inputText failed: %s", result.Message)
	}
	if f.value != "3,23" || f.clears != 0 || f.retyped != 0 {
		t.Fatalf("field %q, %d clears, %d retypes: want it as the app left it", f.value, f.clears, f.retyped)
	}
	if !strings.Contains(result.Message, "field holds 4 of the 7 characters typed; not retyped") {
		t.Errorf("message %q does not note the shorter field", result.Message)
	}
}
