package devicelab

import "testing"

func TestVisiblePageIndex(t *testing.T) {
	for name, tc := range map[string]struct {
		visible []bool
		want    int
		asked   int
	}{
		"one page is not asked":         {visible: []bool{false}, want: 0, asked: 0},
		"hidden first, visible second":  {visible: []bool{false, true, true}, want: 1, asked: 2},
		"visible first":                 {visible: []bool{true, false}, want: 0, asked: 1},
		"none visible falls back first": {visible: []bool{false, false}, want: 0, asked: 2},
	} {
		asked := 0
		got := visiblePageIndex(len(tc.visible), func(i int) bool { asked++; return tc.visible[i] })
		if got != tc.want || asked != tc.asked {
			t.Errorf("%s: picked %d after %d checks, want %d after %d", name, got, asked, tc.want, tc.asked)
		}
	}
}
