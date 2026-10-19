package cli

import "testing"

func TestIsNewerRelease(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"1.1.30", "1.1.29", true},
		{"1.1.30", "v1.1.29", true},
		{"1.2.0", "1.1.29", true},
		{"2.0.0", "1.9.9", true},
		{"1.1.10", "1.1.9", true}, // numeric, not string, order
		{"1.1.29", "1.1.29", false},
		{"1.1.28", "1.1.29", false}, // the backwards nag
		{"1.1.9", "1.1.10", false},
		// git describe builds sit after their base tag.
		{"1.1.29", "v1.1.29-22-gf72df97", false},
		{"1.1.29", "v1.1.29-22-gf72df97-dirty", false},
		{"1.1.29", "v1.1.29-dirty", false},
		{"1.1.30", "v1.1.29-22-gf72df97", true},
		// a pre-release comes before its release
		{"1.1.30", "1.1.30-rc.1", true},
		{"1.1.30-rc.1", "1.1.30", false},
		// unreadable on either side: no notice
		{"", "1.1.29", false},
		{"1.1.30", "dev", false},
		{"1.1.30", "f72df97", false},
		{"latest", "1.1.29", false},
	} {
		if got := isNewerRelease(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewerRelease(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}
