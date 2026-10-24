package core

import (
	"fmt"
	"strings"
	"testing"
)

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

// recordingField is a text field whose value the test sets, and which
// records what a retry typed into it.
type recordingField struct {
	value string
	typed []string
}

func (f *recordingField) Text() (string, error) { return f.value, nil }
func (f *recordingField) Clear() error          { f.value = ""; return nil }
func (f *recordingField) Input(s string) error {
	f.typed = append(f.typed, s)
	f.value += s
	return nil
}

// A retype clears the whole field, so what it held before this step typed
// must go back in with it. Before the fix, "abc" + a dropped "defgh" came
// back as "defgh".
func TestConfirmTypedTextKeepsExistingText(t *testing.T) {
	tests := []struct {
		name, before, typed, after, want string
	}{
		{"existing text survives", "abc", "defgh", "abcd", "abcdefgh"},
		{"empty field", "", "hello", "hel", "hello"},
		{"hint replaced by typing is not kept", "Username", "devicelab", "devi", "devicelab"},
		{"typed text starting with before is ambiguous, nothing kept", "Search", "Search tips", "Search t", "Search tips"},
		{"masked value is not retyped as bullets", "•••", "secret", "•••••", "secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &recordingField{value: tt.after}
			note := ConfirmTypedText(f, tt.typed, tt.before, true, nil)
			if !strings.Contains(note, "retyped") && !strings.Contains(note, "warning") {
				t.Fatalf("no retry happened (note %q)", note)
			}
			if f.value != tt.want {
				t.Errorf("field holds %q after the retry, want %q", f.value, tt.want)
			}
		})
	}
}

// The retype warning goes to the console and the log, so it gives counts and
// never the text: what was typed may be a password.
func TestConfirmTypedTextWarningHidesText(t *testing.T) {
	var logged []string
	warn := func(format string, args ...interface{}) { logged = append(logged, fmt.Sprintf(format, args...)) }
	f := &recordingField{value: "hunt"}
	ConfirmTypedText(f, "hunter2", "", true, warn)
	if len(logged) != 1 {
		t.Fatalf("warnings = %v, want one", logged)
	}
	if strings.Contains(logged[0], "hunt") {
		t.Errorf("warning shows the text: %q", logged[0])
	}
	if !strings.Contains(logged[0], "4 character(s) after typing 7") {
		t.Errorf("warning = %q, want the counts", logged[0])
	}
}

// With retype false a field that reads back short is noted, not typed again.
func TestConfirmTypedTextWithoutRetype(t *testing.T) {
	f := &recordingField{value: "3,23"}
	note := ConfirmTypedText(f, "3,23999", "", false, nil)
	if f.value != "3,23" || len(f.typed) != 0 {
		t.Fatalf("field %q after typing %q, want it untouched", f.value, f.typed)
	}
	if note != " (field holds 4 of the 7 characters typed; not retyped)" {
		t.Errorf("note = %q", note)
	}
}
