package devicelab

import (
	"errors"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// splitClient serves different trees for the all-window snapshot and the
// active-window page source, as a device does while a sheet is up.
type splitClient struct {
	*trackingClient
	snapshot    string
	snapshotErr error
	source      string
}

func (s *splitClient) Snapshot(int) (string, error) { return s.snapshot, s.snapshotErr }
func (s *splitClient) Source() (string, error)      { return s.source, nil }

const activeWindowOnly = `<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="Home" resource-id="" class="android.widget.TextView" bounds="[0,100][1080,200]" displayed="true" enabled="true"/>
</node>
</hierarchy>`

const everyWindow = `<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,1200][1080,2000]" displayed="true" enabled="true">
<node index="0" text="Share" resource-id="" class="android.widget.TextView" bounds="[0,1300][1080,1400]" displayed="true" enabled="true"/>
<node index="1" text="Copy" resource-id="" class="android.widget.TextView" bounds="[0,1500][1080,1600]" displayed="true" enabled="true"/>
<node index="2" text="Copy" resource-id="" class="android.widget.TextView" bounds="[0,1700][1080,1800]" displayed="true" enabled="true"/>
</node>
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="Home" resource-id="" class="android.widget.TextView" bounds="[0,100][1080,200]" displayed="true" enabled="true"/>
</node>
</hierarchy>`

func newSplitDriver(snapshotErr error) *Driver {
	c := &splitClient{trackingClient: newTrackingClient(), snapshot: everyWindow, snapshotErr: snapshotErr, source: activeWindowOnly}
	return New(c, &core.PlatformInfo{}, &mockShell{})
}

// Relative, index and count lookups read every window: an element in a sheet
// (another window) is found the same way a plain text selector finds it.
func TestLookupsReadEveryWindow(t *testing.T) {
	d := newSplitDriver(nil)

	if n, err := d.countVisibleMatches(flow.Selector{Text: "Copy"}); err != nil || n != 2 {
		t.Errorf("countVisibleMatches = %d, %v; want 2 from the sheet window", n, err)
	}

	info, err := d.resolveRelativeSelector(flow.Selector{Text: "Copy", Below: &flow.Selector{Text: "Share"}})
	if err != nil || info == nil || info.Bounds.Y != 1500 {
		t.Errorf("below: Share = %+v, %v; want the Copy at y=1500", info, err)
	}

	if _, info, err := d.findElementByPageSourceOnce(flow.Selector{Text: "Copy", Index: "1"}); err != nil || info == nil || info.Bounds.Y != 1700 {
		t.Errorf("index 1 = %+v, %v; want the second Copy at y=1700", info, err)
	}
}

// When the agent cannot snapshot, lookups fall back to the page source.
func TestLookupsFallBackToSource(t *testing.T) {
	d := newSplitDriver(errors.New("snapshot unsupported"))

	if n, err := d.countVisibleMatches(flow.Selector{Text: "Home"}); err != nil || n != 1 {
		t.Errorf("countVisibleMatches = %d, %v; want 1 from the page source", n, err)
	}
	if n, _ := d.countVisibleMatches(flow.Selector{Text: "Copy"}); n != 0 {
		t.Errorf("countVisibleMatches(Copy) = %d; the page source has none", n)
	}
}
