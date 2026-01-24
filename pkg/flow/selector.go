// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import "gopkg.in/yaml.v3"

// Selector represents element selection criteria.
// This mirrors Maestro's YamlElementSelector exactly.
// Pure data structure - executor decides how to use it.
type Selector struct {
	// Primary selectors
	Text string `yaml:"text"` // Text to match
	ID   string `yaml:"id"`   // Resource ID or accessibility ID

	// Size matching
	Width     int `yaml:"width"`
	Height    int `yaml:"height"`
	Tolerance int `yaml:"tolerance"`

	// State filters
	Enabled  *bool `yaml:"enabled"`
	Selected *bool `yaml:"selected"`
	Checked  *bool `yaml:"checked"`
	Focused  *bool `yaml:"focused"`

	// Index for multiple matches (string for variable support)
	Index string `yaml:"index"`

	// Traits (comma-separated string, e.g., "button,heading")
	Traits string `yaml:"traits"`

	// CSS selector for web views
	CSS string `yaml:"css"`

	// Relative selectors
	ChildOf             *Selector   `yaml:"childOf"`
	Below               *Selector   `yaml:"below"`
	Above               *Selector   `yaml:"above"`
	LeftOf              *Selector   `yaml:"leftOf"`
	RightOf             *Selector   `yaml:"rightOf"`
	ContainsChild       *Selector   `yaml:"containsChild"`
	ContainsDescendants []*Selector `yaml:"containsDescendants"`
	InsideOf            *Selector   `yaml:"insideOf"` // Visual containment (center point inside anchor bounds)

	// Inline step properties (parsed with selector for YAML convenience)
	Optional              *bool  `yaml:"optional"`
	RetryTapIfNoChange    *bool  `yaml:"retryTapIfNoChange"`
	WaitUntilVisible      *bool  `yaml:"waitUntilVisible"`
	Point                 string `yaml:"point"`                 // Tap point "x%, y%"
	Start                 string `yaml:"start"`                 // Swipe start "x%, y%"
	End                   string `yaml:"end"`                   // Swipe end "x%, y%"
	Repeat                int    `yaml:"repeat"`                // Tap repeat count
	Delay                 int    `yaml:"delay"`                 // Delay between repeats (ms)
	WaitToSettleTimeoutMs int    `yaml:"waitToSettleTimeoutMs"` // Wait for UI settle (ms)
	Label                 string `yaml:"label"`                 // Step label
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

// Describe returns a human-readable description.
func (s *Selector) Describe() string {
	switch {
	case s.Text != "":
		return s.Text
	case s.ID != "":
		return "#" + s.ID
	case s.CSS != "":
		return "css:" + s.CSS
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
	default:
		return ""
	}
}
