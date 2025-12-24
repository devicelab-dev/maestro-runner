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

// UnmarshalYAML allows Selector to be unmarshaled from string or struct.
func (s *Selector) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		s.Text = node.Value
		return nil
	}

	type selectorAlias Selector
	var alias selectorAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*s = Selector(alias)
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
		len(s.ContainsDescendants) == 0
}

// HasRelativeSelector returns true if any relative selector is set.
func (s *Selector) HasRelativeSelector() bool {
	return s.ChildOf != nil ||
		s.Below != nil ||
		s.Above != nil ||
		s.LeftOf != nil ||
		s.RightOf != nil ||
		s.ContainsChild != nil ||
		len(s.ContainsDescendants) > 0
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
