package devicelab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// mockDeviceLabClient is a minimal mock for scrollUntilVisible tests.
type mockDeviceLabClient struct {
	findClickGuardW, findClickGuardH int
	findClickHitTest                 bool
	findClickBlockedBy               string
	findClickSkip                    bool
	swipeCoordsCalls                 [][5]int
	swipeCoordsErr                   error
	sourceFunc                       func() (string, error)
	scrollCalls                      int
	scrollAreas                      []uiautomator2.RectModel
	scrollErr                        error
	findClickCalls                   int
}

func (m *mockDeviceLabClient) FindElement(strategy, selector string) (*uiautomator2.Element, error) {
	return nil, fmt.Errorf("element not found")
}
func (m *mockDeviceLabClient) FindAndClick(strategy, selector string) (*uiautomator2.Element, error) {
	m.findClickCalls++
	return nil, nil
}

// findClickGuardW/H record the screen size the driver passed, and
// findClickClicked is what the fake agent reports back (#162).
// FindFirstAndClickChecked reports an agent without the batched call, so the
// driver falls back to FindAndClickChecked per form, which these tests script.
func (m *mockDeviceLabClient) FindFirstAndClickChecked([]string, int, int, bool) (*uiautomator2.Element, bool, string, int, error) {
	return nil, false, "", -1, errors.New("unknown_method: Unknown method: Gesture.findFirstAndClick")
}

func (m *mockDeviceLabClient) FindAndClickChecked(strategy, selector string, screenW, screenH int, hitTest bool) (*uiautomator2.Element, bool, string, error) {
	elem, clicked, err := m.FindAndClickGuarded(strategy, selector, screenW, screenH)
	m.findClickHitTest = hitTest
	return elem, clicked, m.findClickBlockedBy, err
}
func (m *mockDeviceLabClient) FindAndClickGuarded(strategy, selector string, screenW, screenH int) (*uiautomator2.Element, bool, error) {
	m.findClickCalls++
	m.findClickGuardW, m.findClickGuardH = screenW, screenH
	elem, err := (*uiautomator2.Element)(nil), error(nil)
	if m.findClickSkip {
		return elem, false, err
	}
	return elem, true, err
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
	m.scrollAreas = append(m.scrollAreas, area)
	return m.scrollErr
}
func (m *mockDeviceLabClient) SwipeInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	return nil
}
func (m *mockDeviceLabClient) SwipeCoords(startX, startY, endX, endY, durationMs int) error {
	m.swipeCoordsCalls = append(m.swipeCoordsCalls, [5]int{startX, startY, endX, endY, durationMs})
	return m.swipeCoordsErr
}
func (m *mockDeviceLabClient) Back() error                                   { return nil }
func (m *mockDeviceLabClient) HideKeyboard() error                           { return nil }
func (m *mockDeviceLabClient) PressKeyCode(keyCode int) error                { return nil }
func (m *mockDeviceLabClient) SendKeyActions(text string) error              { return nil }
func (m *mockDeviceLabClient) AddMedia(name, mime string, data []byte) error { return nil }
func (m *mockDeviceLabClient) Screenshot() ([]byte, error)                   { return nil, nil }
func (m *mockDeviceLabClient) Source() (string, error) {
	if m.sourceFunc == nil {
		return "", errors.New("no source")
	}
	return m.sourceFunc()
}
func (m *mockDeviceLabClient) Snapshot(int) (string, error) {
	if m.sourceFunc == nil {
		return "", errors.New("no source")
	}
	return m.sourceFunc()
}
func (m *mockDeviceLabClient) GetOrientation() (string, error) { return "PORTRAIT", nil }
func (m *mockDeviceLabClient) SetOrientation(string) error     { return nil }
func (m *mockDeviceLabClient) GetClipboard() (string, error)   { return "", nil }
func (m *mockDeviceLabClient) SetClipboard(string) error       { return nil }
func (m *mockDeviceLabClient) GetDeviceInfo() (*uiautomator2.DeviceInfo, error) {
	return &uiautomator2.DeviceInfo{RealDisplaySize: "1080x2400"}, nil
}
func (m *mockDeviceLabClient) LaunchApp(string, map[string]interface{}) error { return nil }
func (m *mockDeviceLabClient) ForceStop(string) error                         { return nil }
func (m *mockDeviceLabClient) ClearAppData(string) error                      { return nil }
func (m *mockDeviceLabClient) GrantPermissions(string, []string) error        { return nil }
func (m *mockDeviceLabClient) SetAppiumSettings(map[string]interface{}) error { return nil }
func (m *mockDeviceLabClient) WaitForSettle(int, int) (bool, error)           { return true, nil }
func (m *mockDeviceLabClient) TreeHash() (uint64, error)                      { return 0, nil }
func (m *mockDeviceLabClient) FindFirstOf([]string) (*uiautomator2.Element, error) {
	return nil, fmt.Errorf("not implemented in mock")
}
func (m *mockDeviceLabClient) WaitForWindowUpdate(string, int) (bool, error) { return false, nil }

// Compile-time check
var _ DeviceLabClient = (*mockDeviceLabClient)(nil)

func TestScrollUntilVisibleRespectsMaxScrolls(t *testing.T) {
	captures := 0
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return movingList(&captures), nil
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
	captures := 0
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return movingList(&captures), nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		// No timeout of its own (the default 30s outlasts 20 scrolls here)
		// and MaxScrolls not set — defaults to 20
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	if client.scrollCalls != 20 {
		t.Errorf("Expected default 20 scrolls, got %d", client.scrollCalls)
	}
}

// growingList is a list that moves on every capture, with the target row
// partly below the screen's bottom edge until the shownAfter'th scroll, and
// fully on screen from then on. A partly visible match is found at once, so
// each round costs only the scroll's settle.
func growingList(scrolls, shownAfter int) string {
	top := 2350
	if scrolls >= shownAfter {
		top = 1000
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout class="android.widget.FrameLayout" displayed="true" enabled="true" bounds="[0,0][1080,2400]">
    <android.widget.TextView class="android.widget.TextView" text="Row %d" displayed="true" enabled="true" bounds="[100,100][500,200]"/>
    <android.widget.TextView class="android.widget.TextView" text="Target" displayed="true" enabled="true" bounds="[100,%d][500,%d]"/>
  </android.widget.FrameLayout>
</hierarchy>`, scrolls, top, top+100)
}

func TestScrollUntilVisibleWithTimeoutIsNotCutAtDefaultMaxScrolls(t *testing.T) {
	// The target comes on screen after the 21st scroll. A step with its own
	// timeout scrolls until that timeout, as Maestro does; the default of 20
	// scrolls only caps a step without one.
	client := &mockDeviceLabClient{}
	client.sourceFunc = func() (string, error) { return growingList(client.scrollCalls, 21), nil }
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 60000},
	})

	if !result.Success {
		t.Fatalf("expected the target after 21 scrolls, got %q", result.Message)
	}
	if got := client.scrollCalls; got != 21 {
		t.Errorf("expected 21 scrolls, got %d", got)
	}
}

// TestScrollStopCriterion covers scrollUntilVisible's stop check, which now
// requires the flow's visibility percentage (default: fully inside the
// viewport) rather than any 1px overlap — a match half-hidden at the screen
// edge must keep scrolling.
func TestScrollStopCriterion(t *testing.T) {
	tests := []struct {
		name       string
		bounds     core.Bounds
		percentage int // 0 = flow didn't set one → fully visible required
		want       bool
	}{
		{"fully on screen", core.Bounds{X: 100, Y: 100, Width: 200, Height: 200}, 0, true},
		// The old any-overlap check accepted these two — the #2411-class bug.
		{"partial overlap at bottom", core.Bounds{X: 100, Y: 2300, Width: 200, Height: 200}, 0, false},
		{"flush against right edge", core.Bounds{X: 1079, Y: 100, Width: 200, Height: 200}, 0, false},
		{"partial overlap accepted at 50%", core.Bounds{X: 100, Y: 2300, Width: 200, Height: 200}, 50, true},
		{"entirely below screen", core.Bounds{X: 100, Y: 2400, Width: 200, Height: 200}, 0, false},
		{"entirely above screen", core.Bounds{X: 100, Y: -300, Width: 200, Height: 200}, 0, false},
		{"entirely right of screen", core.Bounds{X: 1080, Y: 100, Width: 200, Height: 200}, 0, false},
		{"zero width", core.Bounds{X: 100, Y: 100, Width: 0, Height: 200}, 0, false},
		{"zero height", core.Bounds{X: 100, Y: 100, Width: 200, Height: 0}, 0, false},
		{"negative width", core.Bounds{X: 100, Y: 100, Width: -50, Height: 200}, 0, false},
		// Repro from the field: a clipped below-the-fold ScrollView button the
		// agent reports with top>bottom (raw [270,2300][1080,2274] → h=-26,
		// centre (675,2287)). Overlaps the viewport numerically, but is a
		// degenerate rect tapOn refuses (boundsTappable's #94 guard); the scroll
		// success predicate must reject it too so the loop keeps scrolling.
		{"negative height (clipped below-fold)", core.Bounds{X: 270, Y: 2300, Width: 810, Height: -26}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := core.MeetsVisibility(tt.bounds, 1080, 2400, tt.percentage); got != tt.want {
				t.Errorf("MeetsVisibility(%v, pct=%d) = %v, want %v", tt.bounds, tt.percentage, got, tt.want)
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
	// On every poll an off-screen match is returned, so the loop must
	// exhaust all maxScrolls iterations.
	// The match drifts a little on every capture so the list reads as still
	// moving; the no-progress stop is covered by its own test.
	captures := 0
	client := &mockDeviceLabClient{sourceFunc: func() (string, error) {
		captures++
		y := 3000 + captures
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.view.ViewGroup content-desc="off-screen-target" resource-id="off-screen-target" bounds="[100,%d][800,%d]" displayed="false"/>
  </android.widget.FrameLayout>
</hierarchy>`, y, y+400), nil
	}}
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

// movingList is a page source whose one row shifts up on every capture — a
// list that keeps advancing — so a test about maxScrolls or timeouts is not
// cut short by the no-progress stop.
func movingList(captures *int) string {
	*captures++
	y := 100 + *captures
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,%d][300,%d]"/>
  </android.widget.FrameLayout>
</hierarchy>`, y, y+50)
}

func TestScrollUntilVisibleStopsWhenScreenStopsMoving(t *testing.T) {
	// The same capture on every read: a list at its end. For a step without a
	// timeout of its own, two scrolls that change nothing are proof enough —
	// the loop must not spend the other 18.
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Last row" bounds="[100,2300][300,2350]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
		},
	}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
	})

	if result.Success {
		t.Fatal("expected failure when the element is not in the list")
	}
	if got := client.scrollCalls; got != 2 {
		t.Errorf("expected 2 scrolls before the no-progress stop, got %d", got)
	}
	if !strings.Contains(result.Message, "made no progress") {
		t.Errorf("message should name the reason, got %q", result.Message)
	}
}

// fillsLateSource is a page source that stays the same, a list still loading,
// until the shownAfter'th scroll, and then has the target row on screen.
func fillsLateSource(scrolls, shownAfter int) string {
	row := `<android.widget.ProgressBar class="android.widget.ProgressBar" displayed="true" enabled="true" bounds="[490,1150][590,1250]"/>`
	if scrolls >= shownAfter {
		row = `<android.widget.TextView class="android.widget.TextView" text="Target" displayed="true" enabled="true" bounds="[100,1000][500,1100]"/>`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout class="android.widget.FrameLayout" displayed="true" enabled="true" bounds="[0,0][1080,2400]">
    ` + row + `
  </android.widget.FrameLayout>
</hierarchy>`
}

func TestScrollUntilVisibleWithTimeoutWaitsForListThatFillsLate(t *testing.T) {
	// The screen does not change for four scrolls (the list is still
	// loading), then the target is there. A step with its own timeout looks
	// until that timeout, as Maestro does: a still screen is not proof that
	// the content has ended.
	client := &mockDeviceLabClient{}
	client.sourceFunc = func() (string, error) { return fillsLateSource(client.scrollCalls, 4), nil }
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 20000},
	})

	if !result.Success {
		t.Fatalf("expected the target once the list filled, got %q", result.Message)
	}
	if client.scrollCalls < 4 {
		t.Errorf("expected at least 4 scrolls before the target showed, got %d", client.scrollCalls)
	}
}

func TestScrollUntilVisibleWithTimeoutRunsToItOnStillScreen(t *testing.T) {
	// A screen that never changes and a step with its own timeout: the step
	// keeps scrolling until the timeout, then fails in Maestro's words.
	client := &mockDeviceLabClient{sourceFunc: func() (string, error) { return fillsLateSource(0, 1), nil }}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	start := time.Now()
	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 5000},
	})

	if result.Success {
		t.Fatal("expected failure when the element never shows")
	}
	if elapsed := time.Since(start); elapsed < 5*time.Second {
		t.Errorf("gave up after %v, before the 5s timeout", elapsed)
	}
	if client.scrollCalls <= 2 {
		t.Errorf("expected more scrolls than the no-progress stop's 2, got %d", client.scrollCalls)
	}
	if !strings.Contains(result.Message, "No visible element found") {
		t.Errorf("message should be Maestro's, got %q", result.Message)
	}
}

func TestScrollUntilVisibleWithoutTimeoutKeepsItsMessageAtTheDeadline(t *testing.T) {
	// A step without a timeout of its own that runs into the default 30s (a
	// list that keeps moving, a maxScrolls it never reaches) keeps the message
	// it had: Maestro's words are for a step that set its timeout.
	captures := 0
	client := &mockDeviceLabClient{sourceFunc: func() (string, error) { return movingList(&captures), nil }}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	start := time.Now()
	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "NonExistent"},
		Direction:  "down",
		MaxScrolls: 1000,
	})

	if result.Success {
		t.Fatal("expected failure when the element never shows")
	}
	if elapsed := time.Since(start); elapsed < 30*time.Second {
		t.Fatalf("ended after %v, before the default 30s timeout", elapsed)
	}
	if result.Message != "Element not found after 1000 scrolls" || result.Error == nil || result.Error.Error() != "element not found" {
		t.Errorf("a step without a timeout of its own should keep its message, got %q (%v)", result.Message, result.Error)
	}
}

func TestScrollUntilVisibleStopsWhenRunIsCancelled(t *testing.T) {
	// The run is cancelled (a runFlow's timeout, a caller's cancel) while a
	// step with its own timeout looks on a still screen: it stops at once
	// instead of swiping on until that timeout.
	client := &mockDeviceLabClient{sourceFunc: func() (string, error) { return fillsLateSource(0, 1), nil }}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	driver.SetContext(ctx)

	start := time.Now()
	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 5000},
	})

	if result.Success || !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("expected the cancel as the cause, got %q (%v)", result.Message, result.Error)
	}
	if got := client.scrollCalls; got != 0 {
		t.Errorf("expected no scroll after the cancel, got %d", got)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("stopped after %v; want at once", elapsed)
	}
}
