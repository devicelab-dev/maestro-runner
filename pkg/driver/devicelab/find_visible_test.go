package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A whole-screen check sees what the user sees: hidden and zero-area nodes do
// not count, and hint text, quotes and wrapped text match as in Maestro.
func TestFindVisibleOnce(t *testing.T) {
	screen := `<hierarchy>
<node class="android.widget.TextView" text="Albums" displayed="false" bounds="[0,0][100,50]"/>
<node class="android.widget.TextView" text="Ghost" displayed="true" bounds="[0,0][0,0]"/>
<node class="android.widget.EditText" hint-text="Email" displayed="true" bounds="[0,100][500,150]"/>
<node class="android.widget.TextView" text="Open &quot;Moby Dick&quot;" displayed="true" bounds="[0,200][500,250]"/>
<node class="android.widget.TextView" text="Two&#10;lines" displayed="true" bounds="[0,300][500,350]"/>
<node class="android.widget.Button" resource-id="com.app:id/save" displayed="true" bounds="[0,400][500,450]"/>
</hierarchy>`
	for _, tc := range []struct {
		sel   flow.Selector
		found bool
	}{
		{flow.Selector{Text: "Albums"}, false},
		{flow.Selector{Text: "Ghost"}, false},
		{flow.Selector{Text: "Email"}, true},
		{flow.Selector{Text: `Open "Moby Dick"`}, true},
		{flow.Selector{Text: "Two lines"}, true},
		{flow.Selector{ID: "save"}, true},
		{flow.Selector{Text: "Nowhere"}, false},
	} {
		client := &mockDeviceLabClient{sourceFunc: func() (string, error) { return screen, nil }}
		d := New(client, &core.PlatformInfo{}, &mockShell{})
		info, err := d.findVisibleOnce(tc.sel)
		if found := err == nil && info != nil; found != tc.found {
			t.Errorf("%s: found = %v, want %v (%v)", tc.sel.Describe(), found, tc.found, err)
		}
	}
}

func TestChecksBySnapshot(t *testing.T) {
	if !checksBySnapshot(flow.Selector{Text: "a"}) || !checksBySnapshot(flow.Selector{ID: "a"}) {
		t.Error("text and id selectors should use the snapshot")
	}
	if checksBySnapshot(flow.Selector{Text: "a", Below: &flow.Selector{Text: "b"}}) {
		t.Error("relative selectors keep their own path")
	}
	if checksBySnapshot(flow.Selector{Text: "a", Index: "2"}) {
		t.Error("index selectors keep their own path")
	}
	// The snapshot path ignores the index: one that is not a whole number
	// must take the index path, which reports it (it used to mean "first").
	if checksBySnapshot(flow.Selector{Text: "a", Index: "undefined"}) {
		t.Error("a non-numeric index must not take the snapshot path")
	}
}

// An anchored id matches an id with a package prefix, as in Maestro. The
// anchors are dropped from the device query, where they add nothing to a
// whole match (the query used to be a separate unanchored tier).
func TestAnchoredID(t *testing.T) {
	if !matchesID(`^auth\.login$`, "com.app:id/auth.login") {
		t.Error("page source: anchored id should match after the prefix")
	}
	strategies, err := buildSelectors(flow.Selector{ID: `^auth\.login$`}, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := strategies[len(strategies)-1].Value
	if want := `new UiSelector().resourceIdMatches("(?ims)(?:.*/)?(?:auth\.login)")`; last != want {
		t.Errorf("last strategy = %s, want %s", last, want)
	}
}

// An id matches regardless of case, as Maestro compiles ids with IGNORE_CASE:
// RNTester's flow asks for `id: Flatlist` and the item is "FlatList".
func TestIDIgnoresCase(t *testing.T) {
	if !matchesID("Flatlist", "FlatList") {
		t.Error("page source: id should match ignoring case")
	}
	strategies, err := buildSelectors(flow.Selector{ID: "Flatlist"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range strategies {
		if strings.Contains(s.Value, `resourceIdMatches("(?ims)(?:.*/)?(?:Flatlist)")`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no case-insensitive id strategy in %v", strategies)
	}
}
