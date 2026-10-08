package appium

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ParsedElement represents an element from page source XML.
// Handles both iOS and Android formats.
type ParsedElement struct {
	// Common
	Bounds    core.Bounds
	Enabled   bool
	Displayed bool
	Selected  bool
	Checked   bool
	Focused   bool
	Clickable bool
	// Scrollable marks a scroll container. Android reports a child's bounds
	// already clipped to it, so a sliver at the container's leading edge
	// looks fully visible; scrollUntilVisible walks up to the nearest
	// scrollable ancestor to notice (#164).
	Scrollable bool
	Depth      int
	Children   []*ParsedElement
	Parent     *ParsedElement // parent element for clickable lookup

	// Android
	Text        string
	ResourceID  string
	ContentDesc string
	HintText    string
	// ErrorText is the field's validation error (AccessibilityNodeInfo.getError),
	// the `error` attribute the uiautomator2 server writes for every node —
	// empty for most. A Compose field whose only content is its error
	// semantics is found by that text, as in Maestro since 2.9.
	ErrorText string
	ClassName string

	// iOS
	Type             string // XCUIElementType
	Name             string // accessibility identifier
	Label            string // accessibility label
	Value            string // current value
	PlaceholderValue string
}

// ParsePageSource parses page source XML into elements.
// Auto-detects iOS vs Android format.
func ParsePageSource(xmlData string) ([]*ParsedElement, string, error) {
	// Detect platform by checking for iOS-specific markers
	isIOS := strings.Contains(xmlData, "XCUIElementType") ||
		strings.Contains(xmlData, "AppiumAUT")

	if isIOS {
		elements, err := parseIOSPageSource(xmlData)
		return elements, "ios", err
	}
	elements, err := parseAndroidPageSource(xmlData)
	return elements, "android", err
}

// parseAndroidPageSource parses Android UI hierarchy XML.
func parseAndroidPageSource(xmlData string) ([]*ParsedElement, error) {
	decoder := xml.NewDecoder(strings.NewReader(xmlData))

	var elements []*ParsedElement
	foundHierarchy := false
	var parseElement func() (*ParsedElement, error)

	parseElement = func() (*ParsedElement, error) {
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}

			switch t := token.(type) {
			case xml.StartElement:
				if t.Name.Local == "hierarchy" {
					foundHierarchy = true
					continue
				}

				elem := &ParsedElement{
					ClassName: t.Name.Local,
				}

				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "text":
						elem.Text = attr.Value
					case "resource-id":
						elem.ResourceID = attr.Value
					case "content-desc":
						elem.ContentDesc = attr.Value
					case "hint":
						elem.HintText = attr.Value
					case "error":
						elem.ErrorText = attr.Value
					case "class":
						elem.ClassName = attr.Value
					case "bounds":
						elem.Bounds = parseBounds(attr.Value)
					case "enabled":
						elem.Enabled = attr.Value == "true"
					case "selected":
						elem.Selected = attr.Value == "true"
					case "checked":
						elem.Checked = attr.Value == "true"
					case "focused":
						elem.Focused = attr.Value == "true"
					case "displayed":
						elem.Displayed = attr.Value != "false"
					case "clickable":
						elem.Clickable = attr.Value == "true"
					case "scrollable":
						elem.Scrollable = attr.Value == "true"
					}
				}

				// Parse children
				for {
					child, err := parseElement()
					if err != nil || child == nil {
						break
					}
					elem.Children = append(elem.Children, child)
				}

				return elem, nil

			case xml.EndElement:
				return nil, nil
			}
		}
	}

	// Parse all root elements
	var parseErr error
	for {
		elem, err := parseElement()
		if err != nil {
			if err.Error() != "EOF" {
				parseErr = err
			}
			break
		}
		if elem != nil {
			elements = append(elements, flattenElement(elem, 0)...)
		}
	}

	if parseErr != nil && len(elements) == 0 {
		return nil, parseErr
	}

	if !foundHierarchy {
		return nil, fmt.Errorf("invalid page source: no hierarchy element found")
	}

	return elements, nil
}

// parseIOSPageSource parses iOS UI hierarchy XML.
func parseIOSPageSource(xmlData string) ([]*ParsedElement, error) {
	decoder := xml.NewDecoder(strings.NewReader(xmlData))

	var elements []*ParsedElement
	var parseElement func() (*ParsedElement, error)

	parseElement = func() (*ParsedElement, error) {
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}

			switch t := token.(type) {
			case xml.StartElement:
				// Skip root element
				if t.Name.Local == "AppiumAUT" {
					for {
						child, err := parseElement()
						if err != nil || child == nil {
							break
						}
						elements = append(elements, flattenElement(child, 0)...)
					}
					continue
				}

				elem := &ParsedElement{
					Type:      t.Name.Local,
					Enabled:   true,
					Displayed: true,
				}

				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "type":
						elem.Type = attr.Value
					case "name":
						elem.Name = attr.Value
					case "label":
						elem.Label = attr.Value
					case "value":
						elem.Value = attr.Value
					case "enabled":
						elem.Enabled = attr.Value == "true"
					case "visible":
						elem.Displayed = attr.Value == "true"
					case "selected":
						elem.Selected = attr.Value == "true"
					case "focused":
						elem.Focused = attr.Value == "true"
					case "placeholderValue":
						elem.PlaceholderValue = attr.Value
					case "x":
						if v, err := strconv.Atoi(attr.Value); err == nil {
							elem.Bounds.X = v
						}
					case "y":
						if v, err := strconv.Atoi(attr.Value); err == nil {
							elem.Bounds.Y = v
						}
					case "width":
						if v, err := strconv.Atoi(attr.Value); err == nil {
							elem.Bounds.Width = v
						}
					case "height":
						if v, err := strconv.Atoi(attr.Value); err == nil {
							elem.Bounds.Height = v
						}
					}
				}

				// Parse children
				for {
					child, err := parseElement()
					if err != nil || child == nil {
						break
					}
					elem.Children = append(elem.Children, child)
				}

				return elem, nil

			case xml.EndElement:
				return nil, nil
			}
		}
	}

	// Parse root elements
	var parseErr error
	for {
		elem, err := parseElement()
		if err != nil {
			if err.Error() != "EOF" {
				parseErr = err
			}
			break
		}
		if elem != nil {
			elements = append(elements, flattenElement(elem, 0)...)
		}
	}

	if parseErr != nil && len(elements) == 0 {
		return nil, parseErr
	}

	if len(elements) == 0 {
		return nil, fmt.Errorf("no elements found in page source")
	}

	return elements, nil
}

// flattenElement flattens a tree of elements into a list, setting depth and parent.
func flattenElement(elem *ParsedElement, depth int) []*ParsedElement {
	elem.Depth = depth
	result := []*ParsedElement{elem}
	for _, child := range elem.Children {
		child.Parent = elem // Set parent reference
		result = append(result, flattenElement(child, depth+1)...)
	}
	return result
}

// parseBounds parses Android bounds string "[x1,y1][x2,y2]".
func parseBounds(s string) core.Bounds {
	s = strings.ReplaceAll(s, "][", ",")
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return core.Bounds{}
	}

	x1, _ := strconv.Atoi(parts[0])
	y1, _ := strconv.Atoi(parts[1])
	x2, _ := strconv.Atoi(parts[2])
	y2, _ := strconv.Atoi(parts[3])

	return core.Bounds{
		X:      x1,
		Y:      y1,
		Width:  x2 - x1,
		Height: y2 - y1,
	}
}

// FilterBySelector filters elements by selector properties.
func FilterBySelector(elements []*ParsedElement, sel flow.Selector, platform string) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if !matchesSelector(elem, sel, platform) {
			continue
		}
		result = append(result, elem)
	}

	result = preferExactText(result, sel, platform)
	return preferExactCase(result, sel.Text, func(e *ParsedElement) []string { return regexTextsOf(e, platform) })
}

// preferExactText narrows survivors to those whose text matches the literal
// pattern exactly, when any of them do.
//
// Added when literal text matched by contains, so `text: "0"` resolved to a
// price field reading "7000.00" ahead of the switch whose text is exactly "0"
// (#161). Text now matches whole, as in Maestro (#188), so "0" no longer
// matches "7000.00" at all; this still puts an exact value ahead of one a
// dotted or line-wrapped selector matches only as a regex.
//
// Applied AFTER the full selector has been satisfied, never instead of it. An
// exact text match that skipped the rest of the selector would return an
// element with the right text and the wrong id — the OR behaviour removed in
// #157/#158/#160. Reported by @nt-ben-leblond (#161).
func preferExactText(matches []*ParsedElement, sel flow.Selector, platform string) []*ParsedElement {
	if sel.Text == "" || looksLikeRegex(sel.Text) || len(matches) < 2 {
		return matches
	}
	var exact []*ParsedElement
	for _, elem := range matches {
		if platform == "ios" {
			if equalsAny(sel.Text, elem.Label, elem.Name, elem.Value, elem.PlaceholderValue) {
				exact = append(exact, elem)
			}
		} else if equalsAny(sel.Text, elem.Text, elem.ContentDesc, elem.HintText) {
			exact = append(exact, elem)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return matches
}

// equalsAny reports whether pattern equals any of the candidate strings,
// ignoring case as Maestro's text matching does.
func equalsAny(pattern string, candidates ...string) bool {
	if pattern == "" {
		return false
	}
	for _, c := range candidates {
		if strings.EqualFold(c, pattern) {
			return true
		}
	}
	return false
}

func matchesSelector(elem *ParsedElement, sel flow.Selector, platform string) bool {
	// Text matching
	if sel.Text != "" {
		if platform == "ios" {
			// Not elem.Name: on iOS that is the accessibility identifier,
			// which only id: matches (#178).
			if !matchesText(sel.Text, elem.Label, elem.Value, elem.PlaceholderValue) {
				return false
			}
		} else {
			if !matchesText(sel.Text, elem.Text, elem.ContentDesc, elem.HintText) && !matchesErrorText(sel.Text, elem.ErrorText) {
				return false
			}
		}
	}

	// ID matching (supports regex)
	if sel.ID != "" {
		if platform == "ios" {
			if !matchesID(sel.ID, elem.Name) {
				return false
			}
		} else {
			if !matchesID(sel.ID, elem.ResourceID) {
				return false
			}
		}
	}

	// Size matching
	if sel.Width > 0 || sel.Height > 0 {
		tolerance := sel.Tolerance
		if tolerance == 0 {
			tolerance = 5
		}
		if sel.Width > 0 && !withinTolerance(elem.Bounds.Width, sel.Width, tolerance) {
			return false
		}
		if sel.Height > 0 && !withinTolerance(elem.Bounds.Height, sel.Height, tolerance) {
			return false
		}
	}

	// State filters
	if sel.Enabled != nil && elem.Enabled != *sel.Enabled {
		return false
	}
	if sel.Selected != nil && elem.Selected != *sel.Selected {
		return false
	}
	if sel.Focused != nil && elem.Focused != *sel.Focused {
		return false
	}
	if sel.Checked != nil && elem.Checked != *sel.Checked {
		return false
	}

	return true
}

func withinTolerance(actual, expected, tolerance int) bool {
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// matchesID reports whether an id selector matches a resource id (Android) or
// accessibility identifier (iOS) as Maestro's idMatches does: a
// case-insensitive regex over the whole id, or over the part after a package
// prefix. `id: login` is not "com.app:id/login_button" (#188).
func matchesID(pattern, id string) bool {
	return core.MatchesIDMaestro(pattern, id)
}

// matchesText reports whether a text selector matches any of the texts as
// Maestro's textMatches does: the selector is a case-insensitive regex that
// must match a whole value, so "Open" is not "Talk · Open" and "English" is
// not "Language: English" (#188); a partial match is written `.*Open.*`. When
// several elements match, FilterBySelector puts the ones matching in the
// pattern's own case first (#151).
func matchesText(pattern string, texts ...string) bool {
	return core.MatchesTextMaestro(pattern, texts...)
}

// looksLikeRegex checks if text contains regex metacharacters.
// A standalone period (like in "mastodon.social") is NOT treated as regex.
func looksLikeRegex(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		// Check if it's escaped
		if i > 0 && text[i-1] == '\\' {
			// A backslash-escaped metacharacter is regex syntax (\. matches a
			// literal dot, \$ a literal $), so the whole pattern is a regex.
			// Classifying it as literal would match the backslash verbatim and
			// never hit an element whose text has no backslash (#136).
			switch c {
			case '.', '*', '+', '?', '[', ']', '{', '}', '|', '(', ')', '^', '$', '\\':
				return true
			}
			continue
		}
		switch c {
		case '.':
			// Only treat '.' as regex if followed by a quantifier (*, +, ?)
			// This allows "mastodon.social" to be treated as literal text
			if i+1 < len(text) {
				next := text[i+1]
				if next == '*' || next == '+' || next == '?' {
					return true
				}
			}
		case '*', '+', '?', '[', ']', '{', '}', '|', '(', ')':
			return true
		case '^':
			// ^ at start is common in regex, but at end it's likely literal
			if i == 0 {
				return true
			}
		case '$':
			// $ at end is common in regex (end anchor), but at start it's likely literal (currency)
			if i == len(text)-1 {
				return true
			}
		}
	}
	return false
}

// Position filter functions

// FilterBelow returns elements below the anchor.
func FilterBelow(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorBottom := anchor.Bounds.Y + anchor.Bounds.Height
	var result []*ParsedElement

	for _, elem := range elements {
		if elem.Bounds.Y >= anchorBottom {
			result = append(result, elem)
		}
	}

	sortByDistanceY(result, anchorBottom)
	return result
}

// FilterAbove returns elements above the anchor.
func FilterAbove(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorTop := anchor.Bounds.Y
	var result []*ParsedElement

	for _, elem := range elements {
		elemBottom := elem.Bounds.Y + elem.Bounds.Height
		if elemBottom <= anchorTop {
			result = append(result, elem)
		}
	}

	sortByDistanceYReverse(result, anchorTop)
	return result
}

// FilterLeftOf returns elements left of the anchor.
func FilterLeftOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorLeft := anchor.Bounds.X
	var result []*ParsedElement

	for _, elem := range elements {
		elemRight := elem.Bounds.X + elem.Bounds.Width
		if elemRight <= anchorLeft {
			result = append(result, elem)
		}
	}

	sortByDistanceXReverse(result, anchorLeft)
	return result
}

// FilterRightOf returns elements right of the anchor.
func FilterRightOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorRight := anchor.Bounds.X + anchor.Bounds.Width
	var result []*ParsedElement

	for _, elem := range elements {
		if elem.Bounds.X >= anchorRight {
			result = append(result, elem)
		}
	}

	sortByDistanceX(result, anchorRight)
	return result
}

// FilterChildOf returns elements that are descendants of anchor.
func FilterChildOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	// Element must be a descendant of anchor in the element tree (not merely inside its bounds).
	var result []*ParsedElement

	for _, elem := range elements {
		if isDescendantOf(elem, anchor) {
			result = append(result, elem)
		}
	}

	return result
}

// FilterContainsChild returns elements that are ancestors of anchor.
func FilterContainsChild(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	// Element must be an ancestor of anchor in the element tree (not merely enclose its bounds).
	var result []*ParsedElement

	for _, elem := range elements {
		if anchor.Parent == nil && len(anchor.Children) == 0 {
			if isInside(anchor.Bounds, elem.Bounds) {
				result = append(result, elem)
			}
			continue
		}
		if isDescendantOf(anchor, elem) {
			result = append(result, elem)
		}
	}

	return result
}

// isDescendantOf reports whether elem sits under anchor in the parsed element tree, which is what
// Maestro's childOf means. A position check alone also matches elements that merely lie inside the
// anchor's rectangle, such as list rows behind an overlay that covers them, and turns a failing
// assertion into a pass. Anchors rebuilt from a relative selector carry no tree links; for those,
// fall back to bounds containment.
func isDescendantOf(elem, anchor *ParsedElement) bool {
	if anchor.Parent == nil && len(anchor.Children) == 0 {
		return isInside(elem.Bounds, anchor.Bounds)
	}
	for p := elem.Parent; p != nil; p = p.Parent {
		if p == anchor {
			return true
		}
	}
	return false
}

// FilterInsideOf returns elements whose center is inside anchor.
func FilterInsideOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if elem.Bounds.CenterInside(anchor.Bounds) {
			result = append(result, elem)
		}
	}

	return result
}

func isInside(inner, outer core.Bounds) bool {
	return inner.X >= outer.X &&
		inner.Y >= outer.Y &&
		inner.X+inner.Width <= outer.X+outer.Width &&
		inner.Y+inner.Height <= outer.Y+outer.Height
}

// FilterContainsDescendants returns elements containing all descendants.
func FilterContainsDescendants(elements []*ParsedElement, allElements []*ParsedElement, descendants []*flow.Selector, platform string) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if containsAllDescendants(elem, allElements, descendants, platform) {
			result = append(result, elem)
		}
	}

	return result
}

func containsAllDescendants(parent *ParsedElement, allElements []*ParsedElement, descendants []*flow.Selector, platform string) bool {
	for _, descSel := range descendants {
		found := false
		for _, elem := range allElements {
			if isInside(elem.Bounds, parent.Bounds) && matchesSelector(elem, *descSel, platform) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// DeepestMatchingElement returns the deepest element.
func DeepestMatchingElement(elements []*ParsedElement) *ParsedElement {
	if len(elements) == 0 {
		return nil
	}

	deepest := elements[0]
	for _, elem := range elements[1:] {
		if elem.Depth > deepest.Depth {
			deepest = elem
		}
	}
	return deepest
}

// SelectByIndex picks an element from candidates using the selector's index.
// If index is specified, picks the Nth candidate (supports negative indexing from end).
// If index is out of range, defaults to first element.
// If no index, returns DeepestMatchingElement (or first if nil).
func SelectByIndex(candidates []*ParsedElement, index string) *ParsedElement {
	if index != "" {
		idx := 0
		if i, err := strconv.Atoi(index); err == nil {
			if i < 0 {
				i = len(candidates) + i
			}
			if i >= 0 && i < len(candidates) {
				idx = i
			}
		}
		return candidates[idx]
	}
	selected := DeepestMatchingElement(candidates)
	if selected == nil {
		return candidates[0]
	}
	return selected
}

// SortClickableFirst puts clickable elements first.
func SortClickableFirst(elements []*ParsedElement) []*ParsedElement {
	var clickable, nonClickable []*ParsedElement

	for _, elem := range elements {
		if elem.Clickable {
			clickable = append(clickable, elem)
		} else {
			nonClickable = append(nonClickable, elem)
		}
	}

	return append(clickable, nonClickable...)
}

// GetClickableElement returns the element to tap on.
// If the element itself is clickable, returns it.
// If not clickable, walks up the parent chain to find the first clickable parent.
// Returns the original element if no clickable parent is found.
// This handles React Native pattern where text nodes aren't clickable but their containers are.
func GetClickableElement(elem *ParsedElement) *ParsedElement {
	if elem == nil {
		return nil
	}

	// If element itself is clickable, use it
	if elem.Clickable {
		return elem
	}

	// Walk up parent chain to find clickable parent
	parent := elem.Parent
	for parent != nil {
		if parent.Clickable {
			return parent
		}
		parent = parent.Parent
	}

	// No clickable parent found - return original element
	return elem
}

// Sorting helpers

func sortByDistanceY(elements []*ParsedElement, refY int) {
	for i := 0; i < len(elements); i++ {
		for j := i + 1; j < len(elements); j++ {
			distI := elements[i].Bounds.Y - refY
			distJ := elements[j].Bounds.Y - refY
			if distJ < distI {
				elements[i], elements[j] = elements[j], elements[i]
			}
		}
	}
}

func sortByDistanceYReverse(elements []*ParsedElement, refY int) {
	for i := 0; i < len(elements); i++ {
		for j := i + 1; j < len(elements); j++ {
			distI := refY - (elements[i].Bounds.Y + elements[i].Bounds.Height)
			distJ := refY - (elements[j].Bounds.Y + elements[j].Bounds.Height)
			if distJ < distI {
				elements[i], elements[j] = elements[j], elements[i]
			}
		}
	}
}

func sortByDistanceX(elements []*ParsedElement, refX int) {
	for i := 0; i < len(elements); i++ {
		for j := i + 1; j < len(elements); j++ {
			distI := elements[i].Bounds.X - refX
			distJ := elements[j].Bounds.X - refX
			if distJ < distI {
				elements[i], elements[j] = elements[j], elements[i]
			}
		}
	}
}

func sortByDistanceXReverse(elements []*ParsedElement, refX int) {
	for i := 0; i < len(elements); i++ {
		for j := i + 1; j < len(elements); j++ {
			distI := refX - (elements[i].Bounds.X + elements[i].Bounds.Width)
			distJ := refX - (elements[j].Bounds.X + elements[j].Bounds.Width)
			if distJ < distI {
				elements[i], elements[j] = elements[j], elements[i]
			}
		}
	}
}

// matchesErrorText matches a text selector against a field's validation
// error (Android only). The attribute is written for every node and is usually empty, and
// an empty string must never satisfy a selector — `.*` would.
func matchesErrorText(pattern, errorText string) bool {
	if errorText == "" {
		return false
	}
	return matchesText(pattern, errorText, "", "")
}

// preferExactCase puts the elements a regex text selector matches in its own
// case ahead of those it matches only when case is ignored, keeping order
// otherwise. Plain text and single matches are returned unchanged.
func preferExactCase(elems []*ParsedElement, pattern string, textsOf func(*ParsedElement) []string) []*ParsedElement {
	if len(elems) < 2 || !looksLikeRegex(pattern) {
		return elems
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return elems
	}
	var exact, rest []*ParsedElement
	for _, e := range elems {
		if anyMatches(re, textsOf(e)) {
			exact = append(exact, e)
		} else {
			rest = append(rest, e)
		}
	}
	return append(exact, rest...)
}

func anyMatches(re *regexp.Regexp, texts []string) bool {
	for _, t := range texts {
		if t != "" && (re.MatchString(t) || re.MatchString(strings.ReplaceAll(t, "\n", " "))) {
			return true
		}
	}
	return false
}

// regexTextsOf lists the attributes a text selector is matched against.
func regexTextsOf(e *ParsedElement, platform string) []string {
	if platform == "ios" {
		return []string{e.Label, e.Name, e.Value, e.PlaceholderValue}
	}
	return []string{e.Text, e.ContentDesc, e.HintText}
}
