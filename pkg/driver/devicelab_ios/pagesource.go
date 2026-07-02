package devicelab_ios

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// findInSnapshot resolves a Maestro selector against a flat node list from
// the runner. Returns the best match (or all candidates for index= queries).
// Selector semantics mirror the existing wda driver: substring/regex on
// text+label+value+placeholder, exact-or-regex on id (accessibility
// identifier), state filters, width/height tolerance.
func findInSnapshot(nodes []SnapshotNode, sel flow.Selector) []*SnapshotNode { //nolint:unused
	if len(nodes) == 0 {
		return nil
	}
	var hits []*SnapshotNode
	for i := range nodes {
		n := &nodes[i]
		if matchesSelector(n, sel) {
			hits = append(hits, n)
		}
	}
	return hits
}

func matchesSelector(n *SnapshotNode, sel flow.Selector) bool {
	// State filters first — cheap.
	if sel.Enabled != nil && n.Enabled != *sel.Enabled {
		return false
	}
	if sel.Selected != nil && n.Selected != *sel.Selected {
		return false
	}
	if sel.Focused != nil && n.Focused != *sel.Focused {
		return false
	}

	// Text matches against label, value, or placeholder. Empty selector
	// text means "no text constraint".
	if sel.Text != "" && !matchesText(sel.Text, n.Label, n.Value, n.PlaceholderValue) {
		return false
	}

	// ID matches against accessibility identifier.
	if sel.ID != "" && !matchesID(sel.ID, n.Identifier) {
		return false
	}

	// Size matching (with tolerance) — used by graphical assertions.
	if sel.Width > 0 || sel.Height > 0 {
		if !withinTolerance(int(n.Rect.Width), sel.Width, sel.Tolerance) {
			return false
		}
		if !withinTolerance(int(n.Rect.Height), sel.Height, sel.Tolerance) {
			return false
		}
	}

	return true
}

func matchesID(pattern, id string) bool {
	if pattern == "" {
		return true
	}
	if pattern == id {
		return true
	}
	if looksLikeRegex(pattern) {
		if re, err := regexp.Compile(pattern); err == nil {
			return re.MatchString(id)
		}
	}
	// Fall back to substring match (lenient — matches Maestro/WDA).
	return containsIgnoreCase(id, pattern)
}

// matchesText returns true if `pattern` matches any of the given texts.
// Empty texts are skipped. Pattern may be:
//   - exact (case-insensitive)
//   - regex (compiled if it looks regex-ish — see looksLikeRegex)
//   - substring (case-insensitive) as a last resort
func matchesText(pattern string, texts ...string) bool {
	if pattern == "" {
		return true
	}
	patternLower := strings.ToLower(pattern)

	// Try regex first if pattern looks regex-ish.
	if looksLikeRegex(pattern) {
		if re, err := regexp.Compile(pattern); err == nil {
			for _, t := range texts {
				if t != "" && re.MatchString(t) {
					return true
				}
			}
			// Fall through if no regex hit — try substring.
		}
	}

	for _, t := range texts {
		if t == "" {
			continue
		}
		tLower := strings.ToLower(t)
		if tLower == patternLower {
			return true
		}
		if strings.Contains(tLower, patternLower) {
			return true
		}
	}
	return false
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func withinTolerance(actual, expected, tolerance int) bool {
	if expected == 0 {
		return true
	}
	delta := actual - expected
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}

// looksLikeRegex returns true if `text` contains characters that suggest
// the caller meant it as a regex pattern. Matches WDA's heuristic.
func looksLikeRegex(text string) bool {
	for _, c := range text {
		switch c {
		case '*', '+', '?', '^', '$', '\\', '(', ')', '[', ']', '{', '}', '|':
			return true
		}
	}
	return false
}

// toElementInfo converts a SnapshotNode to core.ElementInfo for return to
// the flow runner. Bounds use ints — XCUI gives us floats but Maestro
// selectors expect ints. We round to nearest.
func toElementInfo(n *SnapshotNode) *core.ElementInfo {
	if n == nil {
		return nil
	}
	return &core.ElementInfo{
		ID:                 n.Identifier,
		Text:               firstNonEmpty(n.Value, n.Label, n.PlaceholderValue),
		Class:              n.Type,
		AccessibilityLabel: n.Label,
		Bounds: core.Bounds{
			X:      int(round(n.Rect.X)),
			Y:      int(round(n.Rect.Y)),
			Width:  int(round(n.Rect.Width)),
			Height: int(round(n.Rect.Height)),
		},
		Visible:  isDisplayed(n),
		Enabled:  n.Enabled,
		Focused:  n.Focused,
		Selected: n.Selected,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func round(f float64) float64 {
	if f >= 0 {
		return float64(int(f + 0.5))
	}
	return float64(int(f - 0.5))
}

// selectByIndex picks one element from a multi-match using selector.Index.
// Supports numeric indices (0, 1, …) and the special string "0" for first.
func selectByIndex(candidates []*SnapshotNode, index string) *SnapshotNode {
	if len(candidates) == 0 {
		return nil
	}
	if index == "" {
		return candidates[0]
	}
	i, err := strconv.Atoi(index)
	if err != nil || i < 0 || i >= len(candidates) {
		return candidates[0]
	}
	return candidates[i]
}
