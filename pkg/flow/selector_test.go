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
		{
			name:     "testId selector",
			selector: Selector{TestID: "submit-btn"},
			expected: `testId="submit-btn"`,
		},
		{
			name:     "role selector",
			selector: Selector{Role: "button"},
			expected: `role="button"`,
		},
		{
			name:     "placeholder selector",
			selector: Selector{Placeholder: "Search..."},
			expected: `placeholder="Search..."`,
		},
		{
			name:     "href selector",
			selector: Selector{Href: "/about"},
			expected: `href="/about"`,
		},
		{
			name:     "alt selector",
			selector: Selector{Alt: "Logo"},
			expected: `alt="Logo"`,
		},
		{
			name:     "title selector",
			selector: Selector{Title: "Info"},
			expected: `title="Info"`,
		},
		{
			name:     "name selector",
			selector: Selector{Name: "email"},
			expected: `name="email"`,
		},
		{
			name:     "textContains selector",
			selector: Selector{TextContains: "Welcome"},
			expected: `textContains="Welcome"`,
		},
		{
			name:     "textRegex selector",
			selector: Selector{TextRegex: "Hello.*"},
			expected: `textRegex="Hello.*"`,
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

func TestSelector_HasNonZeroIndex(t *testing.T) {
	tests := []struct {
		name     string
		index    string
		expected bool
	}{
		{"empty index", "", false},
		{"zero index", "0", false},
		{"positive index", "1", true},
		{"negative index", "-1", true},
		{"large index", "99", true},
		{"non-numeric index", "abc", false},
		{"variable reference", "${idx}", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Selector{Index: tt.index}
			got := s.HasNonZeroIndex()
			if got != tt.expected {
				t.Errorf("HasNonZeroIndex()=%v, want %v for index=%q", got, tt.expected, tt.index)
			}
		})
	}
}

func TestSelector_EffectiveNth(t *testing.T) {
	tests := []struct {
		name  string
		index string
		nth   int
		want  int
	}{
		{name: "both unset", want: 0},
		{name: "nth set, no index", nth: 2, want: 2},
		{name: "index set as string, no nth", index: "3", want: 3},
		{name: "index set with whitespace", index: "  4  ", want: 4},
		{name: "both set: nth wins", nth: 5, index: "9", want: 5},
		{name: "index zero, nth zero", index: "0", nth: 0, want: 0},
		{name: "negative index treated as 0", index: "-1", want: 0},
		{name: "non-numeric index treated as 0", index: "abc", want: 0},
		{name: "variable reference treated as 0", index: "${idx}", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Selector{Nth: tt.nth, Index: tt.index}
			if got := s.EffectiveNth(); got != tt.want {
				t.Errorf("EffectiveNth() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSelector_WebSupportsIndex guards the parity behavior reported in #67:
// flows that use `index: N` (the mobile-style spelling) on a web selector
// should not be flagged as unsupported. Regression in this list is what
// caused upstream-Maestro flows to silently tap the wrong element on web.
func TestSelector_WebSupportsIndex(t *testing.T) {
	s := &Selector{Text: "Help", Index: "1"}
	unsupported := CheckUnsupportedFields(s, "web")
	for _, f := range unsupported {
		if f == "index" {
			t.Errorf("CheckUnsupportedFields(web) reported %q as unsupported; should be allowed (#67)", f)
		}
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

func TestSelector_UnmarshalYAML_NewFields(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		validate func(t *testing.T, s *Selector)
	}{
		{
			name: "placeholder",
			yaml: `placeholder: "Search..."`,
			validate: func(t *testing.T, s *Selector) {
				if s.Placeholder != "Search..." {
					t.Errorf("got Placeholder=%q, want Search...", s.Placeholder)
				}
			},
		},
		{
			name: "role",
			yaml: `role: button`,
			validate: func(t *testing.T, s *Selector) {
				if s.Role != "button" {
					t.Errorf("got Role=%q, want button", s.Role)
				}
			},
		},
		{
			name: "textContains",
			yaml: `textContains: Welcome`,
			validate: func(t *testing.T, s *Selector) {
				if s.TextContains != "Welcome" {
					t.Errorf("got TextContains=%q, want Welcome", s.TextContains)
				}
			},
		},
		{
			name: "href",
			yaml: `href: /about`,
			validate: func(t *testing.T, s *Selector) {
				if s.Href != "/about" {
					t.Errorf("got Href=%q, want /about", s.Href)
				}
			},
		},
		{
			name: "alt",
			yaml: `alt: "Company logo"`,
			validate: func(t *testing.T, s *Selector) {
				if s.Alt != "Company logo" {
					t.Errorf("got Alt=%q, want Company logo", s.Alt)
				}
			},
		},
		{
			name: "title",
			yaml: `title: "More info"`,
			validate: func(t *testing.T, s *Selector) {
				if s.Title != "More info" {
					t.Errorf("got Title=%q, want More info", s.Title)
				}
			},
		},
		{
			name: "name",
			yaml: `name: email`,
			validate: func(t *testing.T, s *Selector) {
				if s.Name != "email" {
					t.Errorf("got Name=%q, want email", s.Name)
				}
			},
		},
		{
			name: "testId",
			yaml: `testId: submit-btn`,
			validate: func(t *testing.T, s *Selector) {
				if s.TestID != "submit-btn" {
					t.Errorf("got TestID=%q, want submit-btn", s.TestID)
				}
			},
		},
		{
			name: "textRegex",
			yaml: `textRegex: "Welcome.*"`,
			validate: func(t *testing.T, s *Selector) {
				if s.TextRegex != "Welcome.*" {
					t.Errorf("got TextRegex=%q, want Welcome.*", s.TextRegex)
				}
			},
		},
		{
			name: "nth",
			yaml: `
css: ".item"
nth: 2
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Nth != 2 {
					t.Errorf("got Nth=%d, want 2", s.Nth)
				}
				if s.CSS != ".item" {
					t.Errorf("got CSS=%q, want .item", s.CSS)
				}
			},
		},
		{
			name: "role with text",
			yaml: `
role: button
text: Submit
`,
			validate: func(t *testing.T, s *Selector) {
				if s.Role != "button" {
					t.Errorf("got Role=%q, want button", s.Role)
				}
				if s.Text != "Submit" {
					t.Errorf("got Text=%q, want Submit", s.Text)
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

func TestSelector_Describe_NewFields(t *testing.T) {
	tests := []struct {
		name     string
		selector Selector
		expected string
	}{
		{
			name:     "testId",
			selector: Selector{TestID: "submit-btn"},
			expected: "testId:submit-btn",
		},
		{
			name:     "role",
			selector: Selector{Role: "button"},
			expected: "role:button",
		},
		{
			name:     "placeholder",
			selector: Selector{Placeholder: "Search..."},
			expected: "placeholder:Search...",
		},
		{
			name:     "href",
			selector: Selector{Href: "/about"},
			expected: "href:/about",
		},
		{
			name:     "alt",
			selector: Selector{Alt: "Logo"},
			expected: "alt:Logo",
		},
		{
			name:     "title",
			selector: Selector{Title: "Info"},
			expected: "title:Info",
		},
		{
			name:     "name",
			selector: Selector{Name: "email"},
			expected: "name:email",
		},
		{
			name:     "textContains",
			selector: Selector{TextContains: "Welcome"},
			expected: "textContains:Welcome",
		},
		{
			name:     "textRegex",
			selector: Selector{TextRegex: "Hello.*"},
			expected: "textRegex:Hello.*",
		},
		{
			name:     "text takes precedence over new fields",
			selector: Selector{Text: "Login", Role: "button"},
			expected: "Login",
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

func TestSelector_IsEmpty_NewFields(t *testing.T) {
	tests := []struct {
		name     string
		selector Selector
		expected bool
	}{
		{name: "placeholder set", selector: Selector{Placeholder: "Search"}, expected: false},
		{name: "role set", selector: Selector{Role: "button"}, expected: false},
		{name: "textContains set", selector: Selector{TextContains: "Hello"}, expected: false},
		{name: "href set", selector: Selector{Href: "/about"}, expected: false},
		{name: "alt set", selector: Selector{Alt: "Logo"}, expected: false},
		{name: "title set", selector: Selector{Title: "Info"}, expected: false},
		{name: "name set", selector: Selector{Name: "email"}, expected: false},
		{name: "testId set", selector: Selector{TestID: "btn"}, expected: false},
		{name: "textRegex set", selector: Selector{TextRegex: ".*"}, expected: false},
		{name: "only nth set - still empty", selector: Selector{Nth: 1}, expected: true},
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

func TestCheckUnsupportedFields(t *testing.T) {
	tests := []struct {
		name        string
		selector    Selector
		platform    string
		unsupported []string
	}{
		{
			name:        "text on android - supported",
			selector:    Selector{Text: "Login"},
			platform:    "android",
			unsupported: nil,
		},
		{
			name:        "placeholder on android - unsupported",
			selector:    Selector{Placeholder: "Search"},
			platform:    "android",
			unsupported: []string{"placeholder"},
		},
		{
			name:        "placeholder on web - supported",
			selector:    Selector{Placeholder: "Search"},
			platform:    "web",
			unsupported: nil,
		},
		{
			name:        "css on ios - unsupported",
			selector:    Selector{CSS: "#login"},
			platform:    "ios",
			unsupported: []string{"css"},
		},
		{
			name:        "checked on ios - unsupported",
			selector:    Selector{Checked: boolPtr(true)},
			platform:    "ios",
			unsupported: []string{"checked"},
		},
		{
			name:        "width on web - unsupported",
			selector:    Selector{Width: 100},
			platform:    "web",
			unsupported: []string{"width"},
		},
		{
			name:        "multiple unsupported on web",
			selector:    Selector{Width: 100, Height: 50, Tolerance: 5},
			platform:    "web",
			unsupported: []string{"width", "height", "tolerance"},
		},
		{
			name:        "relative selector on web - unsupported",
			selector:    Selector{Below: &Selector{Text: "Header"}},
			platform:    "web",
			unsupported: []string{"below"},
		},
		{
			name:        "role on web - supported",
			selector:    Selector{Role: "button"},
			platform:    "web",
			unsupported: nil,
		},
		{
			name:        "testId on web - supported",
			selector:    Selector{TestID: "submit"},
			platform:    "web",
			unsupported: nil,
		},
		{
			name:        "unknown platform - no warnings",
			selector:    Selector{Placeholder: "Search"},
			platform:    "unknown",
			unsupported: nil,
		},
		{
			name:        "empty selector - no warnings",
			selector:    Selector{},
			platform:    "android",
			unsupported: nil,
		},
		{
			name:        "traits on any platform - unsupported everywhere",
			selector:    Selector{Traits: "button"},
			platform:    "android",
			unsupported: []string{"traits"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckUnsupportedFields(&tt.selector, tt.platform)
			if len(got) != len(tt.unsupported) {
				t.Errorf("CheckUnsupportedFields()=%v, want %v", got, tt.unsupported)
				return
			}
			for i, field := range got {
				if field != tt.unsupported[i] {
					t.Errorf("CheckUnsupportedFields()[%d]=%q, want %q", i, field, tt.unsupported[i])
				}
			}
		})
	}
}

func TestExtractSelectors(t *testing.T) {
	tests := []struct {
		name     string
		step     Step
		expected int // number of selectors
	}{
		{
			name:     "TapOnStep",
			step:     &TapOnStep{Selector: Selector{Text: "Login"}},
			expected: 1,
		},
		{
			name:     "AssertVisibleStep",
			step:     &AssertVisibleStep{Selector: Selector{ID: "btn"}},
			expected: 1,
		},
		{
			name:     "WaitUntilStep with visible",
			step:     &WaitUntilStep{Visible: &Selector{Text: "Done"}},
			expected: 1,
		},
		{
			name:     "WaitUntilStep with both",
			step:     &WaitUntilStep{Visible: &Selector{Text: "Done"}, NotVisible: &Selector{Text: "Loading"}},
			expected: 2,
		},
		{
			name:     "BackStep - no selector",
			step:     &BackStep{},
			expected: 0,
		},
		{
			name:     "SwipeStep with selector",
			step:     &SwipeStep{Selector: &Selector{Text: "List"}},
			expected: 1,
		},
		{
			name:     "SwipeStep without selector",
			step:     &SwipeStep{},
			expected: 0,
		},
		{
			name:     "DoubleTapOnStep",
			step:     &DoubleTapOnStep{Selector: Selector{Text: "OK"}},
			expected: 1,
		},
		{
			name:     "LongPressOnStep",
			step:     &LongPressOnStep{Selector: Selector{ID: "item"}},
			expected: 1,
		},
		{
			name:     "ScrollUntilVisibleStep",
			step:     &ScrollUntilVisibleStep{Element: Selector{Text: "Bottom"}},
			expected: 1,
		},
		{
			name:     "InputTextStep",
			step:     &InputTextStep{Selector: Selector{ID: "input"}},
			expected: 1,
		},
		{
			name:     "CopyTextFromStep",
			step:     &CopyTextFromStep{Selector: Selector{Text: "Value"}},
			expected: 1,
		},
		{
			name:     "AssertNotVisibleStep",
			step:     &AssertNotVisibleStep{Selector: Selector{Text: "Error"}},
			expected: 1,
		},
		{
			name:     "AssertConditionStep with visible",
			step:     &AssertConditionStep{Condition: Condition{Visible: &Selector{Text: "OK"}}},
			expected: 1,
		},
		{
			name:     "AssertConditionStep with both",
			step:     &AssertConditionStep{Condition: Condition{Visible: &Selector{Text: "OK"}, NotVisible: &Selector{Text: "Error"}}},
			expected: 2,
		},
		{
			name:     "LaunchAppStep - no selector",
			step:     &LaunchAppStep{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSelectors(tt.step)
			if len(got) != tt.expected {
				t.Errorf("ExtractSelectors() returned %d selectors, want %d", len(got), tt.expected)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
