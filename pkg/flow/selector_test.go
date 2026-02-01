package flow

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSelector_UnmarshalYAML_ScalarValue(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected string
	}{
		{
			name:     "simple text",
			yaml:     `"Login"`,
			expected: "Login",
		},
		{
			name:     "text with spaces",
			yaml:     `"Sign Up Now"`,
			expected: "Sign Up Now",
		},
		{
			name:     "unquoted text",
			yaml:     `Submit`,
			expected: "Submit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Selector
			if err := yaml.Unmarshal([]byte(tt.yaml), &s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Text != tt.expected {
				t.Errorf("got Text=%q, want %q", s.Text, tt.expected)
			}
		})
	}
}

func TestSelector_UnmarshalYAML_StructValue(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		validate func(t *testing.T, s *Selector)
	}{
		{
			name: "id selector",
			yaml: `id: login-btn`,
			validate: func(t *testing.T, s *Selector) {
				if s.ID != "login-btn" {
					t.Errorf("got ID=%q, want login-btn", s.ID)
				}
			},
		},
		{
			name: "text and id",
			yaml: `
text: Login
id: login-btn
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Text != "Login" {
					t.Errorf("got Text=%q, want Login", s.Text)
				}
				if s.ID != "login-btn" {
					t.Errorf("got ID=%q, want login-btn", s.ID)
				}
			},
		},
		{
			name: "size selector",
			yaml: `
width: 100
height: 50
tolerance: 5
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Width != 100 {
					t.Errorf("got Width=%d, want 100", s.Width)
				}
				if s.Height != 50 {
					t.Errorf("got Height=%d, want 50", s.Height)
				}
				if s.Tolerance != 5 {
					t.Errorf("got Tolerance=%d, want 5", s.Tolerance)
				}
			},
		},
		{
			name: "state filters",
			yaml: `
text: Button
enabled: true
selected: false
checked: true
focused: false
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Enabled == nil || !*s.Enabled {
					t.Error("expected enabled=true")
				}
				if s.Selected == nil || *s.Selected {
					t.Error("expected selected=false")
				}
				if s.Checked == nil || !*s.Checked {
					t.Error("expected checked=true")
				}
				if s.Focused == nil || *s.Focused {
					t.Error("expected focused=false")
				}
			},
		},
		{
			name: "index as string",
			yaml: `
text: Item
index: "2"
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Index != "2" {
					t.Errorf("got Index=%q, want 2", s.Index)
				}
			},
		},
		{
			name: "traits as string",
			yaml: `
text: Button
traits: "button,heading"
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Traits != "button,heading" {
					t.Errorf("got Traits=%q, want button,heading", s.Traits)
				}
			},
		},
		{
			name: "css selector",
			yaml: `css: "#login-form input[type=submit]"`,
			validate: func(t *testing.T, s *Selector) {
				if s.CSS != "#login-form input[type=submit]" {
					t.Errorf("got CSS=%q, want #login-form input[type=submit]", s.CSS)
				}
			},
		},
		{
			name: "relative selector - below",
			yaml: `
text: Submit
below:
  text: Username
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Text != "Submit" {
					t.Errorf("got Text=%q, want Submit", s.Text)
				}
				if s.Below == nil {
					t.Fatal("expected Below to be set")
				}
				if s.Below.Text != "Username" {
					t.Errorf("got Below.Text=%q, want Username", s.Below.Text)
				}
			},
		},
		{
			name: "relative selector - above",
			yaml: `
text: Submit
above:
  id: footer
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Above == nil || s.Above.ID != "footer" {
					t.Error("expected Above with id=footer")
				}
			},
		},
		{
			name: "relative selector - leftOf and rightOf",
			yaml: `
text: Middle
leftOf:
  text: Right
rightOf:
  text: Left
`,
			validate: func(t *testing.T, s *Selector) {
				if s.LeftOf == nil || s.LeftOf.Text != "Right" {
					t.Error("expected LeftOf with text=Right")
				}
				if s.RightOf == nil || s.RightOf.Text != "Left" {
					t.Error("expected RightOf with text=Left")
				}
			},
		},
		{
			name: "relative selector - childOf",
			yaml: `
text: Item
childOf:
  id: list-container
`,
			validate: func(t *testing.T, s *Selector) {
				if s.ChildOf == nil || s.ChildOf.ID != "list-container" {
					t.Error("expected ChildOf with id=list-container")
				}
			},
		},
		{
			name: "relative selector - containsChild",
			yaml: `
id: parent
containsChild:
  text: Child Item
`,
			validate: func(t *testing.T, s *Selector) {
				if s.ContainsChild == nil || s.ContainsChild.Text != "Child Item" {
					t.Error("expected ContainsChild with text=Child Item")
				}
			},
		},
		{
			name: "relative selector - containsDescendants",
			yaml: `
id: container
containsDescendants:
  - text: First
  - text: Second
  - id: third
`,
			validate: func(t *testing.T, s *Selector) {
				if len(s.ContainsDescendants) != 3 {
					t.Fatalf("expected 3 descendants, got %d", len(s.ContainsDescendants))
				}
				if s.ContainsDescendants[0].Text != "First" {
					t.Error("expected first descendant text=First")
				}
				if s.ContainsDescendants[1].Text != "Second" {
					t.Error("expected second descendant text=Second")
				}
				if s.ContainsDescendants[2].ID != "third" {
					t.Error("expected third descendant id=third")
				}
			},
		},
		{
			name: "inline step properties",
			yaml: `
text: Submit
optional: true
retryTapIfNoChange: true
waitUntilVisible: false
point: "50%, 50%"
start: "10%, 50%"
end: "90%, 50%"
repeat: 3
delay: 100
waitToSettleTimeoutMs: 500
label: "submit button"
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Optional == nil || !*s.Optional {
					t.Error("expected optional=true")
				}
				if s.RetryTapIfNoChange == nil || !*s.RetryTapIfNoChange {
					t.Error("expected retryTapIfNoChange=true")
				}
				if s.WaitUntilVisible == nil || *s.WaitUntilVisible {
					t.Error("expected waitUntilVisible=false")
				}
				if s.Point != "50%, 50%" {
					t.Errorf("got Point=%q, want 50%%, 50%%", s.Point)
				}
				if s.Start != "10%, 50%" {
					t.Errorf("got Start=%q, want 10%%, 50%%", s.Start)
				}
				if s.End != "90%, 50%" {
					t.Errorf("got End=%q, want 90%%, 50%%", s.End)
				}
				if s.Repeat != 3 {
					t.Errorf("got Repeat=%d, want 3", s.Repeat)
				}
				if s.Delay != 100 {
					t.Errorf("got Delay=%d, want 100", s.Delay)
				}
				if s.WaitToSettleTimeoutMs != 500 {
					t.Errorf("got WaitToSettleTimeoutMs=%d, want 500", s.WaitToSettleTimeoutMs)
				}
				if s.Label != "submit button" {
					t.Errorf("got Label=%q, want submit button", s.Label)
				}
			},
		},
		{
			name: "nested relative selectors",
			yaml: `
text: OK
below:
  id: dialog-title
  rightOf:
    text: Warning
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Below == nil {
					t.Fatal("expected Below")
				}
				if s.Below.ID != "dialog-title" {
					t.Errorf("got Below.ID=%q, want dialog-title", s.Below.ID)
				}
				if s.Below.RightOf == nil {
					t.Fatal("expected Below.RightOf")
				}
				if s.Below.RightOf.Text != "Warning" {
					t.Errorf("got Below.RightOf.Text=%q, want Warning", s.Below.RightOf.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Selector
			if err := yaml.Unmarshal([]byte(tt.yaml), &s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.validate(t, &s)
		})
	}
}

func TestSelector_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		selector Selector
		expected bool
	}{
		{
			name:     "empty selector",
			selector: Selector{},
			expected: true,
		},
		{
			name:     "text set",
			selector: Selector{Text: "Login"},
			expected: false,
		},
		{
			name:     "id set",
			selector: Selector{ID: "btn"},
			expected: false,
		},
		{
			name:     "css set",
			selector: Selector{CSS: "#login"},
			expected: false,
		},
		{
			name:     "width set",
			selector: Selector{Width: 100},
			expected: false,
		},
		{
			name:     "height set",
			selector: Selector{Height: 50},
			expected: false,
		},
		{
			name:     "below set",
			selector: Selector{Below: &Selector{Text: "Header"}},
			expected: false,
		},
		{
			name:     "above set",
			selector: Selector{Above: &Selector{Text: "Footer"}},
			expected: false,
		},
		{
			name:     "leftOf set",
			selector: Selector{LeftOf: &Selector{Text: "Right"}},
			expected: false,
		},
		{
			name:     "rightOf set",
			selector: Selector{RightOf: &Selector{Text: "Left"}},
			expected: false,
		},
		{
			name:     "childOf set",
			selector: Selector{ChildOf: &Selector{ID: "parent"}},
			expected: false,
		},
		{
			name:     "containsChild set",
			selector: Selector{ContainsChild: &Selector{Text: "Child"}},
			expected: false,
		},
		{
			name:     "containsDescendants set",
			selector: Selector{ContainsDescendants: []*Selector{{Text: "Desc"}}},
			expected: false,
		},
		{
			name:     "only index set - still empty for matching",
			selector: Selector{Index: "1"},
			expected: true,
		},
		{
			name:     "only traits set - still empty for matching",
			selector: Selector{Traits: "button"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsEmpty()
			if got != tt.expected {
				t.Errorf("IsEmpty()=%v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSelector_HasRelativeSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector Selector
		expected bool
	}{
		{
			name:     "no relative selectors",
			selector: Selector{Text: "Login"},
			expected: false,
		},
		{
			name:     "childOf set",
			selector: Selector{ChildOf: &Selector{ID: "parent"}},
			expected: true,
		},
		{
			name:     "below set",
			selector: Selector{Below: &Selector{Text: "Header"}},
			expected: true,
		},
		{
			name:     "above set",
			selector: Selector{Above: &Selector{Text: "Footer"}},
			expected: true,
		},
		{
			name:     "leftOf set",
			selector: Selector{LeftOf: &Selector{Text: "Right"}},
			expected: true,
		},
		{
			name:     "rightOf set",
			selector: Selector{RightOf: &Selector{Text: "Left"}},
			expected: true,
		},
		{
			name:     "containsChild set",
			selector: Selector{ContainsChild: &Selector{Text: "Child"}},
			expected: true,
		},
		{
			name:     "containsDescendants set",
			selector: Selector{ContainsDescendants: []*Selector{{Text: "Desc"}}},
			expected: true,
		},
		{
			name:     "empty containsDescendants",
			selector: Selector{ContainsDescendants: []*Selector{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.HasRelativeSelector()
			if got != tt.expected {
				t.Errorf("HasRelativeSelector()=%v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSelector_Describe(t *testing.T) {
	tests := []struct {
		name     string
		selector Selector
		expected string
	}{
		{
			name:     "empty selector",
			selector: Selector{},
			expected: "",
		},
		{
			name:     "text selector",
			selector: Selector{Text: "Login"},
			expected: "Login",
		},
		{
			name:     "id selector",
			selector: Selector{ID: "login-btn"},
			expected: "#login-btn",
		},
		{
			name:     "css selector",
			selector: Selector{CSS: "#form input"},
			expected: "css:#form input",
		},
		{
			name:     "text takes precedence over id",
			selector: Selector{Text: "Submit", ID: "submit-btn"},
			expected: "Submit",
		},
		{
			name:     "id takes precedence over css",
			selector: Selector{ID: "btn", CSS: "#btn"},
			expected: "#btn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.Describe()
			if got != tt.expected {
				t.Errorf("Describe()=%q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSelector_DescribeQuoted(t *testing.T) {
	tests := []struct {
		name     string
		selector Selector
		expected string
	}{
		{
			name:     "empty selector",
			selector: Selector{},
			expected: "",
		},
		{
			name:     "text selector",
			selector: Selector{Text: "Login"},
			expected: `text="Login"`,
		},
		{
			name:     "id selector",
			selector: Selector{ID: "login-btn"},
			expected: `id="login-btn"`,
		},
		{
			name:     "css selector",
			selector: Selector{CSS: "#form input"},
			expected: `css="#form input"`,
		},
		{
			name:     "text takes precedence over id",
			selector: Selector{Text: "Submit", ID: "submit-btn"},
			expected: `text="Submit"`,
		},
		{
			name:     "id takes precedence over css",
			selector: Selector{ID: "btn", CSS: "#btn"},
			expected: `id="btn"`,
		},
		{
			name:     "text with special characters",
			selector: Selector{Text: `Hello "World"`},
			expected: `text="Hello "World""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.DescribeQuoted()
			if got != tt.expected {
				t.Errorf("DescribeQuoted() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSelector_DescribeQuoted_InsideOf(t *testing.T) {
	s := Selector{InsideOf: &Selector{ID: "container"}}
	// InsideOf alone does not set text/id/css, so IsEmpty is false but DescribeQuoted returns ""
	got := s.DescribeQuoted()
	if got != "" {
		t.Errorf("DescribeQuoted() = %q, want empty for insideOf-only selector", got)
	}
}

func TestSelector_UnmarshalYAML_Invalid(t *testing.T) {
	invalidYAML := `
text: valid
invalid_nested:
  - not: valid
    yaml: [structure
`
	var s Selector
	err := yaml.Unmarshal([]byte(invalidYAML), &s)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
