package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// sizeServer answers /window/size (failing while fail is set) and accepts
// every other call; sizeCalls counts the size reads.
func sizeServer(fail *atomic.Bool, sizeCalls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/window/size") {
			sizeCalls.Add(1)
			if fail.Load() {
				http.Error(w, `{"value":{"error":"unknown error"}}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"value":{"width":402,"height":874}}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":null}`))
	}))
}

// A startup that could not read the screen size (#201): the first point tap
// reads it from WDA, keeps it, and later taps use it without asking again.
func TestPointTapReadsScreenSizeMissedAtStartup(t *testing.T) {
	var fail atomic.Bool
	var calls atomic.Int32
	server := sizeServer(&fail, &calls)
	defer server.Close()
	driver := createTestDriver(server)
	driver.info = &core.PlatformInfo{Platform: "ios"} // startup read failed: 0x0

	for i := 0; i < 2; i++ {
		if result := driver.Execute(&flow.TapOnStep{Point: "62%,92%"}); !result.Success {
			t.Fatalf("point tap %d: %v", i+1, result.Error)
		}
	}
	if driver.info.ScreenWidth != 402 || driver.info.ScreenHeight != 874 {
		t.Errorf("kept size %dx%d, want 402x874", driver.info.ScreenWidth, driver.info.ScreenHeight)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("window size read %d times, want once", n)
	}
}

// While WDA cannot report the size, a point tap still fails with the reason,
// and the next tap asks again rather than keeping the failure.
func TestScreenSizeReadAgainUntilWDAAnswers(t *testing.T) {
	var fail atomic.Bool
	var calls atomic.Int32
	fail.Store(true)
	server := sizeServer(&fail, &calls)
	defer server.Close()
	driver := createTestDriver(server)
	driver.info = &core.PlatformInfo{Platform: "ios"}

	if _, _, err := driver.screenSize(); err == nil || !strings.Contains(err.Error(), "screen dimensions not available") {
		t.Fatalf("WDA failing: err = %v, want screen dimensions not available", err)
	}
	fail.Store(false)
	if w, h, err := driver.screenSize(); err != nil || w != 402 || h != 874 {
		t.Fatalf("WDA answering: %dx%d, %v", w, h, err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("window size read %d times, want 2 (once failing, once answering)", n)
	}
}

// noSizeDriver is a driver whose startup got no screen size and whose WDA
// cannot report one either.
func noSizeDriver(t *testing.T) *Driver {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"value":{"error":"unknown error"}}`, http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	driver := createTestDriver(server)
	driver.info = &core.PlatformInfo{Platform: "ios"}
	return driver
}
