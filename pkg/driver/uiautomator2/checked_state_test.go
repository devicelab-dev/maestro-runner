package uiautomator2

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A Switch that is on but not focused has checked="true" selected="false", so
// comparing `checked:` against the selected state could not match it at all —
// and would match a different, unchecked element that happened to be selected.
// The fixture makes the two disagree in both directions on purpose.
func TestFilterBySelectorMatchesCheckedNotSelected(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.Switch resource-id="notifications" checkable="true" checked="true" selected="false" enabled="true" displayed="true" bounds="[0,0][100,50]" />
  <android.widget.Switch resource-id="marketing" checkable="true" checked="false" selected="true" enabled="true" displayed="true" bounds="[0,50][100,100]" />
</hierarchy>`

	elements, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}

	for _, tt := range []struct {
		name    string
		checked bool
		wantID  string
	}{
		{"checked matches the checked switch", true, "notifications"},
		{"unchecked matches the unchecked switch", false, "marketing"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checked := tt.checked
			got := FilterBySelector(elements, flow.Selector{Checked: &checked})
			if len(got) != 1 {
				t.Fatalf("got %d matches, want exactly 1", len(got))
			}
			if got[0].ResourceID != tt.wantID {
				t.Errorf("matched %q, want %q", got[0].ResourceID, tt.wantID)
			}
		})
	}
}
