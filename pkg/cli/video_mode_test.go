package cli

import "testing"

func TestResolveVideoMode(t *testing.T) {
	tests := []struct {
		name   string
		video  string
		record bool
		want   string
	}{
		{"nothing set", "", false, videoNever},
		{"--record alone still means always", "", true, videoAlways},
		{"--video wins over --record", videoNever, true, videoNever},
		{"on-failure", "on-failure", false, videoOnFailure},
		{"case and space tolerated", "  On-Failure ", false, videoOnFailure},
		{"typo falls back to --record", "on-failur", true, videoAlways},
		{"typo without --record records nothing", "on-failur", false, videoNever},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVideoMode(tt.video, tt.record); got != tt.want {
				t.Errorf("resolveVideoMode(%q, %v) = %q, want %q", tt.video, tt.record, got, tt.want)
			}
		})
	}
}
