package core

import (
	"strings"
	"testing"
)

func TestAndroidDarkModeCommand(t *testing.T) {
	if got := AndroidDarkModeCommand(true); got != "cmd uimode night yes" {
		t.Errorf("enabled → %q", got)
	}
	if got := AndroidDarkModeCommand(false); got != "cmd uimode night no" {
		t.Errorf("disabled → %q", got)
	}
}

func TestParseAndroidNightMode(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    bool
		wantErr bool
	}{
		{"labelled yes", "Night mode: yes\n", true, false},
		{"labelled no", "Night mode: no", false, false},
		{"bare value", "yes", true, false},
		{"numeric", "1", true, false},
		{"mixed case", "Night mode: YES", true, false},
		// Schedule modes: the effective appearance depends on the clock, so the
		// only claim this query supports is "not currently forced dark".
		{"auto is not forced dark", "Night mode: auto", false, false},
		{"custom is not forced dark", "Night mode: custom", false, false},
		{"unparseable is an error, not a guess", "Night mode: banana", false, true},
		{"empty is an error", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAndroidNightMode(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseAndroidNightMode(%q) expected an error", tt.output)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAndroidNightMode(%q) error = %v", tt.output, err)
			}
			if got != tt.want {
				t.Errorf("ParseAndroidNightMode(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestIOSAppearance(t *testing.T) {
	if got := IOSAppearanceValue(true); got != "dark" {
		t.Errorf("enabled → %q", got)
	}
	if got := IOSAppearanceValue(false); got != "light" {
		t.Errorf("disabled → %q", got)
	}

	tests := []struct {
		output  string
		want    bool
		wantErr bool
	}{
		{"dark\n", true, false},
		{"light", false, false},
		{"Dark", true, false},
		{"unknown", false, true},
		{"", false, true},
	}
	for _, tt := range tests {
		got, err := ParseIOSAppearance(tt.output)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseIOSAppearance(%q) expected an error", tt.output)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseIOSAppearance(%q) error = %v", tt.output, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseIOSAppearance(%q) = %v, want %v", tt.output, got, tt.want)
		}
	}
}

// The assertion message has to name both sides — "assertion failed" alone
// leaves the reader guessing which way round it went.
func TestDarkModeAssertionError(t *testing.T) {
	err := DarkModeAssertionError(true, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"expected dark", "is in light"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}
