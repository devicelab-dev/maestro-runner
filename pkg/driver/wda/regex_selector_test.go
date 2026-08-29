package wda

import "testing"

// An anchored pattern exists to distinguish one element from another. Matching
// it case-insensitively throws that away: `^SIGN OUT$` accepted a "Sign out"
// row as readily as the "SIGN OUT" button it was written for, and whichever
// came first in the page source won the tap (#151).
func TestMatchesTextHonoursAnchorsAndCase(t *testing.T) {
	if !matchesText("^SIGN OUT$", "SIGN OUT") {
		t.Error("should match the text it was written for")
	}
	if matchesText("^SIGN OUT$", "Sign out") {
		t.Error("an anchored pattern must not match a differently-cased string")
	}
	if matchesText("^SIGN OUT$", "Sign out?") {
		t.Error("an anchored pattern must not match a longer string")
	}
}

// Plain text is not a regex and keeps the case-insensitive contains behaviour
// flows already rely on — the change is scoped to patterns someone wrote
// deliberately as a regex.
func TestMatchesTextKeepsPlainTextCaseInsensitive(t *testing.T) {
	if !matchesText("sign out", "Sign Out") {
		t.Error("plain text should still match case-insensitively")
	}
	if !matchesText("SIGN", "Sign out") {
		t.Error("plain text should still match as a substring")
	}
}

// Case-sensitivity applies to the pattern as written; a regex asking for
// insensitivity can still say so.
func TestMatchesTextAllowsExplicitInsensitivity(t *testing.T) {
	if !matchesText("(?i)^sign out$", "SIGN OUT") {
		t.Error("an explicit (?i) should still be honoured")
	}
}
