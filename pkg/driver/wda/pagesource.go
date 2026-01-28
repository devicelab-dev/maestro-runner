package wda

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ParsedElement represents an element from iOS page source XML.
type ParsedElement struct {
	Type             string // XCUIElementType (e.g., "XCUIElementTypeButton")
	Name             string // accessibility identifier
	Label            string // accessibility label
	Value            string // current value
	PlaceholderValue string // placeholder text for text fields
	Bounds           core.Bounds
	Enabled          bool
	Displayed        bool // visible
	Selected         bool
	Focused          bool
	Children         []*ParsedElement
	Parent           *ParsedElement // parent element for clickable lookup
	Depth            int
}

// ParsePageSource parses iOS UI hierarchy XML into elements.
// iOS WDA uses XCUIElement types with attributes:
// - type: XCUIElementTypeButton, XCUIElementTypeTextField, etc.
// - name: accessibility identifier
// - label: accessibility label (visible text)
// - value: current value
// - enabled, visible, selected, focused: states
// - x, y, width, height: bounds
func ParsePageSource(xmlData string) ([]*ParsedElement, error) {
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
				// iOS uses XCUIElementType* as element tags or "AppiumAUT" as root
				if t.Name.Local == "AppiumAUT" {
					// Root element - skip and parse children
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
					Enabled:   true, // default
					Displayed: true, // default
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

// flattenElement flattens a tree of elements into a list, setting depth.
func flattenElement(elem *ParsedElement, depth int) []*ParsedElement {
	elem.Depth = depth
	result := []*ParsedElement{elem}
	for _, child := range elem.Children {
		child.Parent = elem // Set parent reference
		result = append(result, flattenElement(child, depth+1)...)
	}
	return result
}

// FilterBySelector filters elements by selector properties.
func FilterBySelector(elements []*ParsedElement, sel flow.Selector) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if !matchesSelector(elem, sel) {
			continue
		}
		result = append(result, elem)
	}

	return result
}

func matchesSelector(elem *ParsedElement, sel flow.Selector) bool {
	// Text matching - check label, name, value, and placeholderValue
	if sel.Text != "" {
		if !matchesText(sel.Text, elem.Label, elem.Name, elem.Value, elem.PlaceholderValue) {
			return false
		}
	}

	// ID matching (accessibility identifier)
	if sel.ID != "" {
		if !strings.Contains(elem.Name, sel.ID) {
			return false
		}
	}

	// Size matching with tolerance
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

// matchesText checks if pattern matches any of the text fields.
func matchesText(pattern string, texts ...string) bool {
	if looksLikeRegex(pattern) {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			// Invalid regex - fall back to contains
			for _, text := range texts {
				if containsIgnoreCase(text, pattern) {
					return true
				}
			}
			return false
		}

		for _, text := range texts {
			if text != "" {
				strippedText := strings.ReplaceAll(text, "\n", " ")
				if re.MatchString(text) || re.MatchString(strippedText) || pattern == text {
					return true
				}
			}
		}
		return false
	}

	// Literal text - case-insensitive contains
	for _, text := range texts {
		if containsIgnoreCase(text, pattern) {
			return true
		}
	}
	return false
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// looksLikeRegex checks if text contains regex metacharacters.
// A standalone period (like in "mastodon.social") is NOT treated as regex.
func looksLikeRegex(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		// Check if it's escaped
		if i > 0 && text[i-1] == '\\' {
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

// FilterBelow returns elements below the anchor element.
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

// FilterAbove returns elements above the anchor element.
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

// FilterLeftOf returns elements left of the anchor element.
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

// FilterRightOf returns elements right of the anchor element.
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

// FilterChildOf returns elements that are children of anchor.
func FilterChildOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if isInside(elem.Bounds, anchor.Bounds) {
			result = append(result, elem)
		}
	}

	return result
}

// FilterContainsChild returns elements that contain anchor as child.
func FilterContainsChild(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if isInside(anchor.Bounds, elem.Bounds) {
			result = append(result, elem)
		}
	}

	return result
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

// FilterContainsDescendants returns elements that contain ALL specified descendants.
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

// DeepestMatchingElement returns the element with the highest depth.
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

// isClickableType checks if an iOS element type is typically clickable/interactive.
// This mimics Maestro's smart element selection for iOS.
func isClickableType(elemType string) bool {
	// iOS element types that are interactive/clickable
	switch elemType {
	case "XCUIElementTypeButton",
		"XCUIElementTypeLink",
		"XCUIElementTypeTextField",
		"XCUIElementTypeSecureTextField",
		"XCUIElementTypeSearchField",
		"XCUIElementTypeSwitch",
		"XCUIElementTypeSlider",
		"XCUIElementTypeStepper",
		"XCUIElementTypeSegmentedControl",
		"XCUIElementTypeCell",
		"XCUIElementTypeTab",
		"XCUIElementTypeTabBar",
		"XCUIElementTypeMenu",
		"XCUIElementTypeMenuItem",
		"XCUIElementTypePickerWheel",
		"XCUIElementTypeDatePicker",
		"XCUIElementTypeToggle",
		"XCUIElementTypePageIndicator":
		return true
	default:
		return false
	}
}

// SortClickableFirst reorders elements to prioritize interactive ones.
// iOS doesn't expose a "clickable" attribute, so we determine clickability
// from the element type (Button, Link, TextField, etc. are clickable).
func SortClickableFirst(elements []*ParsedElement) []*ParsedElement {
	var clickable, nonClickable []*ParsedElement

	for _, elem := range elements {
		if isClickableType(elem.Type) {
			clickable = append(clickable, elem)
		} else {
			nonClickable = append(nonClickable, elem)
		}
	}

	return append(clickable, nonClickable...)
}

// GetClickableElement returns the element to tap on.
// If the element itself is a clickable type, returns it.
// If not, walks up the parent chain to find the first clickable parent.
// Returns the original element if no clickable parent is found.
// This handles patterns where text labels aren't interactive but their parent containers are.
func GetClickableElement(elem *ParsedElement) *ParsedElement {
	if elem == nil {
		return nil
	}

	// If element itself is clickable, use it
	if isClickableType(elem.Type) {
		return elem
	}

	// Walk up parent chain to find clickable parent
	parent := elem.Parent
	for parent != nil {
		if isClickableType(parent.Type) {
			return parent
		}
		parent = parent.Parent
	}

	// No clickable parent found - return original element
	return elem
}

// Sorting functions

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
