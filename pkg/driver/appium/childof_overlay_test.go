package appium

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// A list row behind an overlay that covers it is inside the overlay's bounds but not its child.
// childOf must follow the element tree (as Maestro does), or an assertVisible scoped to the empty
// overlay list passes on the rows behind it.
func TestFilterChildOfIgnoresElementsBehindAnOverlay(t *testing.T) {
	full := core.Bounds{X: 0, Y: 300, Width: 1080, Height: 1700}
	root := &ParsedElement{ResourceID: "root", Bounds: full}
	list := &ParsedElement{ResourceID: "item_list", Bounds: full, Parent: root}
	row := &ParsedElement{ResourceID: "item_title", Text: "Alice",
		Bounds: core.Bounds{X: 240, Y: 480, Width: 800, Height: 60}, Parent: list}
	overlay := &ParsedElement{ResourceID: "search_results", Bounds: full, Parent: root}
	list.Children = []*ParsedElement{row}
	root.Children = []*ParsedElement{list, overlay}

	if got := FilterChildOf([]*ParsedElement{row}, overlay); len(got) != 0 {
		t.Fatalf("row behind the overlay counted as its child: %v", got)
	}
	if got := FilterChildOf([]*ParsedElement{row}, list); len(got) != 1 {
		t.Fatalf("row not found in its own list: %v", got)
	}
	if got := FilterContainsChild([]*ParsedElement{list, overlay}, row); len(got) != 1 || got[0] != list {
		t.Fatalf("containsChild should match only the row's list: %v", got)
	}
}
