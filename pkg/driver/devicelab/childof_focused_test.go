package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A Compose text field: the test tag sits on a wrapper with no text, the typed
// text on a focused EditText inside it, and deeper empty views under that.
const composeTextField = `<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="" resource-id="loginUsername" class="android.view.View" bounds="[32,502][1048,628]" displayed="true" enabled="true">
<node index="0" text="probe.user" resource-id="" class="android.widget.EditText" bounds="[32,502][1048,628]" displayed="true" enabled="true" clickable="true" focusable="true" focused="true">
<node index="0" text="" resource-id="" class="android.view.View" bounds="[64,502][1016,628]" displayed="true" enabled="true">
<node index="0" text="Username" resource-id="input_text_label" class="android.widget.TextView" bounds="[64,515][932,557]" displayed="true" enabled="true"/>
<node index="1" text="" resource-id="" class="android.view.View" bounds="[64,562][372,615]" displayed="true" enabled="true"/>
</node>
</node>
</node>
</node>
</hierarchy>`

// `childOf: X, focused: true` looks only at the focused elements inside X, as
// Maestro does. The state filter used to be dropped when the selector had no
// text or id, so the deepest empty view inside X won and copyTextFrom copied "".
func TestChildOfFocusedPicksTheFocusedField(t *testing.T) {
	c := &splitClient{trackingClient: newTrackingClient(), snapshot: composeTextField, source: composeTextField}
	d := New(c, &core.PlatformInfo{}, &mockShell{})
	focused := true

	info, err := d.resolveRelativeSelector(flow.Selector{ChildOf: &flow.Selector{ID: "loginUsername"}, Focused: &focused})
	if err != nil || info == nil || info.Text != "probe.user" {
		t.Fatalf("childOf + focused = %+v, %v; want the focused EditText", info, err)
	}
}
