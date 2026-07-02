package devicelab

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// mockDeviceLabClient is a minimal mock for scrollUntilVisible tests.
type mockDeviceLabClient struct {
	sourceFunc     func() (string, error)
	scrollCalls    int
	scrollErr      error
	findClickCalls int
}

func (m *mockDeviceLabClient) FindElement(strategy, selector string) (*uiautomator2.Element, error) {
	return nil, fmt.Errorf("element not found")
}
func (m *mockDeviceLabClient) FindAndClick(strategy, selector string) (*uiautomator2.Element, error) {
	m.findClickCalls++
	return nil, nil
}
func (m *mockDeviceLabClient) ActiveElement() (*uiautomator2.Element, error) { return nil, nil }
func (m *mockDeviceLabClient) SetImplicitWait(timeout time.Duration) error   { return nil }
func (m *mockDeviceLabClient) Click(x, y int) error                          { return nil }
func (m *mockDeviceLabClient) DoubleClick(x, y int) error                    { return nil }
func (m *mockDeviceLabClient) DoubleClickElement(elementID string) error     { return nil }
func (m *mockDeviceLabClient) LongClick(x, y, durationMs int) error          { return nil }
func (m *mockDeviceLabClient) LongClickElement(elementID string, durationMs int) error {
	return nil
}
func (m *mockDeviceLabClient) ScrollInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	m.scrollCalls++
	return m.scrollErr
}
func (m *mockDeviceLabClient) SwipeInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	return nil
}
func (m *mockDeviceLabClient) Back() error                       { return nil }
func (m *mockDeviceLabClient) HideKeyboard() error               { return nil }
func (m *mockDeviceLabClient) PressKeyCode(keyCode int) error    { return nil }
func (m *mockDeviceLabClient) SendKeyActions(text string) error  { return nil }
func (m *mockDeviceLabClient) Screenshot() ([]byte, error)       { return nil, nil }
func (m *mockDeviceLabClient) Source() (string, error)           { return m.sourceFunc() }
func (m *mockDeviceLabClient) GetOrientation() (string, error)   { return "PORTRAIT", nil }
func (m *mockDeviceLabClient) SetOrientation(string) error       { return nil }
func (m *mockDeviceLabClient) GetClipboard() (string, error)     { return "", nil }
func (m *mockDeviceLabClient) SetClipboard(string) error         { return nil }
func (m *mockDeviceLabClient) GetDeviceInfo() (*uiautomator2.DeviceInfo, error) {
	return &uiautomator2.DeviceInfo{RealDisplaySize: "1080x2400"}, nil
}
func (m *mockDeviceLabClient) LaunchApp(string, map[string]interface{}) error { return nil }
func (m *mockDeviceLabClient) ForceStop(string) error                         { return nil }
func (m *mockDeviceLabClient) ClearAppData(string) error                      { return nil }
func (m *mockDeviceLabClient) GrantPermissions(string, []string) error        { return nil }
func (m *mockDeviceLabClient) SetAppiumSettings(map[string]interface{}) error { return nil }
func (m *mockDeviceLabClient) WaitForSettle(int, int) (bool, error)          { return true, nil }
func (m *mockDeviceLabClient) TreeHash() (uint64, error)                     { return 0, nil }
func (m *mockDeviceLabClient) FindFirstOf([]string) (*uiautomator2.Element, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *mockDeviceLabClient) WaitForWindowUpdate(string, int) (bool, error) { return false, nil }

// Compile-time check
var _ DeviceLabClient = (*mockDeviceLabClient)(nil)

func TestScrollUntilVisibleRespectsMaxScrolls(t *testing.T) {
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "NonExistent"},
		Direction:  "down",
		MaxScrolls: 3,
		BaseStep:   flow.BaseStep{TimeoutMs: 30000},
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	if client.scrollCalls != 3 {
		t.Errorf("Expected exactly 3 scrolls (maxScrolls=3), got %d", client.scrollCalls)
	}
}

func TestScrollUntilVisibleRespectsTimeout(t *testing.T) {
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 500}, // very short timeout
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	// With 500ms timeout, should get far fewer than default 20 scrolls
	if client.scrollCalls >= 20 {
		t.Errorf("Expected timeout to limit scrolls (got %d, default max is 20)", client.scrollCalls)
	}
}

func TestScrollUntilVisibleDefaultMaxScrolls(t *testing.T) {
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 60000}, // long timeout
		// MaxScrolls not set — defaults to 20
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	if client.scrollCalls != 20 {
		t.Errorf("Expected default 20 scrolls, got %d", client.scrollCalls)
	}
}

// TestIsElementOnScreen covers the viewport-overlap guard used by
// scrollUntilVisible to reject hierarchy-only matches.
func TestIsElementOnScreen(t *testing.T) {
	tests := []struct {
		name   string
		bounds core.Bounds
		want   bool
	}{
		{"fully on screen", core.Bounds{X: 100, Y: 100, Width: 200, Height: 200}, true},
		{"partial overlap at bottom", core.Bounds{X: 100, Y: 2300, Width: 200, Height: 200}, true},
		{"flush against right edge", core.Bounds{X: 1079, Y: 100, Width: 200, Height: 200}, true},
		{"entirely below screen", core.Bounds{X: 100, Y: 2400, Width: 200, Height: 200}, false},
		{"entirely above screen", core.Bounds{X: 100, Y: -300, Width: 200, Height: 200}, false},
		{"entirely right of screen", core.Bounds{X: 1080, Y: 100, Width: 200, Height: 200}, false},
		{"zero width", core.Bounds{X: 100, Y: 100, Width: 0, Height: 200}, false},
		{"zero height", core.Bounds{X: 100, Y: 100, Width: 200, Height: 0}, false},
		{"negative width", core.Bounds{X: 100, Y: 100, Width: -50, Height: 200}, false},
		// Repro from the field: a clipped below-the-fold ScrollView button the
		// agent reports with top>bottom (raw [270,2300][1080,2274] → h=-26,
		// centre (675,2287)). Overlaps the viewport numerically, but is a
		// degenerate rect tapOn refuses (boundsTappable's #94 guard); the scroll
		// success predicate must reject it too so the loop keeps scrolling.
		{"negative height (clipped below-fold)", core.Bounds{X: 270, Y: 2300, Width: 810, Height: -26}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &core.ElementInfo{Bounds: tt.bounds}
			if got := isElementOnScreen(info, 1080, 2400); got != tt.want {
				t.Errorf("isElementOnScreen(%v) = %v, want %v", tt.bounds, got, tt.want)
			}
		})
	}
}

// TestIsElementNotFoundError covers the allowlist of "expected during scroll"
// error messages — anything else should bail out the scroll loop immediately.
func TestIsElementNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"context deadline", context.DeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("element 'x' not found: %w", context.DeadlineExceeded), true},
		{"element not found", errors.New("element not found"), true},
		{"no elements match", errors.New("no elements match selector"), true},
		{"no such element", errors.New("no such element"), true},
		{"could not be located", errors.New("an element could not be located on the page"), true},

		{"connection refused", errors.New("dial tcp: connection refused"), false},
		{"agent dead", errors.New("agent session closed unexpectedly"), false},
		{"http 500", errors.New("server returned 500 internal server error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isElementNotFoundError(tt.err); got != tt.want {
				t.Errorf("isElementNotFoundError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestScrollUntilVisibleSkipsOffScreenMatches verifies the regression in
// issue #81: when the agent returns a match from the off-screen portion of
// the view hierarchy, scrollUntilVisible must keep scrolling rather than
// short-circuit.
func TestScrollUntilVisibleSkipsOffScreenMatches(t *testing.T) {
	// Source XML reports an element below the visible screen height.
	// On every poll the same off-screen match is returned, so the loop
	// must exhaust all maxScrolls iterations.
	source := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.view.ViewGroup content-desc="off-screen-target" resource-id="off-screen-target" bounds="[100,3000][800,3400]" displayed="false"/>
  </android.widget.FrameLayout>
</hierarchy>`
	client := &mockDeviceLabClient{sourceFunc: func() (string, error) { return source, nil }}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{ID: "off-screen-target"},
		Direction:  "down",
		MaxScrolls: 4,
		BaseStep:   flow.BaseStep{TimeoutMs: 30000},
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when only hierarchy-only off-screen match exists, got success")
	}
	if client.scrollCalls != 4 {
		t.Errorf("Expected full %d scroll attempts (no short-circuit on off-screen match), got %d", 4, client.scrollCalls)
	}
}

