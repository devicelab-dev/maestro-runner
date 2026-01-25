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
	Focused   bool
	Clickable bool
	Depth     int
	Children  []*ParsedElement

	// Android
	Text        string
	ResourceID  string
	ContentDesc string
	HintText    string
	ClassName   string

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
					case "class":
						elem.ClassName = attr.Value
					case "bounds":
						elem.Bounds = parseBounds(attr.Value)
					case "enabled":
						elem.Enabled = attr.Value == "true"
					case "selected":
						elem.Selected = attr.Value == "true"
					case "focused":
						elem.Focused = attr.Value == "true"
					case "displayed":
						elem.Displayed = attr.Value != "false"
					case "clickable":
						elem.Clickable = attr.Value == "true"
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

// flattenElement flattens a tree of elements into a list.
func flattenElement(elem *ParsedElement, depth int) []*ParsedElement {
	elem.Depth = depth
	result := []*ParsedElement{elem}
	for _, child := range elem.Children {
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

	return result
}

func matchesSelector(elem *ParsedElement, sel flow.Selector, platform string) bool {
	// Text matching
	if sel.Text != "" {
		if platform == "ios" {
			if !matchesText(sel.Text, elem.Label, elem.Name, elem.Value, elem.PlaceholderValue) {
				return false
			}
		} else {
			if !matchesText(sel.Text, elem.Text, elem.ContentDesc, elem.HintText) {
				return false
			}
		}
	}

	// ID matching
	if sel.ID != "" {
		if platform == "ios" {
			if !strings.Contains(elem.Name, sel.ID) {
				return false
			}
		} else {
			if !strings.Contains(elem.ResourceID, sel.ID) {
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
	if sel.Checked != nil && elem.Selected != *sel.Checked {
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
		case '*', '+', '?', '[', ']', '{', '}', '|':
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

// FilterChildOf returns elements inside anchor bounds.
func FilterChildOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if isInside(elem.Bounds, anchor.Bounds) {
			result = append(result, elem)
		}
	}

	return result
}

// FilterContainsChild returns elements that contain anchor.
func FilterContainsChild(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if isInside(anchor.Bounds, elem.Bounds) {
			result = append(result, elem)
		}
	}

	return result
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
