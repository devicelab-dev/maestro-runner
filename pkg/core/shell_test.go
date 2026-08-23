package core

import (
	"os/exec"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain word", "hello", `'hello'`},
		{"empty stays a word", "", `''`},
		{"space", "my photo.jpg", `'my photo.jpg'`},
		{"apostrophe", "it's", `'it'\''s'`},
		{"only an apostrophe", "'", `''\'''`},
		{"double quote is inert inside single quotes", `say "hi"`, `'say "hi"'`},
		{"dollar is not expanded", "$HOME", `'$HOME'`},
		{"command substitution is inert", "$(rm -rf /)", `'$(rm -rf /)'`},
		{"semicolon cannot end the command", "a; b", `'a; b'`},
		{"url with query", "https://example.com/s?q=it's&x=1", `'https://example.com/s?q=it'\''s&x=1'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShellQuote(tt.in); got != tt.want {
				t.Errorf("ShellQuote(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestShellQuoteRoundTripsThroughARealShell is the assertion that matters: the
// quoted form must arrive at the far side as the original string, one argument,
// unexpanded. The device shell is what actually parses these, so checking the
// escaping against a real `sh` proves the construction rather than restating it.
func TestShellQuoteRoundTrips(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	inputs := []string{
		"hello",
		"",
		"my photo.jpg",
		"it's",
		"'",
		`mixed "double" and 'single'`,
		"$HOME",
		"$(echo pwned)",
		"`echo pwned`",
		"a; echo pwned",
		"a && echo pwned",
		"tab\tand newline\nhere",
		"https://example.com/search?q=it's&lang=en",
		"emoji 🎉 and ünïcödé",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			// printf %s with a single conversion echoes exactly one argument,
			// so extra words produced by bad quoting would be dropped and a
			// truncated value would show up as a mismatch either way.
			out, err := exec.Command(sh, "-c", "printf %s "+ShellQuote(in)).Output()
			if err != nil {
				t.Fatalf("sh rejected the quoted form of %q: %v", in, err)
			}
			if string(out) != in {
				t.Errorf("round trip of %q gave %q", in, string(out))
			}
		})
	}
}

// TestShellQuoteKeepsValueAsOneArgument guards the multi-word case separately:
// unquoted text splits into several arguments, which is how `--es key a b` and
// `--arg my photo.jpg` silently lose their tail.
func TestShellQuoteKeepsValueAsOneArgument(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	// $# counts the positional arguments the shell parsed.
	out, err := exec.Command(sh, "-c", `set -- `+ShellQuote("one two three")+`; echo $#`).Output()
	if err != nil {
		t.Fatalf("sh failed: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "1" {
		t.Errorf("quoted multi-word value became %s arguments, want 1", got)
	}
}
