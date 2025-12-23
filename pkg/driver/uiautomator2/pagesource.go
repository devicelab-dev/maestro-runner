package uiautomator2

import (
	"encoding/xml"
	"fmt"
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
	ClassName   string
	Bounds      core.Bounds
	Enabled     bool
	Selected    bool
	Focused     bool
	Displayed   bool
	Clickable   bool
	Children    []*ParsedElement
	Depth       int // depth in hierarchy (for deepestMatchingElement)
}

// ParsePageSource parses Android UI hierarchy XML into elements.
func ParsePageSource(xmlData string) ([]*ParsedElement, error) {
	var hierarchy struct {
		XMLName xml.Name `xml:"hierarchy"`
		Nodes   []xmlNode `xml:"node"`
	}

	if err := xml.Unmarshal([]byte(xmlData), &hierarchy); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	var elements []*ParsedElement
	for _, node := range hierarchy.Nodes {
		elements = append(elements, parseNode(node)...)
	}
	return elements, nil
}

type xmlNode struct {
	Text        string    `xml:"text,attr"`
	ResourceID  string    `xml:"resource-id,attr"`
	ContentDesc string    `xml:"content-desc,attr"`
	Class       string    `xml:"class,attr"`
	Bounds      string    `xml:"bounds,attr"`
	Enabled     string    `xml:"enabled,attr"`
	Selected    string    `xml:"selected,attr"`
	Focused     string    `xml:"focused,attr"`
	Displayed   string    `xml:"displayed,attr"`
	Clickable   string    `xml:"clickable,attr"`
	Children    []xmlNode `xml:"node"`
}

func parseNode(node xmlNode) []*ParsedElement {
	return parseNodeWithDepth(node, 0)
}

func parseNodeWithDepth(node xmlNode, depth int) []*ParsedElement {
	elem := &ParsedElement{
		Text:        node.Text,
		ResourceID:  node.ResourceID,
		ContentDesc: node.ContentDesc,
		ClassName:   node.Class,
		Bounds:      parseBounds(node.Bounds),
		Enabled:     node.Enabled == "true",
		Selected:    node.Selected == "true",
		Focused:     node.Focused == "true",
		Displayed:   node.Displayed != "false", // default true
		Clickable:   node.Clickable == "true",
		Depth:       depth,
	}

	var all []*ParsedElement
	all = append(all, elem)

	// Recursively parse children
	for _, child := range node.Children {
		childElements := parseNodeWithDepth(child, depth+1)
		elem.Children = append(elem.Children, childElements[0]) // first is direct child
		all = append(all, childElements...)
	}

	return all
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

// FilterBySelector filters elements by non-relative selector properties.
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
	// Text matching (case-insensitive contains)
	if sel.Text != "" {
		textLower := strings.ToLower(sel.Text)
		if !strings.Contains(strings.ToLower(elem.Text), textLower) &&
			!strings.Contains(strings.ToLower(elem.ContentDesc), textLower) {
			return false
		}
	}

	// ID matching (partial)
	if sel.ID != "" {
		if !strings.Contains(elem.ResourceID, sel.ID) {
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
	if sel.Checked != nil && elem.Selected != *sel.Checked {
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
	// Element must be fully inside anchor bounds
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
