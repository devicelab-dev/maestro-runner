package uiautomator2

import (
	"strings"
	"testing"
)

func TestAppTerminationError(t *testing.T) {
	const pkg = "com.testhiveapp"
	crashLog := "F libc    : Fatal signal 11 (SIGSEGV), code 1 in tid 2000 (com.testhiveapp)\n" +
		"F DEBUG   : pid: 2000, name: com.testhiveapp  >>> com.testhiveapp <<<"

	t.Run("app alive → nil", func(t *testing.T) {
		sh := &shellMock{responses: map[string]string{"pidof": "1234"}}
		d := New(&MockUIA2Client{}, nil, sh)
		d.currentAppID = pkg
		if err := d.appTerminationError(); err != nil {
			t.Errorf("expected nil for a live app, got %v", err)
		}
	})

	t.Run("process gone with native crash → crash summary", func(t *testing.T) {
		sh := &shellMock{responses: map[string]string{"pidof": "", "logcat": crashLog}}
		d := New(&MockUIA2Client{}, nil, sh)
		d.currentAppID = pkg
		err := d.appTerminationError()
		if err == nil {
			t.Fatal("expected a termination error, got nil")
		}
		if !strings.Contains(err.Error(), "native") || !strings.Contains(err.Error(), pkg) {
			t.Errorf("expected native-crash summary for %s, got %v", pkg, err)
		}
	})

	t.Run("process gone, no crash log → generic termination", func(t *testing.T) {
		sh := &shellMock{responses: map[string]string{"pidof": "", "logcat": ""}}
		d := New(&MockUIA2Client{}, nil, sh)
		d.currentAppID = pkg
		err := d.appTerminationError()
		if err == nil || !strings.Contains(err.Error(), "no longer running") {
			t.Errorf("expected generic termination error, got %v", err)
		}
	})

	t.Run("no app id → nil", func(t *testing.T) {
		d := New(&MockUIA2Client{}, nil, &shellMock{})
		if err := d.appTerminationError(); err != nil {
			t.Errorf("expected nil with no app id, got %v", err)
		}
	})

	t.Run("notFoundOrCrash prefers crash over original", func(t *testing.T) {
		sh := &shellMock{responses: map[string]string{"pidof": "", "logcat": crashLog}}
		d := New(&MockUIA2Client{}, nil, sh)
		d.currentAppID = pkg
		got := d.notFoundOrCrash(errString("element not found"))
		if strings.Contains(got.Error(), "element not found") {
			t.Errorf("expected crash error to replace not-found, got %v", got)
		}
	})
}

type errString string

func (e errString) Error() string { return string(e) }
