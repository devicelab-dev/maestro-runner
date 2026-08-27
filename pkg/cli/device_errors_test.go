package cli

import (
	"strings"
	"testing"
)

func TestNoAndroidDevicesMessage(t *testing.T) {
	tests := []struct {
		name     string
		busy     int
		notReady int
		want     string
	}{
		{"nothing attached", 0, 0, "maestro-runner devices"},
		{"all busy", 2, 0, "already in use"},
		{"unauthorized", 0, 1, "USB debugging prompt"},
		{"busy wins over not-ready", 1, 1, "already in use"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noAndroidDevicesMessage(tt.busy, tt.notReady)
			if !strings.Contains(got, tt.want) {
				t.Errorf("message %q should mention %q", got, tt.want)
			}
		})
	}
}
