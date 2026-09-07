// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package flutter

import (
	"regexp"
	"strings"
)

// WidgetTreeMatch holds cross-reference information found by searching the widget tree.
// It contains enough context to find the corresponding semantics node for coordinates.
type WidgetTreeMatch struct {
	// How the text/identifier was found in the widget tree.
	MatchType string // "hintText", "identifier"

	// For hintText matches: the labelText from the same InputDecoration.
	LabelText string

	// Text content found near the match (e.g., child Text widget content).
	NearbyText string

	// Whether the match is within a TextField/InputDecoration context.
	IsTextField bool

	// Whether the match is a suffix/prefix inside an InputDecoration.
	// When true, tap coordinates should target the right edge of the TextField.
	IsSuffix bool
}

var (
	reWTHintText  = regexp.MustCompile(`hintText:\s*"([^"]*)"`)
	reWTLabelText = regexp.MustCompile(`labelText:\s*"([^"]*)"`)
	reWTIdentProp = regexp.MustCompile(`identifier:\s*"([^"]*)"`)
	reWTText      = regexp.MustCompile(`Text\("([^"]*)"`) // Text("content"...) - may have extra params
	reWTTooltip   = regexp.MustCompile(`tooltip:\s*"([^"]*)"`)
	reWTSuffix    = regexp.MustCompile(`suffix(?:Icon)?:\s*Semantics`)
	reWhitespace  = regexp.MustCompile(`\s+`)
)

// normalizeWhitespace collapses all whitespace sequences (including newlines)
// into single spaces. This is needed because the semantics dump wraps long text
// across lines, inserting newlines that don't exist in the original text.
func normalizeWhitespace(s string) string {
	return reWhitespace.ReplaceAllString(strings.TrimSpace(s), " ")
}

// containsNormalized checks if haystack contains needle after normalizing
// whitespace in both strings.
func containsNormalized(haystack, needle string) bool {
	return strings.Contains(normalizeWhitespace(haystack), normalizeWhitespace(needle))
}

// SearchWidgetTreeForText searches the widget tree dump for text that might be
// a hintText or other property not exposed in the semantics tree.
func SearchWidgetTreeForText(dump string, searchText string) *WidgetTreeMatch {
	if searchText == "" || dump == "" {
		return nil
	}

	// Strategy 1: Check if searchText is a hintText value
	for _, idx := range reWTHintText.FindAllStringSubmatchIndex(dump, -1) {
		hintText := dump[idx[2]:idx[3]]
		if hintText != searchText && !strings.Contains(hintText, searchText) {
			continue
		}

		// Found as hintText — find the associated labelText nearby.
		// Look for the closest labelText (last one before hintText, or first after).
		start := max(0, idx[0]-500)
		end := min(len(dump), idx[1]+500)

		// First try: find the last labelText BEFORE the hintText (same InputDecoration)
		beforeCtx := dump[start:idx[0]]
		if matches := reWTLabelText.FindAllStringSubmatch(beforeCtx, -1); len(matches) > 0 {
			return &WidgetTreeMatch{
				MatchType:   "hintText",
				LabelText:   matches[len(matches)-1][1], // last (closest) match
				IsTextField: true,
			}
		}

		// Fallback: find the first labelText AFTER the hintText
		afterCtx := dump[idx[1]:end]
		if m := reWTLabelText.FindStringSubmatch(afterCtx); m != nil {
			return &WidgetTreeMatch{
				MatchType:   "hintText",
				LabelText:   m[1],
				IsTextField: true,
			}
		}

		// hintText found but no labelText nearby — still useful
		return &WidgetTreeMatch{
			MatchType:   "hintText",
			IsTextField: true,
		}
	}

	return nil
}

// SearchWidgetTreeForID searches the widget tree dump for a widget with the
// given identifier and returns cross-reference information.
func SearchWidgetTreeForID(dump string, searchID string) *WidgetTreeMatch {
	if searchID == "" || dump == "" {
		return nil
	}

	// Look for identifier: "searchID" in the widget tree
	for _, idx := range reWTIdentProp.FindAllStringSubmatchIndex(dump, -1) {
		id := dump[idx[2]:idx[3]]
		if id != searchID {
			continue
		}

		// Found the identifier. Extract the entire subtree below it using
		// indentation tracking — this is more robust than a fixed-char window
		// since Material widget descriptions can consume 2000+ chars.
		afterContext := extractSubtree(dump, idx[1])

		beforeStart := max(0, idx[0]-500)
		beforeContext := dump[beforeStart:idx[0]]

		// Check if this identifier is inside a suffix:/suffixIcon: context
		isSuffix := reWTSuffix.MatchString(beforeContext)

		// Look forward for Text("...") child widget in the subtree
		if m := reWTText.FindStringSubmatch(afterContext); m != nil {
			return &WidgetTreeMatch{
				MatchType:  "identifier",
				NearbyText: m[1],
				IsSuffix:   isSuffix,
			}
		}

		// Look forward for tooltip in the subtree
		if m := reWTTooltip.FindStringSubmatch(afterContext); m != nil {
			return &WidgetTreeMatch{
				MatchType:  "identifier",
				NearbyText: m[1],
				IsSuffix:   isSuffix,
			}
		}

		// Look backward for labelText (e.g., suffix icon inside TextField)
		if matches := reWTLabelText.FindAllStringSubmatch(beforeContext, -1); len(matches) > 0 {
			return &WidgetTreeMatch{
				MatchType:   "identifier",
				LabelText:   matches[len(matches)-1][1], // last (closest) match
				IsTextField: true,
				IsSuffix:    isSuffix,
			}
		}

		// Identifier found but no nearby text — element exists but can't cross-reference
		return &WidgetTreeMatch{
			MatchType: "identifier",
			IsSuffix:  isSuffix,
		}
	}

	return nil
}

// extractSubtree extracts the widget subtree text starting from pos.
// Uses indentation tracking: finds the indentation level of the line containing pos,
// then scans forward collecting all lines at deeper indentation (child widgets).
// Stops when a line at the same or shallower indentation is found (sibling/parent).
func extractSubtree(dump string, pos int) string {
	// Find the start of the line containing pos
	lineStart := strings.LastIndex(dump[:pos], "\n") + 1
	identLine := dump[lineStart:]
	if nlIdx := strings.Index(identLine, "\n"); nlIdx >= 0 {
		identLine = identLine[:nlIdx]
	}

	// Get the indentation level of the identifier's widget line
	baseIndent := widgetIndent(identLine)

	// Scan forward from the end of this line
	endOfLine := strings.Index(dump[pos:], "\n")
	if endOfLine < 0 {
		return dump[pos:]
	}

	scanStart := pos + endOfLine + 1
	subtreeEnd := scanStart

	for i := scanStart; i < len(dump); {
		nlIdx := strings.Index(dump[i:], "\n")
		if nlIdx < 0 {
			subtreeEnd = len(dump)
			break
		}
		line := dump[i : i+nlIdx]

		// Skip empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i += nlIdx + 1
			subtreeEnd = i
			continue
		}

		// If this line's indentation is at or shallower than our identifier,
		// we've left the subtree — stop.
		lineIndent := widgetIndent(line)
		if lineIndent <= baseIndent {
			break
		}

		subtreeEnd = i + nlIdx + 1
		i = subtreeEnd
	}

	return dump[pos:subtreeEnd]
}

// widgetIndent returns the indentation level of a widget tree line.
// Counts leading box-drawing characters (│├└─╎╏┊┆) and spaces.
func widgetIndent(line string) int {
	for i, ch := range line {
		if ch != ' ' && ch != '│' && ch != '├' && ch != '└' && ch != '─' &&
			ch != '╎' && ch != '╏' && ch != '┊' && ch != '┆' && ch != '║' {
			return i
		}
	}
	return len(line)
}

// CrossReferenceWithSemantics finds semantics nodes that correspond to the
// widget tree match, providing coordinates for the element.
func (m *WidgetTreeMatch) CrossReferenceWithSemantics(root *SemanticsNode) []*SemanticsNode {
	if m == nil || root == nil {
		return nil
	}

	switch m.MatchType {
	case "hintText":
		// The hintText belongs to a TextField. Find the semantics TextField
		// node with a matching label (the labelText).
		if m.LabelText != "" {
			var nodes []*SemanticsNode
			walkTree(root, func(n *SemanticsNode) {
				if HasFlag(n, "isTextField") && n.Label != "" && containsNormalized(n.Label, m.LabelText) {
					nodes = append(nodes, n)
				}
			})
			if len(nodes) > 0 {
				return nodes
			}
		}
		// Fallback: return any TextField node (better than nothing)
		var textFields []*SemanticsNode
		walkTree(root, func(n *SemanticsNode) {
			if HasFlag(n, "isTextField") {
				textFields = append(textFields, n)
			}
		})
		return textFields

	case "identifier":
		// The identifier exists in widget tree but was absorbed in semantics.
		// Try to find a semantics node whose label contains the nearby text.
		if m.NearbyText != "" {
			// Use normalized matching — semantics dump may wrap text across lines
			var nodes []*SemanticsNode
			walkTree(root, func(n *SemanticsNode) {
				if n.Label != "" && containsNormalized(n.Label, m.NearbyText) {
					nodes = append(nodes, n)
				}
			})
			if len(nodes) > 0 {
				return nodes
			}
		}
		// If it's a TextField context (e.g., suffix icon), find the TextField
		if m.IsTextField && m.LabelText != "" {
			var nodes []*SemanticsNode
			walkTree(root, func(n *SemanticsNode) {
				if HasFlag(n, "isTextField") && n.Label != "" && containsNormalized(n.Label, m.LabelText) {
					nodes = append(nodes, n)
				}
			})
			return nodes
		}
	}

	return nil
}
