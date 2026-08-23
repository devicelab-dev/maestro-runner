package devicelab_ios

import (
	"strings"
	"testing"
	"time"
)

// TestTextEntryLanded covers the iOS analogue of the Android keyPress
// misdirection (#139): the runner types somewhere, reports success, and the
// field the flow named never receives the text.
//
// iOS gives the host no focus signal to check beforehand — the runner leaves
// SnapshotNode.Focused unpopulated — so the only available evidence is whether
// the field's value moved.
func TestTextEntryLanded(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
		typed  string
		want   bool
	}{
		{"value changed", "", "HelloWorld", "HelloWorld", true},
		{"appended to existing text", "abc", "abcdef", "def", true},
		// A secure field renders dots rather than the characters typed; the
		// value still moves, which is all this check needs.
		{"secure field shows a mask", "", "••••••", "hunter2", true},
		// Controlled inputs may transform what was typed (stripping spaces,
		// changing case). Still a change, so still landed.
		{"transformed input", "", "Itemone", "Item one", true},

		// The failure being caught: nothing moved.
		{"unchanged means nothing landed", "", "", "HelloWorld", false},
		// Placeholders live in PlaceholderValue, not Value, so an untouched
		// field reads as empty here rather than as its placeholder text.
		{"unchanged with prior content", "abc", "abc", "xyz", false},

		// Re-running a flow without clearState legitimately types a value the
		// field already holds; unchanged is correct there, not a failure.
		{"retyping identical content", "HelloWorld", "HelloWorld", "HelloWorld", true},
		{"retyping a substring already present", "HelloWorld", "HelloWorld", "World", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := textEntryLanded(tt.before, tt.after, tt.typed); got != tt.want {
				t.Errorf("textEntryLanded(%q, %q, %q) = %v, want %v",
					tt.before, tt.after, tt.typed, got, tt.want)
			}
		})
	}
}

// Verification is best-effort: with nothing to re-read it must stay silent
// rather than invent a failure, since a false failure is worse than the silent
// success it replaces.
func TestVerifyTextEntrySkipsWithoutIdentifier(t *testing.T) {
	d := &Driver{}
	if err := d.verifyTextEntry("", "before", "typed"); err != nil {
		t.Errorf("expected no verification without an identifier, got %v", err)
	}
}

// TestAwaitTextEntryPollsUntilCommitted is the regression guard for the race
// agent-device hit (#1676): the type command can return before the app has
// committed the keystrokes, so a single read can land on a field that has taken
// nothing yet. Sampling once would call that a misdirection and fail a flow
// that was working.
func TestAwaitTextEntryPollsUntilCommitted(t *testing.T) {
	reads := 0
	// Empty on the first two reads, then the text appears — the shape of a
	// simulator still committing characters when the command returned.
	read := func() (string, bool) {
		reads++
		if reads < 3 {
			return "", true
		}
		return "hello", true
	}

	err := awaitTextEntry("field", "", "hello", time.Second, time.Millisecond, read)
	if err != nil {
		t.Errorf("expected the delayed commit to be accepted, got %v", err)
	}
	if reads < 3 {
		t.Errorf("expected the value to be re-read until it moved, got %d reads", reads)
	}
}

// A field that never moves is the failure the guard exists for, and it must
// still be reported once the field has been given time to settle.
func TestAwaitTextEntryFailsWhenNothingEverLands(t *testing.T) {
	reads := 0
	read := func() (string, bool) {
		reads++
		return "", true
	}

	err := awaitTextEntry("username", "", "hello", 30*time.Millisecond, time.Millisecond, read)
	if err == nil {
		t.Fatal("expected a failure when the field never receives the text")
	}
	if !strings.Contains(err.Error(), "username") {
		t.Errorf("expected the field identifier in the error, got %q", err)
	}
	if reads < 2 {
		t.Errorf("expected more than one read before failing, got %d", reads)
	}
}

// A partial value is enough: any movement means the text reached this field,
// and waiting for the exact string would fail on controlled inputs that
// transform what was typed.
func TestAwaitTextEntryAcceptsPartialCommit(t *testing.T) {
	read := func() (string, bool) { return "hel", true }

	if err := awaitTextEntry("field", "", "hello", 30*time.Millisecond, time.Millisecond, read); err != nil {
		t.Errorf("expected a partially committed value to count as landed, got %v", err)
	}
}

// An unreadable field is not evidence that the text went elsewhere, so
// verification stays silent rather than inventing a failure.
func TestAwaitTextEntrySilentWhenUnreadable(t *testing.T) {
	read := func() (string, bool) { return "", false }

	if err := awaitTextEntry("field", "", "hello", time.Second, time.Millisecond, read); err != nil {
		t.Errorf("expected silence when the field cannot be re-read, got %v", err)
	}
}

// The normal case must not pay for the poll: text already present returns on
// the first read.
func TestAwaitTextEntryReturnsImmediatelyWhenAlreadyLanded(t *testing.T) {
	reads := 0
	read := func() (string, bool) {
		reads++
		return "hello", true
	}

	start := time.Now()
	if err := awaitTextEntry("field", "", "hello", 5*time.Second, time.Second, read); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if reads != 1 {
		t.Errorf("expected a single read on the happy path, got %d", reads)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("happy path waited %s; it should not sleep at all", elapsed)
	}
}

// TestAppearanceToDark covers the appearance decode, including the case that
// matters on upgrade: a runner built before the appearance command answers
// without the field, and reporting that as "light" would let assertLightMode
// pass with no evidence behind it.
func TestAppearanceToDark(t *testing.T) {
	tests := []struct {
		name    string
		data    *ResponseData
		want    bool
		wantErr bool
	}{
		{"dark", &ResponseData{Appearance: "dark"}, true, false},
		{"light", &ResponseData{Appearance: "light"}, false, false},
		{"nil response", nil, false, true},
		{"missing field means an older runner", &ResponseData{}, false, true},
		{"unrecognised value", &ResponseData{Appearance: "sepia"}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := appearanceToDark(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("appearanceToDark() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("appearanceToDark() = %v, want %v", got, tt.want)
			}
		})
	}
}
