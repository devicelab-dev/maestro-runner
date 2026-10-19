package core

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// SanitizeXML returns xmlData with every character XML 1.0 forbids replaced
// by U+FFFD, so encoding/xml can parse it. Android page source carries app
// text as it is: one U+0000 in a label, or a stray control character, made
// the whole document unparseable and every lookup on that screen fail. A
// numeric reference to such a character (&#0;, &#x1B;) is replaced too, since
// the decoder rejects it the same way, and so is each byte that is not UTF-8.
//
// U+FFFD rather than "?" or nothing: a cleaned value must not come to match a
// selector it would not have matched.
//
// A document with nothing to replace is returned as it is, without a copy.
func SanitizeXML(xmlData string) string {
	var b *strings.Builder
	for i := 0; i < len(xmlData); {
		size, bad := 0, false
		if xmlData[i] == '&' {
			size, bad = badCharRef(xmlData[i:])
		}
		if !bad {
			var r rune
			r, size = utf8.DecodeRuneInString(xmlData[i:])
			bad = (r == utf8.RuneError && size == 1) || !isXMLChar(r)
		}
		if bad && b == nil {
			b = &strings.Builder{}
			b.Grow(len(xmlData))
			b.WriteString(xmlData[:i])
		}
		if b != nil {
			if bad {
				b.WriteRune(utf8.RuneError)
			} else {
				b.WriteString(xmlData[i : i+size])
			}
		}
		i += size
	}
	if b == nil {
		return xmlData
	}
	return b.String()
}

// badCharRef reports the length of the numeric character reference s starts
// with, and whether it names a character XML 1.0 forbids. A malformed
// reference is left for the decoder to report.
func badCharRef(s string) (int, bool) {
	if !strings.HasPrefix(s, "&#") {
		return 0, false
	}
	end := strings.IndexByte(s, ';')
	if end < 0 || end > 12 {
		return 0, false
	}
	digits, base := s[2:end], 10
	if strings.HasPrefix(digits, "x") {
		digits, base = digits[1:], 16
	}
	n, err := strconv.ParseUint(digits, base, 32)
	if err != nil || n > utf8.MaxRune || isXMLChar(rune(n)) {
		return 0, false
	}
	return end + 1, true
}

// isXMLChar reports whether r is in XML 1.0's Char production, the range
// encoding/xml accepts.
func isXMLChar(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= 0x10FFFF
}
