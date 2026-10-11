package executor

import (
	"errors"
	"testing"
	"time"
)

func TestCaptureWithin(t *testing.T) {
	data, err := captureWithin("screenshot", time.Second, func() ([]byte, error) { return []byte("png"), nil })
	if err != nil || string(data) != "png" {
		t.Errorf("fast read: %q, %v", data, err)
	}
	if _, err := captureWithin("screenshot", time.Second, func() ([]byte, error) { return nil, errors.New("boom") }); err == nil {
		t.Error("a failing read must report its error")
	}

	// A hung agent: the read never answers in time, and the step's report
	// must not wait for it.
	release := make(chan struct{})
	defer close(release)
	start := time.Now()
	_, err = captureWithin("view hierarchy", 50*time.Millisecond, func() ([]byte, error) { <-release; return nil, nil })
	if err == nil || time.Since(start) > time.Second {
		t.Errorf("hung read: err %v after %v; want a prompt timeout", err, time.Since(start))
	}
}
