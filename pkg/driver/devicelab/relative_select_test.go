package devicelab

import "testing"

func elem(text string, clickable bool, depth int) *ParsedElement {
	return &ParsedElement{Text: text, Clickable: clickable, Depth: depth}
}

// A directional selector means "the one nearest the anchor". Candidates arrive
// sorted by distance, so the deepest-node tiebreak SelectByIndex applies would
// return something further away that merely sits lower in the tree.
func TestSelectRelativeCandidatePrefersNearestForDirectionalFilters(t *testing.T) {
	near := elem("near", false, 1)
	farButDeep := elem("far-but-deep", false, 9)
	candidates := []*ParsedElement{near, farButDeep}

	for _, filter := range []relativeFilterType{filterBelow, filterAbove, filterLeftOf, filterRightOf} {
		if got := selectRelativeCandidate(candidates, "", filter); got != near {
			t.Errorf("filter %v selected %q, want the nearest candidate", filter, got.Text)
		}
	}
}

// Non-directional filters are not distance-ordered, so the previous
// deepest-match behaviour is still the useful one.
func TestSelectRelativeCandidateKeepsDepthTiebreakElsewhere(t *testing.T) {
	shallow := elem("shallow", false, 1)
	deep := elem("deep", false, 9)

	if got := selectRelativeCandidate([]*ParsedElement{shallow, deep}, "", filterContainsChild); got != deep {
		t.Errorf("selected %q, want the deepest match for a non-directional filter", got.Text)
	}
}

// An explicit index always wins — it is the author saying exactly which one.
func TestSelectRelativeCandidateHonoursExplicitIndex(t *testing.T) {
	first := elem("first", false, 1)
	second := elem("second", false, 1)

	if got := selectRelativeCandidate([]*ParsedElement{first, second}, "1", filterBelow); got != second {
		t.Errorf("selected %q, want the indexed candidate", got.Text)
	}
}
