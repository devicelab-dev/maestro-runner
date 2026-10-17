package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

const copyTextSource = `<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" x="0" y="0" width="390" height="844">
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="total" value="42" label="Total" x="20" y="100" width="200" height="30"/>
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="email" placeholderValue="Email address" label="" x="20" y="200" width="300" height="44"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`

// copyTextServer finds every element through WDA and answers text reads with
// texts in turn, the last one repeating; textError in texts is an error
// response. sourceOK decides whether the page source can be read.
const textError = "<read error>"

type copyTextServer struct {
	mu          sync.Mutex
	texts       []string
	textReads   int
	sourceReads int
}

func (s *copyTextServer) start(t *testing.T, sourceOK bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		s.mu.Lock()
		defer s.mu.Unlock()
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/source"):
			s.sourceReads++
			if !sourceOK {
				w.WriteHeader(http.StatusInternalServerError)
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "unknown error", "message": "snapshot failed"}})
				return
			}
			jsonResponse(w, map[string]interface{}{"value": copyTextSource})
		case strings.HasSuffix(p, "/elements"):
			jsonResponse(w, map[string]interface{}{"value": []map[string]interface{}{{"ELEMENT": "el"}}})
		case strings.HasSuffix(p, "/element") && r.Method == "POST":
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"ELEMENT": "el"}})
		case strings.HasSuffix(p, "/text"):
			i := s.textReads
			s.textReads++
			if i >= len(s.texts) {
				i = len(s.texts) - 1
			}
			if s.texts[i] == textError {
				w.WriteHeader(http.StatusInternalServerError)
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "stale element reference", "message": "stale"}})
				return
			}
			jsonResponse(w, map[string]interface{}{"value": s.texts[i]})
		case strings.HasSuffix(p, "/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 20, "y": 100, "width": 200, "height": 30}})
		case strings.HasSuffix(p, "/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
}

// A text read that fails alongside the lookup is read again; it no longer
// copies an empty string (it did, once, on a real device).
func TestCopyTextFromReadsTextAgain(t *testing.T) {
	s := &copyTextServer{texts: []string{textError, "42"}}
	server := s.start(t, true)
	defer server.Close()
	d := createTestDriver(server)

	res := d.copyTextFrom(&flow.CopyTextFromStep{Selector: flow.Selector{ID: "total"}})
	if !res.Success || res.Data != "42" {
		t.Fatalf("copied %q (success=%v), want %q: %s", res.Data, res.Success, "42", res.Message)
	}
	if s.sourceReads != 0 {
		t.Errorf("read the page source %d times; the second text read was enough", s.sourceReads)
	}
}

// When the text cannot be read at all, the page source supplies it, value
// first as Maestro takes it.
func TestCopyTextFromFallsBackToPageSource(t *testing.T) {
	s := &copyTextServer{texts: []string{textError}}
	server := s.start(t, true)
	defer server.Close()
	d := createTestDriver(server)

	res := d.copyTextFrom(&flow.CopyTextFromStep{Selector: flow.Selector{ID: "total"}})
	if !res.Success || res.Data != "42" {
		t.Fatalf("copied %q (success=%v), want the value %q: %s", res.Data, res.Success, "42", res.Message)
	}
}

// With neither the text nor the page source readable, the step fails rather
// than copying nothing.
func TestCopyTextFromFailsWhenTextUnreadable(t *testing.T) {
	s := &copyTextServer{texts: []string{textError}}
	server := s.start(t, false)
	defer server.Close()
	d := createTestDriver(server)

	res := d.copyTextFrom(&flow.CopyTextFromStep{Selector: flow.Selector{ID: "total"}})
	if res.Success {
		t.Fatalf("copyTextFrom passed with %q although no read worked", res.Data)
	}
	if !strings.Contains(res.Message, "Could not read the text") {
		t.Errorf("message should say the text could not be read: %s", res.Message)
	}
}

// An element whose text reads back empty has no text; that is copied without
// a trip to the page source.
func TestCopyTextFromEmptyTextIsCopied(t *testing.T) {
	s := &copyTextServer{texts: []string{""}}
	server := s.start(t, true)
	defer server.Close()
	d := createTestDriver(server)

	res := d.copyTextFrom(&flow.CopyTextFromStep{Selector: flow.Selector{ID: "total"}})
	if !res.Success || res.Data != "" {
		t.Fatalf("copied %q (success=%v), want the empty text: %s", res.Data, res.Success, res.Message)
	}
	if s.textReads != 2 || s.sourceReads != 0 {
		t.Errorf("text reads = %d, page source reads = %d; want 2 and 0", s.textReads, s.sourceReads)
	}
}

// The page source copies what Maestro copies: the value, else the
// placeholder, else the label. It copied only the label.
func TestElementTextOrder(t *testing.T) {
	for _, tc := range []struct {
		e    ParsedElement
		want string
	}{
		{ParsedElement{Value: "42", Label: "Total"}, "42"},
		{ParsedElement{PlaceholderValue: "Email address"}, "Email address"},
		{ParsedElement{Label: "Continue"}, "Continue"},
		{ParsedElement{}, ""},
	} {
		if got := elementText(&tc.e); got != tc.want {
			t.Errorf("elementText(%+v) = %q, want %q", tc.e, got, tc.want)
		}
	}

	server := sourceServer(t, copyTextSource)
	defer server.Close()
	d := createTestDriver(server)
	res := d.copyTextFrom(&flow.CopyTextFromStep{Selector: flow.Selector{ID: "email"}})
	if !res.Success || res.Data != "Email address" {
		t.Errorf("copied %q from an empty field, want its placeholder: %s", res.Data, res.Message)
	}
}
