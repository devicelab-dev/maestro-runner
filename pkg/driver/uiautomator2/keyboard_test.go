package uiautomator2

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestParseKeyboardFrame(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *core.Bounds
	}{
		{
			name: "Android <=12 mFrame visible",
			input: `  Window #1 Window{abcdef InputMethod}:
    mFrame=[0,1584][1080,2400]
    mShown=true`,
			want: &core.Bounds{X: 0, Y: 1584, Width: 1080, Height: 816},
		},
		{
			name:  "no frame info",
			input: "some random output with no frame info",
			want:  nil,
		},
		{
			name: "mFrame zero height",
			input: `  Window #1 Window{abcdef InputMethod}:
    mFrame=[0,2400][1080,2400]`,
			want: nil,
		},
		{
			name:  "mFrame zero width",
			input: `mFrame=[500,1584][500,2400]`,
			want:  nil,
		},
		{
			name:  "empty output",
			input: "",
			want:  nil,
		},
		{
			name:  "small keyboard (suggestions bar)",
			input: `mFrame=[0,2200][1080,2400]`,
			want:  &core.Bounds{X: 0, Y: 2200, Width: 1080, Height: 200},
		},
		{
			name: "Android 13+ touchable region format",
			input: `    mViewVisibility=0x0 mHaveFrame=true mObscured=false
    touchable region=SkRegion((0,1428,1080,2340))
    mHasSurface=true isReadyForDisplay()=true mWindowRemovalAllowed=false
    Frames: parent=[0,136][1080,2340] display=[0,136][1080,2340] frame=[0,136][1080,2340]
    mForceSeamlesslyRotate=false seamlesslyRotate: pending=null    isOnScreen=true`,
			want: &core.Bounds{X: 0, Y: 1428, Width: 1080, Height: 912},
		},
		{
			name: "Android 13+ with both mFrame and touchable region prefers touchable region",
			input: `  Window #2 Window{abcdef InputMethod}:
    mDisplayId=0 stackId=0 mSession=Session{...}
    mAttrs={(0,0)(fillxfill) ty=INPUT_METHOD fmt=TRANSLUCENT}
    mBaseLayer=131000 mSubLayer=0
    mFrame=[0,84][1080,2400]
    mViewVisibility=0x0 mHaveFrame=true mObscured=false
    touchable region=SkRegion((0,1428,1080,2340))
    mHasSurface=true isReadyForDisplay()=true
    Frames: parent=[0,84][1080,2400] display=[0,84][1080,2400] frame=[0,84][1080,2400]
    isOnScreen=true`,
			want: &core.Bounds{X: 0, Y: 1428, Width: 1080, Height: 912},
		},
		{
			name: "Android 13+ keyboard hidden (isOnScreen=false)",
			input: `    mViewVisibility=0x8 mHaveFrame=true
    touchable region=SkRegion((0,1538,1080,2340))
    mHasSurface=false isReadyForDisplay()=false
    Frames: parent=[0,136][1080,2340] frame=[0,136][1080,2340]
    isOnScreen=false`,
			want: nil,
		},
		{
			name: "keyboard hidden by mViewVisibility=0x8 alone (no isOnScreen field)",
			input: `    mViewVisibility=0x8 mHaveFrame=true
    touchable region=SkRegion((0,1538,1080,2340))
    mHasSurface=false`,
			want: nil,
		},
		{
			name: "SDK 30 touchable region without isOnScreen field",
			input: `  Window #1 Window{abcdef InputMethod}:
    mFrame=[0,84][1080,2400]
    mViewVisibility=0x0 mHaveFrame=true mObscured=false
    mGivenContentInsets=[0,1292][0,0]
    mTouchableInsets=3
    touchable region=SkRegion((0,1428,1080,2340))
    mHasSurface=true`,
			want: &core.Bounds{X: 0, Y: 1428, Width: 1080, Height: 912},
		},
		{
			name: "vendor keyboard — no touchable region, uses mFrame + content insets",
			input: `  Window #1 Window{abcdef InputMethod}:
    mFrame=[0,84][1080,2400]
    mViewVisibility=0x0 mHaveFrame=true mObscured=false
    mGivenContentInsets=[0,1292][0,0] mGivenVisibleInsets=[0,1292][0,0]
    mHasSurface=true isReadyForDisplay()=true
    isOnScreen=true`,
			want: &core.Bounds{X: 0, Y: 1376, Width: 1080, Height: 1024},
		},
		{
			name: "full-screen mFrame without insets or touchable region — rejected",
			input: `  Window #1 Window{abcdef InputMethod}:
    mFrame=[0,84][1080,2400]
    mViewVisibility=0x0 mHaveFrame=true
    mGivenContentInsets=[0,0][0,0]
    isOnScreen=true`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseKeyboardFrame(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil bounds, got nil")
			}
			if *got != *tt.want {
				t.Errorf("got %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

func TestGetKeyboardBounds(t *testing.T) {
	t.Run("no device returns nil", func(t *testing.T) {
		mock := &MockUIA2Client{}
		d := New(mock, nil, nil)
		if d.getKeyboardBounds() != nil {
			t.Error("expected nil when device is nil")
		}
	})

	t.Run("shell error returns nil", func(t *testing.T) {
		mock := &MockUIA2Client{}
		shell := &MockShellExecutor{err: errors.New("shell failed")}
		d := New(mock, nil, shell)
		if d.getKeyboardBounds() != nil {
			t.Error("expected nil on shell error")
		}
	})

	t.Run("mInputShown=false returns nil", func(t *testing.T) {
		mock := &MockUIA2Client{}
		shell := &MockShellExecutor{
			response: `mFrame=[0,1584][1080,2400]
    mInputShown=false`,
		}
		d := New(mock, nil, shell)
		if d.getKeyboardBounds() != nil {
			t.Error("expected nil when mInputShown=false")
		}
	})

	t.Run("keyboard visible with mFrame", func(t *testing.T) {
		mock := &MockUIA2Client{}
		shell := &MockShellExecutor{
			response: `mInputShown=true
mFrame=[0,1584][1080,2400]`,
		}
		d := New(mock, nil, shell)
		bounds := d.getKeyboardBounds()
		if bounds == nil {
			t.Fatal("expected non-nil bounds")
		}
		if bounds.Y != 1584 {
			t.Errorf("keyboard top = %d, want 1584", bounds.Y)
		}
	})

	t.Run("keyboard visible Android 13+", func(t *testing.T) {
		mock := &MockUIA2Client{}
		shell := &MockShellExecutor{
			response: `mInputShown=true
    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
		}
		d := New(mock, nil, shell)
		bounds := d.getKeyboardBounds()
		if bounds == nil {
			t.Fatal("expected non-nil bounds")
		}
		if bounds.Y != 1428 {
			t.Errorf("keyboard top = %d, want 1428", bounds.Y)
		}
	})
}

func TestIsKeyboardVisible(t *testing.T) {
	mock := &MockUIA2Client{}
	d := New(mock, nil, nil)
	if d.isKeyboardVisible() {
		t.Error("expected false when device is nil")
	}

	shell := &MockShellExecutor{response: `mInputShown=true
mFrame=[0,1584][1080,2400]`}
	d2 := New(mock, nil, shell)
	if !d2.isKeyboardVisible() {
		t.Error("expected true when keyboard frame is present")
	}
}

func TestConsumeInputFlag(t *testing.T) {
	t.Run("returns false when not set", func(t *testing.T) {
		mock := &MockUIA2Client{}
		d := New(mock, nil, nil)
		if d.consumeInputFlag() {
			t.Error("expected false")
		}
	})

	t.Run("returns true and resets when set", func(t *testing.T) {
		mock := &MockUIA2Client{}
		d := New(mock, nil, nil)
		d.lastStepWasInput = true

		if !d.consumeInputFlag() {
			t.Error("expected true")
		}
		if d.lastStepWasInput {
			t.Error("expected flag to be reset")
		}
	})
}

func TestExecuteFlagLifecycle(t *testing.T) {
	t.Run("inputText sets flag on success", func(t *testing.T) {
		server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
			"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": "active-elem"},
				})
			},
			"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]interface{}{"value": nil})
			},
		})
		defer server.Close()

		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

		step := &flow.InputTextStep{Text: "hello"}
		result := d.Execute(step)
		if !result.Success {
			t.Fatalf("inputText failed: %s", result.Message)
		}
		if !d.lastStepWasInput {
			t.Error("expected lastStepWasInput to be true after inputText")
		}
	})

	t.Run("other step clears flag", func(t *testing.T) {
		mock := &MockUIA2Client{}
		d := New(mock, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
		d.lastStepWasInput = true

		result := d.Execute(&flow.BackStep{})
		if !result.Success {
			t.Fatalf("back failed: %s", result.Message)
		}
		if d.lastStepWasInput {
			t.Error("expected lastStepWasInput to be false after non-input step")
		}
	})

	t.Run("inputRandom sets flag on success", func(t *testing.T) {
		server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
			"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": "active-elem"},
				})
			},
			"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]interface{}{"value": nil})
			},
		})
		defer server.Close()

		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

		step := &flow.InputRandomStep{DataType: "TEXT", Length: 5}
		result := d.Execute(step)
		if !result.Success {
			t.Fatalf("inputRandom failed: %s", result.Message)
		}
		if !d.lastStepWasInput {
			t.Error("expected lastStepWasInput to be true after inputRandom")
		}
	})

	t.Run("inputText failure does not set flag", func(t *testing.T) {
		mock := &MockUIA2Client{}
		d := New(mock, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

		step := &flow.InputTextStep{Text: ""}
		result := d.Execute(step)
		if result.Success {
			t.Fatal("expected failure for empty text")
		}
		if d.lastStepWasInput {
			t.Error("expected lastStepWasInput to be false after failed inputText")
		}
	})
}

func TestTapWouldHitKeyboard(t *testing.T) {
	// Keyboard: top at y=1428, height 912, bottom at 2340
	keyboard := core.Bounds{X: 0, Y: 1428, Width: 1080, Height: 912}

	tests := []struct {
		name    string
		element core.Bounds
		want    bool
	}{
		{
			name:    "element above keyboard — no hit",
			element: core.Bounds{X: 100, Y: 200, Width: 200, Height: 60},
			want:    false,
		},
		{
			name:    "element center below keyboard top — hit",
			element: core.Bounds{X: 100, Y: 1800, Width: 200, Height: 60},
			want:    true,
		},
		{
			name:    "element straddles keyboard top but center above — no hit",
			element: core.Bounds{X: 100, Y: 1380, Width: 200, Height: 60},
			want:    false,
		},
		{
			name:    "element straddles keyboard top but center below — hit",
			element: core.Bounds{X: 100, Y: 1410, Width: 200, Height: 60},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tapWouldHitKeyboard(tt.element, keyboard)
			if got != tt.want {
				_, cy := tt.element.Center()
				t.Errorf("tapWouldHitKeyboard = %v, want %v (element center Y=%d, keyboard top=%d)", got, tt.want, cy, keyboard.Y)
			}
		})
	}
}

// mockElementServer returns a mock HTTP server that finds an element at the given bounds.
func mockElementServer(t *testing.T, bounds core.Bounds) *httptest.Server {
	return setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-kb-test"},
			})
		},
		"GET /element/elem-kb-test/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Sign In"})
		},
		"GET /element/elem-kb-test/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": bounds.X, "y": bounds.Y, "width": bounds.Width, "height": bounds.Height},
			})
		},
		"POST /element/elem-kb-test/click": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
}

func TestTapOnKeyboardHintMessage(t *testing.T) {
	t.Run("keyboard covering element after inputText shows hint", func(t *testing.T) {
		// Element at y=1800 (center=1830), keyboard top at y=1428 → overlap
		server := mockElementServer(t, core.Bounds{X: 100, Y: 1800, Width: 200, Height: 60})
		defer server.Close()

		shell := &MockShellExecutor{
			response: `mInputShown=true
    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
		}
		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)
		d.lastStepWasInput = true

		step := &flow.TapOnStep{}
		step.Selector = flow.Selector{Text: "Sign In"}
		result := d.tapOn(step)

		if result.Success {
			t.Fatal("expected failure")
		}
		if !strings.Contains(result.Message, "keyboard") {
			t.Errorf("expected message to mention keyboard, got: %s", result.Message)
		}
		if !strings.Contains(result.Message, "hideKeyboard") {
			t.Errorf("expected message to mention hideKeyboard, got: %s", result.Message)
		}
	})

	t.Run("element above keyboard — tap proceeds normally", func(t *testing.T) {
		// Element at y=200 (center=230), keyboard top at y=1428 → NO overlap
		server := mockElementServer(t, core.Bounds{X: 100, Y: 200, Width: 200, Height: 60})
		defer server.Close()

		shell := &MockShellExecutor{
			response: `mInputShown=true
    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
		}
		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)
		d.lastStepWasInput = true

		step := &flow.TapOnStep{}
		step.Selector = flow.Selector{Text: "Sign In"}
		result := d.tapOn(step)

		if !result.Success {
			t.Fatalf("expected success (element above keyboard), got: %s", result.Message)
		}
	})

	t.Run("no keyboard — tap proceeds normally", func(t *testing.T) {
		server := mockElementServer(t, core.Bounds{X: 100, Y: 1800, Width: 200, Height: 60})
		defer server.Close()

		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
		d.lastStepWasInput = true

		step := &flow.TapOnStep{}
		step.Selector = flow.Selector{Text: "Sign In"}
		result := d.tapOn(step)

		if !result.Success {
			t.Fatalf("expected success (no keyboard), got: %s", result.Message)
		}
	})

	// This case used to assert the opposite — that a covered element taps
	// "successfully" as long as the previous step wasn't an input. That was #139:
	// the tap landed on the keyboard, the step reported success, and the next
	// inputText typed into whatever still held focus. A keyboard raised by an
	// autoFocus field is just as blocking as one raised by typing.
	t.Run("keyboard covering element without a preceding input step also fails", func(t *testing.T) {
		server := mockElementServer(t, core.Bounds{X: 100, Y: 1800, Width: 200, Height: 60})
		defer server.Close()

		shell := &MockShellExecutor{
			response: `mInputShown=true
    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
		}
		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)
		d.lastStepWasInput = false

		step := &flow.TapOnStep{}
		step.Selector = flow.Selector{Text: "Sign In"}
		result := d.tapOn(step)

		if result.Success {
			t.Fatal("expected failure: the element is behind the keyboard (#139)")
		}
		if !strings.Contains(result.Message, "hideKeyboard") {
			t.Errorf("expected the actionable hideKeyboard hint, got: %s", result.Message)
		}
	})
}

func TestAssertVisibleKeyboardBlocking(t *testing.T) {
	t.Run("keyboard covering element after inputText shows hint", func(t *testing.T) {
		server := mockElementServer(t, core.Bounds{X: 100, Y: 1800, Width: 200, Height: 60})
		defer server.Close()

		shell := &MockShellExecutor{
			response: `mInputShown=true
    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
		}
		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)
		d.lastStepWasInput = true

		step := &flow.AssertVisibleStep{}
		step.Selector = flow.Selector{Text: "Sign In"}
		result := d.assertVisible(step)

		if result.Success {
			t.Fatal("expected failure")
		}
		if !strings.Contains(result.Message, "keyboard") {
			t.Errorf("expected message to mention keyboard, got: %s", result.Message)
		}
	})

	t.Run("element above keyboard — assert passes", func(t *testing.T) {
		server := mockElementServer(t, core.Bounds{X: 100, Y: 200, Width: 200, Height: 60})
		defer server.Close()

		shell := &MockShellExecutor{
			response: `    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
		}
		client := newMockHTTPClient(server.URL)
		d := New(client.Client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)
		d.lastStepWasInput = true

		step := &flow.AssertVisibleStep{}
		step.Selector = flow.Selector{Text: "Sign In"}
		result := d.assertVisible(step)

		if !result.Success {
			t.Fatalf("expected success (element above keyboard), got: %s", result.Message)
		}
	})
}

// TestCheckKeyboardBlocking_RunsWhenKeyboardUpWithoutPriorInput is the #139
// regression, mirrored from the devicelab driver so the two can't drift. The
// keyboard can be up because a field auto-focused on screen entry, not because
// the previous step typed. The old `if !wasInput { return }` gate skipped the
// check entirely then, so the coordinate tap landed on the keyboard while the
// step still reported success.
func TestCheckKeyboardBlocking_RunsWhenKeyboardUpWithoutPriorInput(t *testing.T) {
	shell := &MockShellExecutor{
		response: `    touchable region=SkRegion((0,1428,1080,2340))
    isOnScreen=true`,
	}
	d := New(&MockUIA2Client{}, nil, shell)

	d.checkKeyboardBlocking(false, flow.Selector{ID: "rewrite_no_sp_input"})

	queried := false
	for _, cmd := range shell.commands {
		if strings.Contains(cmd, "InputMethod") {
			queried = true
		}
	}
	if !queried {
		t.Errorf("keyboard must be consulted even without a preceding input step (#139); shell saw %v", shell.commands)
	}
}

// TestCheckKeyboardBlocking_SkipsWhenKeyboardDown guards the cost side of the
// widened gate: with the keyboard down there is nothing to check, so the tap
// must not pay for an element lookup.
func TestCheckKeyboardBlocking_SkipsWhenKeyboardDown(t *testing.T) {
	shell := &MockShellExecutor{response: `mInputShown=false`}
	d := New(&MockUIA2Client{}, nil, shell)

	if res := d.checkKeyboardBlocking(false, flow.Selector{ID: "some-button"}); res != nil {
		t.Errorf("check must not apply while the keyboard is down, got %+v", res)
	}
}
