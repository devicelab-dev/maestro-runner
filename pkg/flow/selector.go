// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Selector represents element selection criteria.
// This mirrors Maestro's YamlElementSelector exactly.
// Pure data structure - executor decides how to use it.
type Selector struct {
	// Primary selectors
	Text string `yaml:"text" json:"text,omitempty"` // Text to match
	ID   string `yaml:"id" json:"id,omitempty"`     // Resource ID or accessibility ID

	// Size matching
	Width     int `yaml:"width" json:"width,omitempty"`
	Height    int `yaml:"height" json:"height,omitempty"`
	Tolerance int `yaml:"tolerance" json:"tolerance,omitempty"`

	// State filters
	Enabled  *bool `yaml:"enabled" json:"enabled,omitempty"`
	Selected *bool `yaml:"selected" json:"selected,omitempty"`
	Checked  *bool `yaml:"checked" json:"checked,omitempty"`
	Focused  *bool `yaml:"focused" json:"focused,omitempty"`

	// Index for multiple matches (string for variable support)
	Index string `yaml:"index" json:"index,omitempty"`

	// Traits (comma-separated string, e.g., "button,heading")
	Traits string `yaml:"traits" json:"traits,omitempty"`

	// CSS selector for web views
	CSS string `yaml:"css" json:"css,omitempty"`

	// Web-specific selectors
	Placeholder  string `yaml:"placeholder"`  // Match by HTML placeholder attribute
	Role         string `yaml:"role"`         // Match by ARIA role (button, link, tab, etc.)
	TextContains string `yaml:"textContains"` // Partial text match (contains)
	Href         string `yaml:"href"`         // Match links by href attribute
	Alt          string `yaml:"alt"`          // Match by alt attribute (images)
	Title        string `yaml:"title"`        // Match by title attribute (tooltips)
	Name         string `yaml:"name"`         // Match by form field name attribute
	TestID       string `yaml:"testId"`       // Match by data-testid attribute
	TextRegex    string `yaml:"textRegex"`    // Match text by regex pattern
	Nth          int    `yaml:"nth"`          // Pick Nth match (0-based) when multiple elements match

	// Relative selectors
	ChildOf             *Selector   `yaml:"childOf" json:"childOf,omitempty"`
	Below               *Selector   `yaml:"below" json:"below,omitempty"`
	Above               *Selector   `yaml:"above" json:"above,omitempty"`
	LeftOf              *Selector   `yaml:"leftOf" json:"leftOf,omitempty"`
	RightOf             *Selector   `yaml:"rightOf" json:"rightOf,omitempty"`
	ContainsChild       *Selector   `yaml:"containsChild" json:"containsChild,omitempty"`
	ContainsDescendants []*Selector `yaml:"containsDescendants" json:"containsDescendants,omitempty"`
	InsideOf            *Selector   `yaml:"insideOf" json:"insideOf,omitempty"` // Visual containment (center point inside anchor bounds)

	// Inline step properties (parsed with selector for YAML convenience)
	Optional              *bool  `yaml:"optional" json:"-"`
	RetryTapIfNoChange    *bool  `yaml:"retryTapIfNoChange" json:"-"`
	WaitUntilVisible      *bool  `yaml:"waitUntilVisible" json:"-"`
	Point                 string `yaml:"point" json:"-"`                 // Tap point "x%, y%"
	Start                 string `yaml:"start" json:"-"`                 // Swipe start "x%, y%"
	End                   string `yaml:"end" json:"-"`                   // Swipe end "x%, y%"
	Repeat                int    `yaml:"repeat" json:"-"`                // Tap repeat count
	Delay                 int    `yaml:"delay" json:"-"`                 // Delay between repeats (ms)
	WaitToSettleTimeoutMs int    `yaml:"waitToSettleTimeoutMs" json:"-"` // Wait for UI settle (ms)
	Timeout               int    `yaml:"timeout" json:"-"`               // Timeout in ms for element finding
	Label                 string `yaml:"label" json:"-"`                 // Step label
}

// selectorRaw is used for YAML parsing to capture the "element" field.
type selectorRaw struct {
	Text                  string      `yaml:"text"`
	Element               string      `yaml:"element"` // Shorthand for text (used in scrollUntilVisible, etc.)
	ID                    string      `yaml:"id"`
	Width                 int         `yaml:"width"`
	Height                int         `yaml:"height"`
	Tolerance             int         `yaml:"tolerance"`
	Enabled               *bool       `yaml:"enabled"`
	Selected              *bool       `yaml:"selected"`
	Checked               *bool       `yaml:"checked"`
	Focused               *bool       `yaml:"focused"`
	Index                 string      `yaml:"index"`
	Traits                string      `yaml:"traits"`
	CSS                   string      `yaml:"css"`
	Placeholder           string      `yaml:"placeholder"`
	Role                  string      `yaml:"role"`
	TextContains          string      `yaml:"textContains"`
	Href                  string      `yaml:"href"`
	Alt                   string      `yaml:"alt"`
	Title                 string      `yaml:"title"`
	Name                  string      `yaml:"name"`
	TestID                string      `yaml:"testId"`
	TextRegex             string      `yaml:"textRegex"`
	Nth                   int         `yaml:"nth"`
	ChildOf               *Selector   `yaml:"childOf"`
	Below                 *Selector   `yaml:"below"`
	Above                 *Selector   `yaml:"above"`
	LeftOf                *Selector   `yaml:"leftOf"`
	RightOf               *Selector   `yaml:"rightOf"`
	ContainsChild         *Selector   `yaml:"containsChild"`
	ContainsDescendants   []*Selector `yaml:"containsDescendants"`
	InsideOf              *Selector   `yaml:"insideOf"`
	Optional              *bool       `yaml:"optional"`
	RetryTapIfNoChange    *bool       `yaml:"retryTapIfNoChange"`
	WaitUntilVisible      *bool       `yaml:"waitUntilVisible"`
	Point                 string      `yaml:"point"`
	Start                 string      `yaml:"start"`
	End                   string      `yaml:"end"`
	Repeat                int         `yaml:"repeat"`
	Delay                 int         `yaml:"delay"`
	WaitToSettleTimeoutMs int         `yaml:"waitToSettleTimeoutMs"`
	Timeout               int         `yaml:"timeout"`
	Label                 string      `yaml:"label"`
}

// UnmarshalYAML allows Selector to be unmarshaled from string or struct.
func (s *Selector) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		s.Text = node.Value
		return nil
	}

	var raw selectorRaw
	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Copy fields
	s.Text = raw.Text
	s.ID = raw.ID
	s.Width = raw.Width
	s.Height = raw.Height
	s.Tolerance = raw.Tolerance
	s.Enabled = raw.Enabled
	s.Selected = raw.Selected
	s.Checked = raw.Checked
	s.Focused = raw.Focused
	s.Index = raw.Index
	s.Traits = raw.Traits
	s.CSS = raw.CSS
	s.Placeholder = raw.Placeholder
	s.Role = raw.Role
	s.TextContains = raw.TextContains
	s.Href = raw.Href
	s.Alt = raw.Alt
	s.Title = raw.Title
	s.Name = raw.Name
	s.TestID = raw.TestID
	s.TextRegex = raw.TextRegex
	s.Nth = raw.Nth
	s.ChildOf = raw.ChildOf
	s.Below = raw.Below
	s.Above = raw.Above
	s.LeftOf = raw.LeftOf
	s.RightOf = raw.RightOf
	s.ContainsChild = raw.ContainsChild
	s.ContainsDescendants = raw.ContainsDescendants
	s.InsideOf = raw.InsideOf
	s.Optional = raw.Optional
	s.RetryTapIfNoChange = raw.RetryTapIfNoChange
	s.WaitUntilVisible = raw.WaitUntilVisible
	s.Point = raw.Point
	s.Start = raw.Start
	s.End = raw.End
	s.Repeat = raw.Repeat
	s.Delay = raw.Delay
	s.WaitToSettleTimeoutMs = raw.WaitToSettleTimeoutMs
	s.Timeout = raw.Timeout
	s.Label = raw.Label

	// "element" is a shorthand for "text" (used in scrollUntilVisible, etc.)
	if raw.Element != "" && s.Text == "" {
		s.Text = raw.Element
	}

	return nil
}

// IsEmpty returns true if no selector properties are set.
func (s *Selector) IsEmpty() bool {
	return s.Text == "" &&
		s.ID == "" &&
		s.CSS == "" &&
		s.Placeholder == "" &&
		s.Role == "" &&
		s.TextContains == "" &&
		s.Href == "" &&
		s.Alt == "" &&
		s.Title == "" &&
		s.Name == "" &&
		s.TestID == "" &&
		s.TextRegex == "" &&
		s.Width == 0 &&
		s.Height == 0 &&
		s.ChildOf == nil &&
		s.Below == nil &&
		s.Above == nil &&
		s.LeftOf == nil &&
		s.RightOf == nil &&
		s.ContainsChild == nil &&
		len(s.ContainsDescendants) == 0 &&
		s.InsideOf == nil
}

// HasRelativeSelector returns true if any relative selector is set.
func (s *Selector) HasRelativeSelector() bool {
	return s.ChildOf != nil ||
		s.Below != nil ||
		s.Above != nil ||
		s.LeftOf != nil ||
		s.RightOf != nil ||
		s.ContainsChild != nil ||
		len(s.ContainsDescendants) > 0 ||
		s.InsideOf != nil
}

// HasNonZeroIndex returns true if the selector has an index that is not zero.
// Used to route element finding through page source (which returns all matches)
// instead of native APIs (which return a single match).
func (s *Selector) HasNonZeroIndex() bool {
	if s.Index == "" {
		return false
	}
	idx, err := strconv.Atoi(s.Index)
	return err == nil && idx != 0
}

// EffectiveNth returns the integer index to use when picking among multiple
// matches, unifying the mobile-style `index` (string) and web-style `nth`
// (int) selectors. When both are set, `nth` wins (it's already typed and was
// the original web spelling). Negative or unparseable values are treated as 0.
//
// This lets the web driver accept the same `index: N` syntax that flows on
// Android/iOS use, instead of forcing users to remember a separate `nth` field
// per platform.
func (s *Selector) EffectiveNth() int {
	if s.Nth > 0 {
		return s.Nth
	}
	if s.Index == "" {
		return 0
	}
	idx, err := strconv.Atoi(strings.TrimSpace(s.Index))
	if err != nil || idx < 0 {
		return 0
	}
	return idx
}

// Describe returns a human-readable description.
func (s *Selector) Describe() string {
	switch {
	case s.Text != "":
		return s.Text
	case s.ID != "":
		return "#" + s.ID
	case s.CSS != "":
		return "css:" + s.CSS
	case s.TestID != "":
		return "testId:" + s.TestID
	case s.Role != "":
		return "role:" + s.Role
	case s.Placeholder != "":
		return "placeholder:" + s.Placeholder
	case s.Href != "":
		return "href:" + s.Href
	case s.Alt != "":
		return "alt:" + s.Alt
	case s.Title != "":
		return "title:" + s.Title
	case s.Name != "":
		return "name:" + s.Name
	case s.TextContains != "":
		return "textContains:" + s.TextContains
	case s.TextRegex != "":
		return "textRegex:" + s.TextRegex
	default:
		return ""
	}
}

// DescribeQuoted returns a quoted description like text="value" or id="value".
func (s *Selector) DescribeQuoted() string {
	switch {
	case s.Text != "":
		return "text=\"" + s.Text + "\""
	case s.ID != "":
		return "id=\"" + s.ID + "\""
	case s.CSS != "":
		return "css=\"" + s.CSS + "\""
	case s.TestID != "":
		return "testId=\"" + s.TestID + "\""
	case s.Role != "":
		return "role=\"" + s.Role + "\""
	case s.Placeholder != "":
		return "placeholder=\"" + s.Placeholder + "\""
	case s.Href != "":
		return "href=\"" + s.Href + "\""
	case s.Alt != "":
		return "alt=\"" + s.Alt + "\""
	case s.Title != "":
		return "title=\"" + s.Title + "\""
	case s.Name != "":
		return "name=\"" + s.Name + "\""
	case s.TextContains != "":
		return "textContains=\"" + s.TextContains + "\""
	case s.TextRegex != "":
		return "textRegex=\"" + s.TextRegex + "\""
	default:
		return ""
	}
}

// Per-platform supported selector fields.
// Fields not in a platform's set will generate a warning when used.
var platformSupportedFields = map[string]map[string]bool{
	"android": {
		"text": true, "id": true, "css": true,
		"width": true, "height": true, "tolerance": true,
		"enabled": true, "selected": true, "checked": true, "focused": true,
		"index":   true,
		"childOf": true, "below": true, "above": true,
		"leftOf": true, "rightOf": true,
		"containsChild": true, "containsDescendants": true, "insideOf": true,
	},
	"ios": {
		"text": true, "id": true,
		"width": true, "height": true, "tolerance": true,
		"enabled": true, "selected": true, "focused": true,
		"index":   true,
		"childOf": true, "below": true, "above": true,
		"leftOf": true, "rightOf": true,
		"containsChild": true, "containsDescendants": true, "insideOf": true,
	},
	"web": {
		"text": true, "id": true, "css": true,
		"enabled": true, "selected": true, "checked": true, "focused": true,
		"placeholder": true, "role": true, "textContains": true,
		"href": true, "alt": true, "title": true,
		"name": true, "testId": true, "textRegex": true,
		"nth": true, "index": true,
	},
}

// selectorFieldNames maps each selector field to its YAML tag name.
// Only includes fields that are actual element-matching criteria (not inline step properties).
type selectorField struct {
	Name  string
	IsSet func(s *Selector) bool
}

var selectorFields = []selectorField{
	{"text", func(s *Selector) bool { return s.Text != "" }},
	{"id", func(s *Selector) bool { return s.ID != "" }},
	{"css", func(s *Selector) bool { return s.CSS != "" }},
	{"width", func(s *Selector) bool { return s.Width != 0 }},
	{"height", func(s *Selector) bool { return s.Height != 0 }},
	{"tolerance", func(s *Selector) bool { return s.Tolerance != 0 }},
	{"enabled", func(s *Selector) bool { return s.Enabled != nil }},
	{"selected", func(s *Selector) bool { return s.Selected != nil }},
	{"checked", func(s *Selector) bool { return s.Checked != nil }},
	{"focused", func(s *Selector) bool { return s.Focused != nil }},
	{"index", func(s *Selector) bool { return s.Index != "" }},
	{"traits", func(s *Selector) bool { return s.Traits != "" }},
	{"placeholder", func(s *Selector) bool { return s.Placeholder != "" }},
	{"role", func(s *Selector) bool { return s.Role != "" }},
	{"textContains", func(s *Selector) bool { return s.TextContains != "" }},
	{"href", func(s *Selector) bool { return s.Href != "" }},
	{"alt", func(s *Selector) bool { return s.Alt != "" }},
	{"title", func(s *Selector) bool { return s.Title != "" }},
	{"name", func(s *Selector) bool { return s.Name != "" }},
	{"testId", func(s *Selector) bool { return s.TestID != "" }},
	{"textRegex", func(s *Selector) bool { return s.TextRegex != "" }},
	{"nth", func(s *Selector) bool { return s.Nth != 0 }},
	{"childOf", func(s *Selector) bool { return s.ChildOf != nil }},
	{"below", func(s *Selector) bool { return s.Below != nil }},
	{"above", func(s *Selector) bool { return s.Above != nil }},
	{"leftOf", func(s *Selector) bool { return s.LeftOf != nil }},
	{"rightOf", func(s *Selector) bool { return s.RightOf != nil }},
	{"containsChild", func(s *Selector) bool { return s.ContainsChild != nil }},
	{"containsDescendants", func(s *Selector) bool { return len(s.ContainsDescendants) > 0 }},
	{"insideOf", func(s *Selector) bool { return s.InsideOf != nil }},
}

// CheckUnsupportedFields returns the names of selector fields that are set
// but not supported on the given platform. The platform should be lowercase
// ("android", "ios", "web"). Returns nil if all set fields are supported.
func CheckUnsupportedFields(sel *Selector, platform string) []string {
	supported := platformSupportedFields[strings.ToLower(platform)]
	if supported == nil {
		return nil // unknown platform — don't warn
	}

	var unsupported []string
	for _, f := range selectorFields {
		if f.IsSet(sel) && !supported[f.Name] {
			unsupported = append(unsupported, f.Name)
		}
	}
	return unsupported
}

// ExtractSelectors returns all selectors referenced by a step (including nested ones
// in relative selectors). Returns nil for steps without selectors.
func ExtractSelectors(step Step) []*Selector {
	var selectors []*Selector

	addSel := func(s *Selector) {
		if s == nil {
			return
		}
		selectors = append(selectors, s)
	}

	switch s := step.(type) {
	case *TapOnStep:
		addSel(&s.Selector)
	case *DoubleTapOnStep:
		addSel(&s.Selector)
	case *LongPressOnStep:
		addSel(&s.Selector)
	case *SwipeStep:
		addSel(s.Selector)
	case *ScrollUntilVisibleStep:
		addSel(&s.Element)
	case *InputTextStep:
		addSel(&s.Selector)
	case *CopyTextFromStep:
		addSel(&s.Selector)
	case *AssertVisibleStep:
		addSel(&s.Selector)
	case *AssertNotVisibleStep:
		addSel(&s.Selector)
	case *AssertConditionStep:
		addSel(s.Condition.Visible)
		addSel(s.Condition.NotVisible)
	case *WaitUntilStep:
		addSel(s.Visible)
		addSel(s.NotVisible)
	}

	return selectors
}
