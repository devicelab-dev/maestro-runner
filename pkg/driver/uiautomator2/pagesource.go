package uiautomator2

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
type ParsedElement struct {
	Text        string
	ResourceID  string
	ContentDesc string
	HintText    string // hint attribute for EditText fields
	// ErrorText is the field's validation error (AccessibilityNodeInfo.getError),
	// the `error` attribute the uiautomator2 server writes for every node —
	// empty for most. A Compose field whose only content is its error
	// semantics is found by that text, as in Maestro since 2.9.
	ErrorText  string
	ClassName  string
	Bounds     core.Bounds
	Enabled    bool
	Selected   bool
	Checked    bool
	Focused    bool
	Displayed  bool
	Clickable  bool
	Scrollable bool
	Children   []*ParsedElement
	Parent     *ParsedElement // parent element for clickable lookup
	Depth      int            // depth in hierarchy (for deepestMatchingElement)
}

// ParsePageSource parses Android UI hierarchy XML into elements.
// Supports both formats:
// - UIAutomator dump: uses class name as element tag (e.g., <android.widget.FrameLayout>)
// - Appium format: uses <node> elements
func ParsePageSource(xmlData string) ([]*ParsedElement, error) {
	// Use a flexible decoder that handles any element names
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
				// Skip the hierarchy element
				if t.Name.Local == "hierarchy" {
					foundHierarchy = true
					continue
				}

				// Parse attributes
				elem := &ParsedElement{
					ClassName: t.Name.Local, // Class name is the element tag
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
						elem.ClassName = attr.Value // Override if class attr exists
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

				// Parse children recursively
				for {
					child, err := parseElement()
					if err != nil || child == nil {
						break
					}
					elem.Children = append(elem.Children, child)
				}

				return elem, nil

			case xml.EndElement:
				return nil, nil // End of current element
			}
		}
	}

	// Parse all root-level elements under hierarchy
	var parseErr error
	for {
		elem, err := parseElement()
		if err != nil {
			// io.EOF is expected at end of document
			if err.Error() != "EOF" {
				parseErr = err
			}
			break
		}
		if elem != nil {
			elements = append(elements, flattenElement(elem, 0)...)
		}
	}

	// Return error for invalid XML
	if parseErr != nil && len(elements) == 0 {
		return nil, parseErr
	}

	// Return error if no valid hierarchy found
	if !foundHierarchy {
		return nil, fmt.Errorf("invalid page source: no hierarchy element found")
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

// parseBounds parses Android bounds string "[x1,y1][x2,y2]" to Bounds.
func parseBounds(s string) core.Bounds {
	// Format: [x1,y1][x2,y2]
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

// FilterOutOfBounds removes elements that are less than 10% visible on screen.
// This matches Maestro's filterOutOfBounds behavior — page source XML includes
// elements from the full accessibility tree, not just the visible viewport.
func FilterOutOfBounds(elements []*ParsedElement, screenWidth, screenHeight int) []*ParsedElement {
	result := make([]*ParsedElement, 0, len(elements))
	for _, e := range elements {
		if e.Bounds.VisiblePercentage(screenWidth, screenHeight) >= 0.1 {
			result = append(result, e)
		}
	}
	return result
}

// CountDisplayedMatches returns how many elements match the selector and are
// marked displayed. Matching reuses FilterBySelector, so "what counts" is
// exactly what an index: selector could pick; the displayed check mirrors the
// Visible field assertVisible already gates on.
func CountDisplayedMatches(elements []*ParsedElement, sel flow.Selector) int {
	count := 0
	for _, elem := range FilterBySelector(elements, sel) {
		if elem.Displayed {
			count++
		}
	}
	return count
}

// FilterBySelector filters elements by non-relative selector properties.
func FilterBySelector(elements []*ParsedElement, sel flow.Selector) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if !matchesSelector(elem, sel) {
			continue
		}
		result = append(result, elem)
	}

	result = preferExactID(result, sel.ID)
	result = preferExactText(result, sel)
	return preferExactCase(result, sel.Text, func(e *ParsedElement) []string { return regexTextsOf(e) })
}

// preferExactID keeps only the matches whose whole id matches the selector —
// the full resource-id or the part after the last "/", as Maestro matches ids —
// when there are any. The substring match otherwise lets a superset id win:
// `omnibarTextInput|inputField` also hits the empty
// `omnibarTextInputClickCatcher` overlay, and copyTextFrom copied nothing.
// With no whole-id match the lenient set is kept.
func preferExactID(elems []*ParsedElement, id string) []*ParsedElement {
	if id == "" || len(elems) < 2 {
		return elems
	}
	re, err := regexp.Compile(`(?i)\A(?:` + id + `)\z`)
	if err != nil {
		return elems
	}
	var exact []*ParsedElement
	for _, e := range elems {
		short := e.ResourceID[strings.LastIndex(e.ResourceID, "/")+1:]
		if re.MatchString(e.ResourceID) || re.MatchString(short) {
			exact = append(exact, e)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return elems
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
func preferExactText(matches []*ParsedElement, sel flow.Selector) []*ParsedElement {
	if sel.Text == "" || looksLikeRegex(sel.Text) || len(matches) < 2 {
		return matches
	}
	var exact []*ParsedElement
	for _, elem := range matches {
		if equalsAny(sel.Text, elem.Text, elem.ContentDesc, elem.HintText) {
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

func matchesSelector(elem *ParsedElement, sel flow.Selector) bool {
	// Text matching - supports regex patterns and literal contains
	// Checks text, content-desc (accessibility text), and hint text
	if sel.Text != "" {
		if !matchesText(sel.Text, elem.Text, elem.ContentDesc, elem.HintText) && !matchesErrorText(sel.Text, elem.ErrorText) {
			return false
		}
	}

	// ID matching (partial, supports regex)
	if sel.ID != "" {
		if !matchesID(sel.ID, elem.ResourceID) {
			return false
		}
	}

	// Size matching with tolerance
	if sel.Width > 0 || sel.Height > 0 {
		tolerance := sel.Tolerance
		if tolerance == 0 {
			tolerance = 5 // default 5px tolerance
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
		// checked maps to selected in Android
		return false
	}

	return true
}

// withinTolerance checks if actual is within tolerance of expected.
func withinTolerance(actual, expected, tolerance int) bool {
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// matchesID reports whether an id selector matches a resource id as Maestro's
// idMatches does: a case-insensitive regex over the whole id, or over the part
// after its package prefix. `id: login` is "com.app:id/login", not
// "com.app:id/login_button" (#188); `id: Flatlist` finds "FlatList".
func matchesID(pattern, id string) bool {
	return core.MatchesIDMaestro(pattern, id)
}

// matchesText reports whether a text selector matches the element's text,
// content-desc or hint, as Maestro's textMatches does: the selector is a
// case-insensitive regex that must match a whole value, so "Open" is not
// "Talk · Open" and "English" is not "Language: English" (#188); a partial
// match is written `.*Open.*`. When several elements match, FilterBySelector
// puts the ones matching in the pattern's own case first (#151).
func matchesText(pattern, text, contentDesc, hintText string) bool {
	return core.MatchesTextMaestro(pattern, text, contentDesc, hintText)
}

// Position filter functions

// FilterBelow returns elements below the anchor element.
func FilterBelow(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorBottom := anchor.Bounds.Y + anchor.Bounds.Height
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's top must be below anchor's bottom
		if elem.Bounds.Y >= anchorBottom {
			result = append(result, elem)
		}
	}

	// Sort by distance (closest first)
	sortByDistanceY(result, anchorBottom)
	return result
}

// FilterAbove returns elements above the anchor element.
func FilterAbove(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorTop := anchor.Bounds.Y
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's bottom must be above anchor's top
		elemBottom := elem.Bounds.Y + elem.Bounds.Height
		if elemBottom <= anchorTop {
			result = append(result, elem)
		}
	}

	// Sort by distance (closest first - highest Y value)
	sortByDistanceYReverse(result, anchorTop)
	return result
}

// FilterLeftOf returns elements left of the anchor element.
func FilterLeftOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorLeft := anchor.Bounds.X
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's right must be left of anchor's left
		elemRight := elem.Bounds.X + elem.Bounds.Width
		if elemRight <= anchorLeft {
			result = append(result, elem)
		}
	}

	sortByDistanceXReverse(result, anchorLeft)
	return result
}

// FilterRightOf returns elements right of the anchor element.
func FilterRightOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorRight := anchor.Bounds.X + anchor.Bounds.Width
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's left must be right of anchor's right
		if elem.Bounds.X >= anchorRight {
			result = append(result, elem)
		}
	}

	sortByDistanceX(result, anchorRight)
	return result
}

// FilterChildOf returns elements that are children of anchor.
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

// FilterContainsChild returns elements that contain anchor as child.
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

// FilterInsideOf returns elements whose center point is inside anchor bounds.
// Different from ChildOf - uses visual center containment, not full bounds.
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

// Simple sorting by distance (not using sort package to keep it simple)
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

// FilterContainsDescendants returns elements that contain ALL specified descendants.
// Each descendant selector must match at least one child within the element's bounds.
func FilterContainsDescendants(elements []*ParsedElement, allElements []*ParsedElement, descendants []*flow.Selector) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if containsAllDescendants(elem, allElements, descendants) {
			result = append(result, elem)
		}
	}

	return result
}

func containsAllDescendants(parent *ParsedElement, allElements []*ParsedElement, descendants []*flow.Selector) bool {
	for _, descSel := range descendants {
		found := false
		for _, elem := range allElements {
			// Check if elem is inside parent and matches selector
			if isInside(elem.Bounds, parent.Bounds) && matchesSelector(elem, *descSel) {
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

// DeepestMatchingElement returns the element with the highest depth (deepest in hierarchy).
// This helps avoid tapping on container elements when a more specific child matches.
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
		if i, err := core.ParseIndex(index); err == nil {
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

// SortClickableFirst reorders elements to prioritize clickable ones.
// Clickable elements come first, maintaining relative order within each group.
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

// FilterScrollable returns only scrollable elements from the list.
// Used to find scrollable containers for swipe operations.
func FilterScrollable(elements []*ParsedElement) []*ParsedElement {
	var result []*ParsedElement
	for _, elem := range elements {
		if elem.Scrollable && elem.Bounds.Width > 0 && elem.Bounds.Height > 0 {
			result = append(result, elem)
		}
	}
	return result
}

// FindLargestScrollable returns the scrollable element with the largest area.
// Returns nil if no scrollable elements are found.
func FindLargestScrollable(elements []*ParsedElement) *ParsedElement {
	scrollables := FilterScrollable(elements)
	if len(scrollables) == 0 {
		return nil
	}

	largest := scrollables[0]
	largestArea := largest.Bounds.Width * largest.Bounds.Height

	for _, elem := range scrollables[1:] {
		area := elem.Bounds.Width * elem.Bounds.Height
		if area > largestArea {
			largest = elem
			largestArea = area
		}
	}

	return largest
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

// matchesErrorText matches a text selector against a field's validation
// error. The attribute is written for every node and is usually empty, and
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
func regexTextsOf(e *ParsedElement) []string {
	return []string{e.Text, e.ContentDesc, e.HintText}
}
