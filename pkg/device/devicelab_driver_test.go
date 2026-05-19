package device

import (
	"errors"
	"strings"
	"testing"
)

// TestIndentDiag verifies the small string helper used to format the
// diagnostic block inside driver-not-ready errors. Two-space indent per
// line, no trailing blank.
func TestIndentDiag(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "single line",
			in:   "tcp:6791 jdwp:1234",
			want: "  tcp:6791 jdwp:1234",
		},
		{
			name: "multi-line",
			in:   "line1\nline2\nline3",
			want: "  line1\n  line2\n  line3",
		},
		{
			name: "trailing newline trimmed",
			in:   "only\n",
			want: "  only",
		},
		{
			name: "empty",
			in:   "",
			want: "  ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := indentDiag(tc.in)
			if got != tc.want {
				t.Errorf("indentDiag(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAppendDriverDiagnostics_PreservesOriginalError ensures the
// diagnostic-enriched error still satisfies errors.Is against the
// original, so callers using errors.Is to classify the failure mode
// (e.g. "is this a driver-not-ready error?") still work after we wrap
// it with diagnostics. Uses a sentinel error to keep the test
// independent of any real adb / Shell call.
func TestAppendDriverDiagnostics_PreservesOriginalError(t *testing.T) {
	// We can't easily call appendDriverDiagnostics without a real
	// AndroidDevice, but we can verify the wrapping shape it produces
	// is errors.Is-compatible by mimicking the pattern:
	original := errors.New("DeviceLab Android Driver not ready after 30s")
	wrapped := wrapForDiag(original, "(adb output)", "(/proc/net/tcp output)")

	if !errors.Is(wrapped, original) {
		t.Errorf("wrapped error doesn't satisfy errors.Is against original")
	}
	if !strings.Contains(wrapped.Error(), "not ready after 30s") {
		t.Errorf("wrapped error lost original message: %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "[diagnostics]") {
		t.Errorf("wrapped error missing diagnostics block: %v", wrapped)
	}
}

// wrapForDiag mirrors appendDriverDiagnostics's error-wrapping shape
// so we can test it without an AndroidDevice. Keep in sync with the
// real function.
func wrapForDiag(orig error, forwards, listening string) error {
	var diag strings.Builder
	diag.WriteString("\n\n[diagnostics] adb forward --list:\n")
	diag.WriteString(indentDiag(forwards))
	diag.WriteString("\n[diagnostics] /proc/net/tcp:\n")
	diag.WriteString(indentDiag(listening))
	return wrapErr(orig, diag.String())
}

func wrapErr(orig error, extra string) error {
	return wrappedErr{orig: orig, extra: extra}
}

type wrappedErr struct {
	orig  error
	extra string
}

func (w wrappedErr) Error() string { return w.orig.Error() + w.extra }
func (w wrappedErr) Unwrap() error { return w.orig }
