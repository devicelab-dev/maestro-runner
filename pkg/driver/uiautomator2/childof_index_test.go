package uiautomator2

import (
	"net/http"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Two pickup points with the same id, each with a two-line address (#262).
const twoPickupPoints = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="" resource-id="pickupPoint_POSTNL" class="android.view.ViewGroup" bounds="[0,400][1080,600]" displayed="true" enabled="true" clickable="true">
<node index="0" text="PostNL" resource-id="" class="android.widget.TextView" bounds="[40,420][400,470]" displayed="true" enabled="true"/>
<node index="1" text="Bijlmerplein 993&#10;1102MK AMSTERDAM" resource-id="" class="android.widget.TextView" bounds="[40,480][800,580]" displayed="true" enabled="true"/>
</node>
<node index="1" text="" resource-id="pickupPoint_POSTNL" class="android.view.ViewGroup" bounds="[0,620][1080,820]" displayed="true" enabled="true" clickable="true">
<node index="0" text="PostNL" resource-id="" class="android.widget.TextView" bounds="[40,640][400,690]" displayed="true" enabled="true"/>
<node index="1" text="Lemelerbergweg 63&#10;1101AW AMSTERDAM" resource-id="" class="android.widget.TextView" bounds="[40,700][800,800]" displayed="true" enabled="true"/>
</node>
</node>
</hierarchy>`

// copyTextFrom with childOf an indexed anchor copies from the anchor the index
// names, as in Maestro (#262, fixed by #241).
func TestCopyTextFromChildOfAnchorIndex(t *testing.T) {
	server := setupRelativeSelectorServer(t, twoPickupPoints, func(w http.ResponseWriter, r *http.Request) {})
	defer server.Close()
	driver := New(newMockHTTPClient(server.URL).Client, nil, nil)

	for index, want := range map[string]string{
		"":  "Bijlmerplein 993\n1102MK AMSTERDAM",
		"0": "Bijlmerplein 993\n1102MK AMSTERDAM",
		"1": "Lemelerbergweg 63\n1101AW AMSTERDAM",
	} {
		result := driver.Execute(&flow.CopyTextFromStep{Selector: flow.Selector{
			Text:    `.+\n.+`,
			ChildOf: &flow.Selector{ID: "pickupPoint_POSTNL", Index: index},
		}})
		if !result.Success || result.Data != want {
			t.Errorf("anchor index %q: copied %q (success %v, %v); want %q", index, result.Data, result.Success, result.Error, want)
		}
	}
}
