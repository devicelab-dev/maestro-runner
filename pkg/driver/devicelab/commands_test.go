package devicelab

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// mockDeviceLabClient is a minimal mock for tests.
type mockDeviceLabClient struct {
	sourceFunc         func() (string, error)
	findAndClickFunc   func(strategy, selector string) (*uiautomator2.Element, error)
	activeElementFunc  func() (*uiautomator2.Element, error)
	sendKeyActionsFunc func(text string) error
	scrollCalls        int
	scrollErr          error
	findClickCalls     int
}

func (m *mockDeviceLabClient) FindElement(strategy, selector string) (*uiautomator2.Element, error) {
	return nil, fmt.Errorf("element not found")
}
func (m *mockDeviceLabClient) FindAndClick(strategy, selector string) (*uiautomator2.Element, error) {
	m.findClickCalls++
	if m.findAndClickFunc != nil {
		return m.findAndClickFunc(strategy, selector)
	}
	return nil, nil
}
func (m *mockDeviceLabClient) ActiveElement() (*uiautomator2.Element, error) {
	if m.activeElementFunc != nil {
		return m.activeElementFunc()
	}
	return nil, nil
}
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
func (m *mockDeviceLabClient) SendKeyActions(text string) error {
	if m.sendKeyActionsFunc != nil {
		return m.sendKeyActionsFunc(text)
	}
	return nil
}
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

// ============================================================================
// TapOn — ID selector uses atomic FindAndClick
// ============================================================================

func TestTapOnIDSelectorUsesAtomicFindAndClick(t *testing.T) {
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) { return "", nil },
		findAndClickFunc: func(strategy, selector string) (*uiautomator2.Element, error) {
			return uiautomator2.NewCachedElement("elem-1", "Hello", uiautomator2.ElementRect{
				X: 100, Y: 200, Width: 50, Height: 30,
			}), nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.TapOnStep{
		Selector: flow.Selector{ID: "my_button"},
	}

	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if client.findClickCalls != 1 {
		t.Errorf("Expected 1 FindAndClick call, got %d", client.findClickCalls)
	}
	if result.Message != "Tapped on element" {
		t.Errorf("Expected 'Tapped on element', got %q", result.Message)
	}
}

func TestTapOnTextSelectorUsesAtomicFindAndClick(t *testing.T) {
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) { return "", nil },
		findAndClickFunc: func(strategy, selector string) (*uiautomator2.Element, error) {
			return uiautomator2.NewCachedElement("elem-2", "Submit", uiautomator2.ElementRect{
				X: 50, Y: 100, Width: 200, Height: 40,
			}), nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Submit"},
	}

	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if client.findClickCalls != 1 {
		t.Errorf("Expected 1 FindAndClick call, got %d", client.findClickCalls)
	}
}

// ============================================================================
// InputText — SendKeys fallback to SendKeyActions
// ============================================================================

func TestInputTextSendKeysFallbackToSendKeyActions(t *testing.T) {
	sendKeyActionsCalled := false
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) { return "", nil },
		activeElementFunc: func() (*uiautomator2.Element, error) {
			elem := uiautomator2.NewCachedElement("active-1", "", uiautomator2.ElementRect{})
			elem.SetSendKeysFunc(func(text string) error {
				return errors.New("stale element reference")
			})
			return elem, nil
		},
		sendKeyActionsFunc: func(text string) error {
			sendKeyActionsCalled = true
			return nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.InputTextStep{
		Text: "hello",
	}

	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if !sendKeyActionsCalled {
		t.Error("Expected SendKeyActions fallback to be called")
	}
	if !strings.Contains(result.Message, "fallback") {
		t.Errorf("Expected fallback in message, got %q", result.Message)
	}
}

func TestInputTextSendKeysFallbackAlsoFails(t *testing.T) {
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) { return "", nil },
		activeElementFunc: func() (*uiautomator2.Element, error) {
			elem := uiautomator2.NewCachedElement("active-1", "", uiautomator2.ElementRect{})
			elem.SetSendKeysFunc(func(text string) error {
				return errors.New("stale element reference")
			})
			return elem, nil
		},
		sendKeyActionsFunc: func(text string) error {
			return errors.New("key actions failed")
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.InputTextStep{
		Text: "hello",
	}

	result := driver.inputText(step)

	if result.Success {
		t.Error("Expected failure when both SendKeys and SendKeyActions fail")
	}
	if !strings.Contains(result.Message, "SendKeyActions fallback also failed") {
		t.Errorf("Expected both errors in message, got %q", result.Message)
	}
}

// ============================================================================
// ScrollUntilVisible tests
// ============================================================================

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
