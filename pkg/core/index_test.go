package core

import (
	"strings"
	"testing"
)

// An index past the matches names no element (#188): `index: 1` with one match
// is out of range, where it used to fall back to the first match.
func TestIndexOutOfRange(t *testing.T) {
	for _, tc := range []struct {
		count int
		index string
		want  bool
	}{
		{1, "0", false},
		{1, "1", true}, // "there is no second row"
		{3, "2", false},
		{3, "3", true},
		{3, "-1", false}, // last
		{3, "-3", false}, // first
		{3, "-4", true},
		{0, "0", true},
		{2, "", false},    // no index: unchanged handling
		{2, "abc", false}, // not a number: unchanged handling
		{2, " 1 ", false},
	} {
		if got := IndexOutOfRange(tc.count, tc.index); got != tc.want {
			t.Errorf("IndexOutOfRange(%d, %q) = %v, want %v", tc.count, tc.index, got, tc.want)
		}
	}
}

// An index that is not a number fails, as in Maestro, instead of meaning the
// first match: `index: ${ROW}` with ROW unset expands to "undefined".
func TestIndexError(t *testing.T) {
	for _, tc := range []struct {
		count int
		index string
		want  string // "" = no error
	}{
		{3, "", ""}, {3, "0", ""}, {3, "2", ""}, {3, "-1", ""}, {3, " 1 ", ""}, {3, "2.0", ""},
		{3, "3", "no element at index 3: 3 match(es)"},
		{3, "-4", "no element at index -4: 3 match(es)"},
		{3, "undefined", `invalid index "undefined": index must be a whole number`},
		{3, "abc", `invalid index "abc"`},
		{3, "NaN", `invalid index "NaN"`},
		{3, "Infinity", `invalid index "Infinity"`},
	} {
		err := IndexError(tc.count, tc.index)
		if tc.want == "" {
			if err != nil {
				t.Errorf("IndexError(%d, %q) = %v, want nil", tc.count, tc.index, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("IndexError(%d, %q) = %v, want %q", tc.count, tc.index, err, tc.want)
		}
	}
	if i, err := ParseIndex("2.9"); err != nil || i != 2 {
		t.Errorf("ParseIndex(2.9) = %d, %v; want 2 (truncated, as Maestro)", i, err)
	}
}

// An anchor's index counts the anchors by their place on screen, as Maestro
// does, not in tree order and not clickable first: the cards here are listed
// out of screen order, and only card B is clickable.
func TestPickAnchor(t *testing.T) {
	type card struct {
		name      string
		clickable bool
		b         Bounds
	}
	cards := []card{
		{"C", false, Bounds{X: 0, Y: 500, Width: 400, Height: 200}},
		{"B", true, Bounds{X: 420, Y: 100, Width: 400, Height: 200}},
		{"A", false, Bounds{X: 0, Y: 100, Width: 400, Height: 200}},
	}
	bounds := func(c card) Bounds { return c.b }

	for index, want := range map[string]string{"0": "A", "1": "B", "2": "C", "-1": "C", "1.0": "B"} {
		got, err := PickAnchor(cards, index, bounds)
		if err != nil || len(got) != 1 || got[0].name != want {
			t.Errorf("PickAnchor(index %s) = %v, %v; want card %s", index, got, err, want)
		}
	}
	if got, err := PickAnchor(cards, "", bounds); err != nil || len(got) != 3 || got[0].name != "C" {
		t.Errorf("PickAnchor without an index = %v, %v; want every anchor as found", got, err)
	}
	for index, want := range map[string]string{
		"3":         "anchor element not found: no element at index 3: 3 match(es)",
		"undefined": `anchor element not found: invalid index "undefined"`,
	} {
		if _, err := PickAnchor(cards, index, bounds); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("PickAnchor(index %s) error = %v, want %q", index, err, want)
		}
	}
}
