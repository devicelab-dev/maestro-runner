package appium

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// The uiautomator2 server writes AccessibilityNodeInfo.getError as `error` on
// every node. A Compose field whose only visible content is its validation
// error is found by that text; an empty `error` never matches anything.
func TestTextSelectorMatchesErrorAttribute(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <node class="android.widget.EditText" resource-id="com.example:id/email" text="" hint="Email" error="This is an error!" bounds="[0,0][1080,100]" enabled="true" displayed="true" />
  <node class="android.widget.TextView" text="Submit" error="" bounds="[0,100][1080,200]" enabled="true" displayed="true" />
</hierarchy>`
	elements, _, _ := ParsePageSource(xml)
	if len(elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elements))
	}
	if elements[0].ErrorText != "This is an error!" {
		t.Fatalf("error attribute not parsed: %q", elements[0].ErrorText)
	}
	got := FilterBySelector(elements, flow.Selector{Text: "This is an error!"}, "android")
	if len(got) != 1 || got[0].ErrorText != "This is an error!" {
		t.Errorf("literal error text should match the field, got %d", len(got))
	}
	got = FilterBySelector(elements, flow.Selector{Text: "This is an.*"}, "android")
	if len(got) != 1 {
		t.Errorf("regex error text should match the field, got %d", len(got))
	}
	// `.*` matches an empty string; an empty error attribute must not count.
	for _, e := range FilterBySelector(elements, flow.Selector{Text: ".*"}, "android") {
		if e.ErrorText == "" && e.Text == "" && e.ContentDesc == "" && e.HintText == "" {
			t.Errorf("an element with no text at all matched via its empty error attribute")
		}
	}
	if got := FilterBySelector(elements, flow.Selector{Text: "Submit"}, "android"); len(got) != 1 || got[0].Text != "Submit" {
		t.Errorf("ordinary text matching unchanged, got %d", len(got))
	}
}
