package core

import "testing"

func TestVerifyTypedText(t *testing.T) {
	tests := []struct {
		name   string
		typed  string
		before string
		after  string
		readOK bool
		want   TextEntryVerdict
	}{
		{"exact match", "hello", "", "hello", true, TextEntryLanded},
		{"appended to existing content", "world", "hello ", "hello world", true, TextEntryLanded},
		{"dropped middle character", "Ada Lovelace", "", "Avelace", true, TextEntryDropped},
		{"dropped trailing character", "hello", "", "hell", true, TextEntryDropped},
		{"read failed", "hello", "", "", false, TextEntryUnverifiable},
		{"field reports no value", "hello", "x", "", true, TextEntryUnverifiable},
		{"nothing typed", "", "", "anything", true, TextEntryUnverifiable},
		{"masked value of the right length", "s3cret", "", "••••••", true, TextEntryLanded},
		{"masked value that is too short", "s3cret", "", "•••", true, TextEntryDropped},
		{"app rewrote the value", "5551234567", "", "(555) 123-4567", true, TextEntryTransformed},
		{"masked with asterisks", "abcd", "", "****", true, TextEntryLanded},
		// The field reads exactly as it did before: a driver reporting an empty
		// input's hint looks identical to a total loss, so neither is acted on.
		{"static hint is not a dropped keystroke", "devicelab", "Username", "Username", true, TextEntryUnverifiable},
		{"empty stays empty", "hello", "", "", true, TextEntryUnverifiable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyTypedText(tt.typed, tt.before, tt.after, tt.readOK); got != tt.want {
				t.Errorf("VerifyTypedText(%q, %q, %q, %v) = %v, want %v", tt.typed, tt.before, tt.after, tt.readOK, got, tt.want)
			}
		})
	}
}

func TestVerifyTypedTextSeparatesLossFromRewriting(t *testing.T) {
	// Retyping fixes a dropped keystroke and does nothing for a formatter, so
	// the two must not share a verdict — otherwise every formatted field pays
	// for a pointless second round of typing.
	if got := VerifyTypedText("Ada Lovelace", "", "Avelace", true); got != TextEntryDropped {
		t.Errorf("a value missing characters should be TextEntryDropped, got %v", got)
	}
	if got := VerifyTypedText("5551234567", "", "(555) 123-4567", true); got != TextEntryTransformed {
		t.Errorf("a reformatted value should be TextEntryTransformed, got %v", got)
	}
}

func TestIsMasked(t *testing.T) {
	for _, masked := range []string{"•••", "***", "●●●●"} {
		if !isMasked(masked) {
			t.Errorf("%q should read as masked", masked)
		}
	}
	for _, plain := range []string{"", "hello", "•••a"} {
		if isMasked(plain) {
			t.Errorf("%q should not read as masked", plain)
		}
	}
}
