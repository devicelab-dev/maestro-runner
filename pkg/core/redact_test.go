package core

import "testing"

func TestRedactTypedText(t *testing.T) {
	body := `{"text":"hunter2"}`
	for _, p := range []string{
		"/session/abc/element/e1/value", "/session/abc/keys", "/session/abc/wda/keys",
		"/session/abc/element/e1/wda/type", "/session/abc/actions",
	} {
		if got := RedactTypedText(p, body); got != RedactedBody {
			t.Errorf("%s: body should be redacted, got %s", p, got)
		}
	}
	for _, p := range []string{"/session/abc/element", "/session/abc/source", "/session/abc/appium/gestures/click", "/status"} {
		if got := RedactTypedText(p, body); got != body {
			t.Errorf("%s: body should be logged as-is, got %s", p, got)
		}
	}
	if got := RedactTypedText("/session/abc/keys", ""); got != "" {
		t.Errorf("an empty body stays empty, got %q", got)
	}
}
