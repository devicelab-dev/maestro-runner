package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// TestBuildSelectors_IDAndTextAreANDed is the correctness guarantee behind the
// Android half of #130: a selector naming both an id and a text must match one
// element carrying both.
//
// Before this, the builder emitted resourceId-only strategies and then
// text-only ones as separate candidates, and tryFindElementFast returns on the
// first that finds anything — so the id matched, the text was never read, and a
// wrong text: passed green against the right element.
func TestBuildSelectors_IDAndTextAreANDed(t *testing.T) {
	strategies, err := buildSelectors(flow.Selector{ID: "boss.hp", Text: "7 misses"}, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}
	if len(strategies) == 0 {
		t.Fatal("expected at least one strategy")
	}

	for _, s := range strategies {
		hasID := strings.Contains(s.Value, "resourceId")
		hasText := strings.Contains(s.Value, "text") ||
			strings.Contains(s.Value, "description") ||
			strings.Contains(s.Value, "hint") ||
			strings.Contains(s.Value, "error")
		if !hasID || !hasText {
			t.Errorf("every strategy must constrain both id and text, got: %s", s.Value)
		}
	}
}

// The single-attribute paths constrain only their own attribute.
func TestBuildSelectors_SingleAttributeUnchanged(t *testing.T) {
	idOnly, err := buildSelectors(flow.Selector{ID: "boss.hp"}, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}
	// Exact resourceId, then the whole-id match with or without the prefix.
	if len(idOnly) != 2 {
		t.Errorf("expected 2 id strategies, got %d", len(idOnly))
	}
	for _, s := range idOnly {
		if strings.Contains(s.Value, "textContains") || strings.Contains(s.Value, "textMatches") {
			t.Errorf("id-only selector must not constrain text, got: %s", s.Value)
		}
	}

	textOnly, err := buildSelectors(flow.Selector{Text: "7 misses"}, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}
	// text/description equal to it, then text/description/hint/error
	// matching it whole ignoring case. Plain text has no own-case regex tier:
	// it would repeat the first.
	if len(textOnly) != 6 {
		t.Errorf("expected 6 text strategies, got %d", len(textOnly))
	}
	for _, s := range textOnly {
		if strings.Contains(s.Value, "resourceId") {
			t.Errorf("text-only selector must not constrain id, got: %s", s.Value)
		}
	}
}

// A tap prefers a clickable element, and that preference has to survive the
// combination rather than being dropped along with the separate strategies.
func TestBuildSelectorsForTap_CombinedKeepsClickableFirst(t *testing.T) {
	strategies, err := buildSelectorsForTap(flow.Selector{ID: "boss.hp", Text: "7 misses"}, 5000)
	if err != nil {
		t.Fatalf("buildSelectorsForTap failed: %v", err)
	}
	if len(strategies) == 0 {
		t.Fatal("expected at least one strategy")
	}
	if want := `new UiSelector().resourceId("boss.hp").text("7 misses").clickable(true)`; strategies[0].Value != want {
		t.Errorf("first strategy = %s, want %s", strategies[0].Value, want)
	}
}

// Every Android query matches the whole text, description or hint, as
// Maestro does (#188): `tapOn: Open` must not reach a "Talk · Open" row
// through textContains, and a tap is no longer reordered to put whole-text
// strategies ahead of substring ones, as there are none.
func TestTextStrategiesMatchWhole(t *testing.T) {
	for _, text := range []string{"Open", "English", "DDG.", "Username"} {
		clickable, err := buildClickableOnlyStrategies(flow.Selector{Text: text})
		if err != nil {
			t.Fatal(err)
		}
		all, err := buildSelectors(flow.Selector{Text: text}, 5000)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range append(clickable, all...) {
			if strings.Contains(s.Value, "Contains(") || strings.Contains(s.Value, ".*") || strings.Contains(s.Value, `\Q`) {
				t.Errorf("%q got a partial strategy: %s", text, s.Value)
			}
		}
	}

	clickable, _ := buildClickableOnlyStrategies(flow.Selector{Text: "Username"})
	if want := `new UiSelector().text("Username").clickable(true)`; clickable[0].Value != want {
		t.Errorf("first clickable strategy = %s, want %s", clickable[0].Value, want)
	}
	last := clickable[len(clickable)-1].Value
	if want := `new UiSelector().errorMatches("(?ims)(?:Username)").clickable(true)`; last != want {
		t.Errorf("last clickable strategy = %s, want %s", last, want)
	}

	// A partial match written as a regex passes through, whole-matched.
	regex, _ := buildSelectors(flow.Selector{Text: ".*Open.*"}, 5000)
	found := false
	for _, s := range regex {
		if s.Value == `new UiSelector().textMatches("(?ims)(?:.*Open.*)")` {
			found = true
		}
	}
	if !found {
		t.Errorf("no case-insensitive regex strategy for .*Open.* in %v", regex)
	}
}

// A dotted text selector is a regex to Maestro: its dot matches any
// character, and in its own case first (#151).
func TestDottedTextStrategies(t *testing.T) {
	clickable, err := buildClickableOnlyStrategies(flow.Selector{Text: "DDG."})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`new UiSelector().text("DDG.").clickable(true)`,
		`new UiSelector().description("DDG.").clickable(true)`,
		`new UiSelector().textMatches("(?ms)(?:DDG.)").clickable(true)`,
		`new UiSelector().descriptionMatches("(?ms)(?:DDG.)").clickable(true)`,
		`new UiSelector().hintMatches("(?ms)(?:DDG.)").clickable(true)`,
		`new UiSelector().textMatches("(?ims)(?:DDG.)").clickable(true)`,
		`new UiSelector().descriptionMatches("(?ims)(?:DDG.)").clickable(true)`,
		`new UiSelector().hintMatches("(?ims)(?:DDG.)").clickable(true)`,
		`new UiSelector().errorMatches("(?ims)(?:DDG.)").clickable(true)`,
	}
	if len(clickable) != len(want) {
		t.Fatalf("clickable strategies = %d, want %d: %v", len(clickable), len(want), clickable)
	}
	for i, s := range clickable {
		if s.Value != want[i] {
			t.Errorf("strategy %d = %s, want %s", i, s.Value, want[i])
		}
	}
}

// An id matches whole (#188): `id: login` must not find "login_button".
func TestIDStrategiesMatchWhole(t *testing.T) {
	strategies, _ := buildSelectors(flow.Selector{ID: "login"}, 5000)
	for _, s := range strategies {
		if strings.Contains(s.Value, ".*(?:login).*") {
			t.Errorf("id got a substring strategy: %s", s.Value)
		}
	}
	if want := `new UiSelector().resourceIdMatches("(?ims)(?:.*/)?(?:login)")`; strategies[len(strategies)-1].Value != want {
		t.Errorf("last id strategy = %s, want %s", strategies[len(strategies)-1].Value, want)
	}
}
