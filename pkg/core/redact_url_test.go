package core

import "testing"

func TestRedactURL(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"worldidstg://verify?key=fake-key&request_id=123#fake-fragment", "worldidstg://verify"},
		{"https://fake-user:fake-password@world.org/verify?key=fake-key#fake-fragment", "https://world.org/verify"},
		{"https://example.com/path", "https://example.com/path"},
		{"https://example.com/path?", "https://example.com/path"},
		{"https://example.com/%zz?key=fake-key", "<redacted-url>"},
		{"not a URL?key=fake-key", "<redacted-url>"},
		{"custom:fake-key", "<redacted-url>"},
	} {
		if got := RedactURL(tc.raw); got != tc.want {
			t.Errorf("RedactURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
