package uiautomator2

import (
	"errors"
	"testing"
)

// staleTextElement returns a cached element whose device-side text is held in
// *device, with writes applied to it, and a counter of device reads.
func staleTextElement(cached string, device *string, reads *int, readErr *error) *Element {
	e := NewCachedElement("e1", cached, ElementRect{Width: 100, Height: 40})
	e.SetSendKeysFunc(func(text string) error { *device += text; return nil })
	e.SetClearFunc(func() error { *device = ""; return nil })
	e.SetTextFunc(func() (string, error) {
		*reads++
		if *readErr != nil {
			return "", *readErr
		}
		return *device, nil
	})
	return e
}

func TestCachedElementTextRefreshesAfterWrite(t *testing.T) {
	tests := []struct {
		name      string
		write     func(e *Element) error
		wantText  string
		wantReads int
	}{
		{"no write reads cache", func(e *Element) error { return nil }, "Username", 0},
		{"send keys re-reads device", func(e *Element) error { return e.SendKeys("bob") }, "bob", 1},
		{"clear re-reads device", func(e *Element) error { return e.Clear() }, "", 1},
		{"invalidate re-reads device", func(e *Element) error { e.InvalidateText(); return nil }, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, reads := "", 0
			var readErr error
			e := staleTextElement("Username", &device, &reads, &readErr)
			if err := tt.write(e); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := e.Text()
			if err != nil || got != tt.wantText {
				t.Fatalf("Text() = %q, %v; want %q", got, err, tt.wantText)
			}
			if reads != tt.wantReads {
				t.Errorf("device reads = %d, want %d", reads, tt.wantReads)
			}
			// A second read is served from the refreshed cache.
			if _, err := e.Text(); err != nil || reads != tt.wantReads {
				t.Errorf("second read: reads = %d, err = %v; want cached", reads, err)
			}
		})
	}
}

func TestCachedElementRefreshErrorStaysStale(t *testing.T) {
	device, reads := "", 0
	readErr := errors.New("focus moved")
	e := staleTextElement("Username", &device, &reads, &readErr)
	if err := e.SendKeys("bob"); err != nil {
		t.Fatal(err)
	}
	if got, err := e.Text(); err == nil || got != "" {
		t.Fatalf("Text() = %q, %v; want error, never the stale %q", got, err, "Username")
	}
	readErr = nil
	if got, err := e.Text(); err != nil || got != "bob" {
		t.Fatalf("retry Text() = %q, %v; want bob", got, err)
	}
	if reads != 2 {
		t.Errorf("device reads = %d, want 2", reads)
	}
}

func TestCachedElementFailedWriteStillInvalidates(t *testing.T) {
	tests := []struct {
		name  string
		write func(e *Element) error
	}{
		{"send keys", func(e *Element) error { return e.SendKeys("x") }},
		{"clear", func(e *Element) error { return e.Clear() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewCachedElement("e1", "before", ElementRect{})
			e.SetSendKeysFunc(func(string) error { return errors.New("rejected") })
			e.SetClearFunc(func() error { return errors.New("rejected") })
			e.SetTextFunc(func() (string, error) { return "after", nil })
			if err := tt.write(e); err == nil {
				t.Fatal("expected write error")
			}
			if got, _ := e.Text(); got != "after" {
				t.Errorf("Text() = %q, want device reading %q", got, "after")
			}
		})
	}
}

// Without a text func there is nothing to re-read through, and the element has
// no HTTP client: writes must leave the cache in place rather than send Text()
// to a nil client.
func TestCachedElementWithoutTextFuncKeepsCache(t *testing.T) {
	e := NewCachedElement("e1", "cached", ElementRect{})
	e.SetSendKeysFunc(func(string) error { return nil })
	e.SetClearFunc(func() error { return nil })
	if err := e.SendKeys("x"); err != nil {
		t.Fatal(err)
	}
	if err := e.Clear(); err != nil {
		t.Fatal(err)
	}
	e.InvalidateText()
	if got, err := e.Text(); err != nil || got != "cached" {
		t.Errorf("Text() = %q, %v; want cached", got, err)
	}
}
