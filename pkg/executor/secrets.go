package executor

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// secretMask replaces a secret's value in what a typing step reports.
const secretMask = "***"

// minSecretLen is the shortest value masked: a one- or two-character value
// would turn up in nearly every message.
const minSecretLen = 3

// secretWords are the words of a variable name that make its value a secret.
// "apikey" covers a name written as one word; API_KEY and apiKey split into
// "api" and "key".
var secretWords = map[string]bool{
	"password": true, "passwd": true, "pass": true, "secret": true, "token": true,
	"apikey": true, "key": true, "pin": true, "otp": true, "credential": true,
}

// isSecretName reports whether a variable's name marks its value as a secret:
// one of its words, split on non-letters and camelCase, is a secret word
// (case-insensitive, plural too). API_KEY, userPassword, PIN and OTP_CODE are
// secrets; MONKEY, PINNED and PASSENGER are not.
func isSecretName(name string) bool {
	for _, w := range nameWords(name) {
		w = strings.ToLower(w)
		if secretWords[w] || (strings.HasSuffix(w, "s") && secretWords[strings.TrimSuffix(w, "s")]) {
			return true
		}
	}
	return false
}

// nameWords splits an identifier into its words: at anything that is not a
// letter, at a lower-to-upper change (userPassword) and before the last
// capital of an acronym followed by lower case (APIKey → API, Key).
func nameWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = cur[:0]
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		if !unicode.IsLetter(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && len(cur) > 0 {
			prev := cur[len(cur)-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// secretValues returns the values of the defined variables whose names mark
// them as secrets, longest first so a secret that contains another is masked
// whole.
func (se *ScriptEngine) secretValues() []string {
	if se == nil {
		return nil
	}
	seen := map[string]bool{}
	var values []string
	for name, value := range se.variables {
		if utf8.RuneCountInString(value) < minSecretLen || seen[value] || !isSecretName(name) {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}
		return values[i] < values[j]
	})
	return values
}

// MaskSecrets replaces in s the value of every variable whose name marks it
// as a secret (see isSecretName) with "***".
func (se *ScriptEngine) MaskSecrets(s string) string {
	if s == "" {
		return s
	}
	return maskValues(se.secretValues(), s)
}

// maskValues replaces each of values in s with "***".
func maskValues(values []string, s string) string {
	for _, v := range values {
		s = strings.ReplaceAll(s, v, secretMask)
	}
	return s
}

// typesText reports whether a step types or holds text a flow may fill from
// a secret: its name, message and error get the secrets masked.
func typesText(step flow.Step) bool {
	switch step.(type) {
	case *flow.InputTextStep, *flow.PasteTextStep, *flow.SetClipboardStep:
		return true
	}
	return false
}

// maskTypedName masks the secrets in a typing step's name: the name fills in
// flow-defined values, so `inputText: ${PASSWORD}` with the password set in
// the flow's env: would show it.
func (fr *FlowRunner) maskTypedName(step flow.Step, name string) string {
	if !typesText(step) {
		return name
	}
	return fr.script.MaskSecrets(name)
}

// maskTypedResult masks the secrets in a typing step's message and error.
// Drivers echo the typed text ("Entered text: …") to whatever reads the
// result, and a failure can carry it into report.json, the HTML, JUnit and
// Allure reports, the console and the log. The text typed is unchanged:
// this runs after the driver.
func (fr *FlowRunner) maskTypedResult(step flow.Step, result *core.CommandResult) *core.CommandResult {
	if result == nil || !typesText(step) {
		return result
	}
	secrets := fr.script.secretValues()
	if len(secrets) == 0 {
		return result
	}
	result.Message = maskValues(secrets, result.Message)
	if result.Error != nil {
		if masked := maskValues(secrets, result.Error.Error()); masked != result.Error.Error() {
			result.Error = &maskedError{msg: masked, err: result.Error}
		}
	}
	return result
}

// maskedError is an error whose text has its secrets masked; errors.Is and
// errors.As still see the error underneath.
type maskedError struct {
	msg string
	err error
}

func (e *maskedError) Error() string { return e.msg }
func (e *maskedError) Unwrap() error { return e.err }
