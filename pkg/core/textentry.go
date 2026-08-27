package core

import "strings"

// TextEntryVerdict is what a read-back of a text field after typing tells us.
type TextEntryVerdict int

const (
	// TextEntryLanded — the field contains what was typed.
	TextEntryLanded TextEntryVerdict = iota
	// TextEntryUnverifiable — the field's value could not be read, or reading
	// it tells us nothing. Not a failure: plenty of custom inputs report no
	// value at all, and a flow must not start failing because of that.
	TextEntryUnverifiable
	// TextEntryDropped — the field holds less than was typed, so characters
	// were lost in flight. The usual cause is a keystroke dropped by a janky
	// frame, and retyping fixes it.
	TextEntryDropped
	// TextEntryTransformed — the field holds something different but not
	// shorter, which is what a formatter, an autocomplete or an input mask
	// does. Retyping would only produce the same result, so callers leave it
	// alone.
	TextEntryTransformed
)

// VerifyTypedText decides whether typed actually landed in a field that read as
// before beforehand and reads as after now.
//
// Both readings are needed because a field's value accessor is not always a
// value accessor: on some drivers an empty input reports its hint, so "Username"
// comes back whether or not anything was typed. A reading that did not change
// therefore proves nothing, and is reported as unverifiable rather than acted
// on.
//
// The check is deliberately a suffix rather than an equality: every mobile text
// path appends to whatever the field already held, and the caller would
// otherwise have to spend a round trip reading the field before typing just to
// know what to expect.
//
// Two shapes are accepted beyond an exact suffix:
//
//   - A secure field reports its value as bullets, one per character. Comparing
//     text would fail on every password in every suite, so a masked value of the
//     right length counts as landed — and the secret is never read.
//   - A value that differs but is no shorter than what was typed has been
//     rewritten by the app (a formatter, an autocomplete, an input mask).
//     Retyping reproduces it, so it is reported separately from a real loss.
//   - A field that reports no value at all is unverifiable, not failed. Custom
//     inputs, Flutter's EditableText and some WebView controls all do this.
//
// A value that is present, unmasked and does not end with what was typed is the
// case worth acting on: characters were dropped in flight.
func VerifyTypedText(typed, before, after string, readOK bool) TextEntryVerdict {
	if !readOK || typed == "" {
		return TextEntryUnverifiable
	}
	if after == before {
		// The field reads exactly as it did before typing. Either nothing
		// landed, or this accessor does not report the entered value at all —
		// several drivers return the hint or label of an empty field, so
		// "Username" comes back whether or not a name was typed. The two are
		// indistinguishable from here, and retyping on the strength of a
		// static label would clear a field for no reason.
		return TextEntryUnverifiable
	}
	if after == "" {
		// Nothing came back. Either the field genuinely reports no value, or
		// every character was lost. The two are indistinguishable from here,
		// and treating a silent control as a failure would break more flows
		// than it fixes.
		return TextEntryUnverifiable
	}
	if strings.HasSuffix(after, typed) {
		return TextEntryLanded
	}
	// Below here the value is present and is not what was typed. Length
	// separates the two reasons that happens. Anything at least as long as
	// what was typed has been rewritten — by a mask, a formatter or an
	// autocomplete — and retyping would reproduce it exactly. Anything
	// shorter is missing characters, which is the case worth retrying.
	if len([]rune(after)) < len([]rune(typed)) {
		return TextEntryDropped
	}
	if isMasked(after) {
		return TextEntryLanded
	}
	return TextEntryTransformed
}

// maskCharacters are what the platforms substitute for a secure field's real
// value: the bullet iOS and Android report, and the asterisk some custom
// controls use.
const maskCharacters = "•●*·"

// isMasked reports whether every character of value is a masking glyph, which
// is how a secure field's value comes back.
func isMasked(value string) bool {
	for _, r := range value {
		if !strings.ContainsRune(maskCharacters, r) {
			return false
		}
	}
	return value != ""
}

// TextField is the slice of an element a text-entry check needs. core.Element
// satisfies it; drivers whose element type spells typing differently adapt in a
// few lines.
type TextField interface {
	Text() (string, error)
	Input(text string) error
	Clear() error
}

// TextFieldFuncs adapts any element to TextField from its three operations, so
// a driver whose element type spells typing differently — SendKeys rather than
// Input — needs no wrapper type of its own.
func TextFieldFuncs(text func() (string, error), input func(string) error, clear func() error) TextField {
	return funcTextField{text: text, input: input, clear: clear}
}

type funcTextField struct {
	text  func() (string, error)
	input func(string) error
	clear func() error
}

func (f funcTextField) Text() (string, error) { return f.text() }
func (f funcTextField) Input(s string) error  { return f.input(s) }
func (f funcTextField) Clear() error          { return f.clear() }

// ConfirmTypedText reads a field back after typing and, if characters were lost
// on the way in, types them again. before is the field's reading from just
// before typing, which is what makes a static hint distinguishable from a real
// value. It returns a note for the step result —
// empty when everything landed.
//
// Retyping is the point: a dropped keystroke is a timing accident and the
// second attempt almost always succeeds. What this must not do is turn
// legitimate app behaviour into a failure, so a field reporting no value, or
// one the app has reformatted, is left alone — and a field that still disagrees
// after the retry is reported in the result rather than failed, because the
// assertion that follows will describe the real problem better than a driver
// can.
func ConfirmTypedText(field TextField, typed, before string, warn func(format string, args ...interface{})) string {
	if field == nil {
		return ""
	}

	after, err := field.Text()
	if VerifyTypedText(typed, before, after, err == nil) != TextEntryDropped {
		return ""
	}

	if warn != nil {
		warn("inputText: field holds %q after typing %q — retyping", after, typed)
	}
	if clearErr := field.Clear(); clearErr != nil {
		// Without the clear, retyping would append to the partial value and
		// leave the field worse than it started.
		return " (warning: characters were dropped and the field could not be cleared to retry)"
	}
	if inputErr := field.Input(typed); inputErr != nil {
		return " (warning: characters were dropped and retyping failed)"
	}

	retyped, retypedErr := field.Text()
	if VerifyTypedText(typed, before, retyped, retypedErr == nil) == TextEntryDropped {
		return " (warning: characters are still missing after retyping)"
	}
	return " (retyped after dropped characters)"
}
