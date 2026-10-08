package wda

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// A list cell behind an overlay that covers it is inside the overlay's bounds but not its child.
// childOf must follow the element tree (as Maestro does), or an assertVisible scoped to the empty
// overlay list passes on the cells behind it.
func TestFilterChildOfIgnoresElementsBehindAnOverlay(t *testing.T) {
	full := core.Bounds{X: 0, Y: 100, Width: 390, Height: 700}
	root := &ParsedElement{Name: "root", Bounds: full}
	list := &ParsedElement{Name: "item_list", Bounds: full, Parent: root}
	row := &ParsedElement{Name: "item_title", Label: "Alice",
		Bounds: core.Bounds{X: 80, Y: 160, Width: 280, Height: 22}, Parent: list}
	overlay := &ParsedElement{Name: "search_results", Bounds: full, Parent: root}
	list.Children = []*ParsedElement{row}
	root.Children = []*ParsedElement{list, overlay}

	if got := FilterChildOf([]*ParsedElement{row}, overlay); len(got) != 0 {
		t.Fatalf("cell behind the overlay counted as its child: %v", got)
	}
	if got := FilterChildOf([]*ParsedElement{row}, list); len(got) != 1 {
		t.Fatalf("cell not found in its own list: %v", got)
	}
	if got := FilterContainsChild([]*ParsedElement{list, overlay}, row); len(got) != 1 || got[0] != list {
		t.Fatalf("containsChild should match only the cell's list: %v", got)
	}
}
