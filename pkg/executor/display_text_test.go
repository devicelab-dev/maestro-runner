package executor

import "testing"

// Step names fill in values defined in the flow files and never ones given
// from outside, which may be secrets.
func TestDisplayText(t *testing.T) {
	se := NewScriptEngine()
	se.SetExternalVariables(map[string]string{"PASSWORD": "hunter2secret", "SHORT": "1"})
	se.SetVariable("TEXT", "Article by Gandalf")
	se.SetVariable("PASS", "hunter2secret")         // copied from PASSWORD
	se.SetVariable("GREETING", "Hi hunter2secret!") // carries it
	se.SetVariable("COUNT", "1")                    // equals a short external value
	se.SetVariable("NAME", "alice 1")               // contains a short one: fine

	tests := []struct{ in, want string }{
		{`tapOn: text="${TEXT}"`, `tapOn: text="Article by Gandalf"`},
		{`inputText: "${PASSWORD}"`, `inputText: "${PASSWORD}"`},
		{`inputText: "${PASS}"`, `inputText: "${PASS}"`},
		{`tapOn: "${GREETING}"`, `tapOn: "${GREETING}"`},
		{`tapOn: "${COUNT}"`, `tapOn: "${COUNT}"`},
		{`tapOn: "${NAME}"`, `tapOn: "alice 1"`},
		{`tapOn: "${UNDEFINED}"`, `tapOn: "${UNDEFINED}"`},
		{`tapOn: "${output.x}"`, `tapOn: "${output.x}"`},
		{`tapOn: "${TEXT + 1}"`, `tapOn: "${TEXT + 1}"`},
		{`no variables`, `no variables`},
	}
	for _, tt := range tests {
		if got := se.DisplayText(tt.in); got != tt.want {
			t.Errorf("DisplayText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
