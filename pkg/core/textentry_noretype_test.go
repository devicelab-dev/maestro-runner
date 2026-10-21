package core

import (
	"strings"
	"testing"
)

type shortenedField struct {
	text   string
	inputs []string
}

func (f *shortenedField) Text() (string, error) { return f.text, nil }
func (f *shortenedField) Input(s string) error {
	f.inputs = append(f.inputs, s)
	f.text = s
	return nil
}
func (f *shortenedField) Clear() error { f.text = ""; return nil }

// With MAESTRO_RUNNER_NO_RETYPE a field the app shortened is left as it is.
func TestConfirmTypedTextNoRetype(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_NO_RETYPE", "1")
	f := &shortenedField{text: "3,23"}
	note := ConfirmTypedText(f, "3,23999", "", nil)
	if len(f.inputs) != 0 || f.text != "3,23" {
		t.Fatalf("retyped: inputs %v, text %q", f.inputs, f.text)
	}
	if !strings.Contains(note, "not retyped") {
		t.Fatalf("note = %q", note)
	}
}
