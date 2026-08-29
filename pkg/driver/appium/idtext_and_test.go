package appium

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// An id and a text together mean "the element with both". The page-source
// matcher is the only place every field of a selector is checked at once —
// the direct queries each answer for one attribute and return — so this pins
// that both are required and that neither alone is enough.
func TestMatchesSelectorRequiresIDAndTextOnTheSameElement(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.TextView resource-id="total" text="BREAKFAST" displayed="true" bounds="[0,0][100,50]" />
  <android.widget.TextView resource-id="total" text="LUNCH" displayed="true" bounds="[0,50][100,100]" />
</hierarchy>`

	elements, platform, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}

	both := flow.Selector{ID: "total", Text: "LUNCH"}
	got := FilterBySelector(elements, both, platform)
	if len(got) != 1 {
		t.Fatalf("id+text matched %d elements, want exactly the one with both", len(got))
	}
	if got[0].Text != "LUNCH" {
		t.Errorf("matched the element reading %q, want LUNCH", got[0].Text)
	}

	// The failure that made this dangerous: a right id with a wrong text used
	// to pass, because the id query answered first and the text was never read.
	wrongText := flow.Selector{ID: "total", Text: "NOT ON SCREEN"}
	if n := len(FilterBySelector(elements, wrongText, platform)); n != 0 {
		t.Errorf("a right id with a wrong text matched %d elements, want 0", n)
	}
}
