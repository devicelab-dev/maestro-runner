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
