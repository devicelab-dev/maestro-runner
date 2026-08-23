package devicelab_ios

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// TestPreferExactID verifies that when an id-selector matches both an exact
// node and a substring superset, only the exact match is kept (#128), while
// regex ids and no-exact-match cases keep the lenient set.
func TestPreferExactID(t *testing.T) {
	button := SnapshotNode{Identifier: "set-enriched-text-button"}
	exact := SnapshotNode{Identifier: "enriched-text"}
	hits := []SnapshotNode{button, exact} // substring match ordered first

	t.Run("exact wins over substring superset", func(t *testing.T) {
		got := preferExactID(hits, flow.Selector{ID: "enriched-text"})
		if len(got) != 1 || got[0].Identifier != "enriched-text" {
			t.Errorf("got %v, want only exact 'enriched-text'", ids(got))
		}
	})

	t.Run("no exact match keeps lenient set", func(t *testing.T) {
		got := preferExactID([]SnapshotNode{button}, flow.Selector{ID: "enriched-text"})
		if len(got) != 1 || got[0].Identifier != "set-enriched-text-button" {
			t.Errorf("got %v, want substring fallback kept", ids(got))
		}
	})

	t.Run("regex id is untouched", func(t *testing.T) {
		got := preferExactID(hits, flow.Selector{ID: "enriched-.*"})
		if len(got) != 2 {
			t.Errorf("regex id should keep all matches, got %v", ids(got))
		}
	})

	t.Run("single hit untouched", func(t *testing.T) {
		got := preferExactID([]SnapshotNode{exact}, flow.Selector{ID: "enriched-text"})
		if len(got) != 1 {
			t.Errorf("single hit should be unchanged, got %v", ids(got))
		}
	})
}

func ids(ns []SnapshotNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Identifier
	}
	return out
}
