package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A text selector finds a field by its validation error, as in Maestro 2.9+
// and on the uiautomator2 driver; an absent error never matches, even `.*`.
func TestTextSelectorMatchesErrorText(t *testing.T) {
	src := `<hierarchy rotation="0">
<node index="0" text="" resource-id="email" class="android.widget.EditText" error="Invalid email" bounds="[0,100][1080,200]" displayed="true" enabled="true"/>
<node index="1" text="Password" resource-id="password" class="android.widget.EditText" bounds="[0,300][1080,400]" displayed="true" enabled="true"/>
</hierarchy>`
	elems, err := ParsePageSource(src)
	if err != nil {
		t.Fatal(err)
	}
	var email *ParsedElement
	for _, e := range elems {
		if e.ResourceID == "email" {
			email = e
		}
	}
	if email == nil || email.ErrorText != "Invalid email" {
		t.Fatalf("error attribute not parsed: %+v", email)
	}
	if got := FilterBySelector(elems, flow.Selector{Text: "Invalid email"}); len(got) != 1 || got[0] != email {
		t.Errorf("Invalid email matched %d elements, want the email field", len(got))
	}
	if got := FilterBySelector(elems, flow.Selector{Text: "invalid.*"}); len(got) != 1 {
		t.Errorf("regex on the error matched %d, want 1", len(got))
	}
	if matchesErrorText(".*", "") {
		t.Error("an empty error matched .*")
	}
}

// The agent queries carry one errorMatches, in the last, ignore-case tier.
func TestAgentTextTiersAddErrorMatches(t *testing.T) {
	tiers := agentTextTiers("Invalid.email")
	for i, tier := range tiers {
		n := strings.Count(strings.Join(tier, " "), ".errorMatches(")
		if want := map[bool]int{true: 1, false: 0}[i == len(tiers)-1]; n != want {
			t.Errorf("tier %d %q: %d errorMatches, want %d", i, tier, n, want)
		}
	}
	if last := tiers[len(tiers)-1]; !strings.Contains(last[len(last)-1], "(?ims)") {
		t.Errorf("errorMatches not ignore-case: %q", last)
	}
}
