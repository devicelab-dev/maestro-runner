package core

import "strings"

// Quoting for values that reach a device shell.
//
// "adb shell <cmd>" hands the whole command string to the device's shell, so
// anything a flow supplies — a deep link, a launch argument, a pushed file's
// name — is re-parsed there. Interpolating those values into a command with
// bare surrounding quotes works right up until the value contains a quote of
// its own, at which point the word ends early and the rest of the value is read
// as shell syntax rather than data.
//
// The values involved are author-supplied YAML, not untrusted input, so the
// failure this prevents is a broken command rather than an injection: a URL
// like https://example.com/search?q=it's is a shell parse error today, and an
// unquoted path means addMedia cannot handle a file with a space in its name.

// ShellQuote renders s as a single shell word, safe to interpolate into a
// command destined for a device shell.
//
// It always quotes rather than quoting only when a metacharacter is present:
// the callers all interpolate into a fixed position where a quoted word is
// valid, and an unconditional rule has no "is this character safe?" list to
// keep correct. Note that callers must NOT add their own quotes around the
// result — the quotes are part of it.
//
// Single quotes suppress every form of shell expansion, so the only character
// needing special treatment is the single quote itself. It cannot be escaped
// inside a single-quoted string, so the standard construction closes the
// string, contributes an escaped quote, and reopens, which the shell then
// concatenates back into one word:
//
//	it's  ->  'it'\''s'
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
