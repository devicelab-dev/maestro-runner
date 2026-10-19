package core

import (
	"encoding/xml"
	"testing"
)

func TestSanitizeXML(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"clean", `<node text="Hello"/>`, `<node text="Hello"/>`},
		{"tab newline cr kept", "<n text=\"a\tb\nc\rd\"/>", "<n text=\"a\tb\nc\rd\"/>"},
		{"multibyte kept", `<n text="héllo 日本 🎉"/>`, `<n text="héllo 日本 🎉"/>`},
		{"nul", "<n text=\"a\x00b\"/>", `<n text="a` + "�" + `b"/>`},
		{"control", "<n text=\"\x1b[0m\"/>", `<n text="` + "�" + `[0m"/>`},
		{"invalid utf8", "<n text=\"a\xffb\"/>", `<n text="a` + "�" + `b"/>`},
		{"U+FFFE", "<n text=\"a￾b\"/>", `<n text="a` + "�" + `b"/>`},
		{"decimal ref", `<n text="a&#0;b"/>`, `<n text="a` + "�" + `b"/>`},
		{"hex ref", `<n text="a&#x1B;b"/>`, `<n text="a` + "�" + `b"/>`},
		{"valid refs kept", `<n text="&#10;&#x41;&amp;&lt;"/>`, `<n text="&#10;&#x41;&amp;&lt;"/>`},
		{"malformed ref kept", `<n text="&#zz;"/>`, `<n text="&#zz;"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeXML(tc.in); got != tc.want {
				t.Errorf("SanitizeXML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Every case that encoding/xml rejects must parse once sanitized.
func TestSanitizeXMLParses(t *testing.T) {
	for _, in := range []string{
		"<n text=\"a\x00b\"/>", "<n>\x07</n>", "<n text=\"\xc3\"/>", `<n text="&#0;"/>`, `<n text="&#xFFFF;"/>`,
	} {
		var v struct {
			Text string `xml:"text,attr"`
		}
		if err := xml.Unmarshal([]byte(in), &v); err == nil {
			t.Fatalf("%q: expected the raw input to be rejected", in)
		}
		if err := xml.Unmarshal([]byte(SanitizeXML(in)), &v); err != nil {
			t.Errorf("%q: sanitized input should parse: %v", in, err)
		}
	}
}

func TestSanitizeXMLCleanInputNotCopied(t *testing.T) {
	in := `<hierarchy><node text="ok"/></hierarchy>`
	if allocs := testing.AllocsPerRun(10, func() { _ = SanitizeXML(in) }); allocs != 0 {
		t.Errorf("clean input allocated %v times, want 0", allocs)
	}
}
