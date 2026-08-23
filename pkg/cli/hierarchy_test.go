package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

const androidXML = `<?xml version="1.0"?>
<hierarchy rotation="0">
  <node class="android.widget.FrameLayout" resource-id="" text="" bounds="[0,0][1080,2400]">
    <node class="android.widget.Button" resource-id="com.app:id/login" text="Sign In" bounds="[100,200][300,260]"/>
  </node>
</hierarchy>`

const iosXML = `<?xml version="1.0"?>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" x="0" y="0" width="390" height="844">
  <XCUIElementTypeButton type="XCUIElementTypeButton" name="loginBtn" label="Sign In" x="50" y="100" width="290" height="50"/>
</XCUIElementTypeApplication>`

const flatJSON = `{"nodes":[
  {"index":0,"type":"Application","identifier":"","label":"","rect":{"x":0,"y":0,"width":390,"height":844},"parentIndex":null},
  {"index":1,"type":"Button","identifier":"loginBtn","label":"Sign In","rect":{"x":50,"y":100,"width":290,"height":50},"parentIndex":0}
]}`

const treeJSON = `{"type":"View","children":[{"type":"Button","id":"login","text":"Sign In"}]}`

func TestFormatHierarchy_Android(t *testing.T) {
	out, err := formatHierarchy([]byte(androidXML), false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "Button"`, `"id": "com.app:id/login"`, `"text": "Sign In"`, `"width": 200`, `"height": 60`} {
		if !strings.Contains(out, want) {
			t.Errorf("android JSON missing %q\n%s", want, out)
		}
	}
}

func TestFormatHierarchy_IOS(t *testing.T) {
	out, err := formatHierarchy([]byte(iosXML), false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "Button"`, `"id": "loginBtn"`, `"text": "Sign In"`, `"width": 290`} {
		if !strings.Contains(out, want) {
			t.Errorf("ios JSON missing %q\n%s", want, out)
		}
	}
}

func TestFormatHierarchy_FlatJSON(t *testing.T) {
	out, err := formatHierarchy([]byte(flatJSON), false, "")
	if err != nil {
		t.Fatal(err)
	}
	// The Button (parentIndex 0) must be nested under the Application root.
	if !strings.Contains(out, `"id": "loginBtn"`) || !strings.Contains(out, `"children"`) {
		t.Errorf("flat JSON not normalized into a tree:\n%s", out)
	}
}

func TestFormatHierarchy_Compact(t *testing.T) {
	out, err := formatHierarchy([]byte(treeJSON), true, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "{") {
		t.Errorf("compact should be flat, not JSON:\n%s", out)
	}
	if !strings.Contains(out, "Button") || !strings.Contains(out, "id=login") || !strings.Contains(out, `text="Sign In"`) {
		t.Errorf("compact missing expected fields:\n%s", out)
	}
	// The child Button must be indented under View.
	if !strings.Contains(out, "\n  Button") {
		t.Errorf("compact should indent children:\n%s", out)
	}
}

const androidStatesXML = `<?xml version="1.0"?>
<hierarchy>
  <node class="android.widget.Button" resource-id="submit" text="Submit" enabled="false" checkable="false" bounds="[0,0][100,50]"/>
  <node class="android.widget.CheckBox" resource-id="agree" text="Agree" enabled="true" checkable="true" checked="true" bounds="[0,60][100,110]"/>
  <node class="android.widget.EditText" resource-id="name" text="" enabled="true" focused="true" bounds="[0,120][100,170]"/>
</hierarchy>`

func TestFormatHierarchy_States(t *testing.T) {
	// JSON: disabled button, checked checkbox, focused field.
	out, err := formatHierarchy([]byte(androidStatesXML), false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"enabled": false`, `"checked": true`, `"focused": true`} {
		if !strings.Contains(out, want) {
			t.Errorf("states JSON missing %q\n%s", want, out)
		}
	}
	// An enabled, non-checkable element must not emit noise fields.
	if strings.Contains(out, `"checked": false`) && !strings.Contains(out, "agree") {
		t.Errorf("unexpected checked:false on non-checkable element")
	}

	// Compact: state flags appear as tags.
	comp, _ := formatHierarchy([]byte(androidStatesXML), true, "")
	for _, want := range []string{"[disabled]", "[checked]", "[focused]"} {
		if !strings.Contains(comp, want) {
			t.Errorf("compact missing state tag %q\n%s", want, comp)
		}
	}
}

func TestFormatHierarchy_Find(t *testing.T) {
	out, err := formatHierarchy([]byte(androidXML), false, "sign in")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Button") || !strings.Contains(out, "Sign In") {
		t.Errorf("find should surface the matching Button:\n%s", out)
	}
	// The non-matching FrameLayout should be filtered out.
	if strings.Contains(out, "FrameLayout") {
		t.Errorf("find should exclude non-matching elements:\n%s", out)
	}

	none, _ := formatHierarchy([]byte(androidXML), false, "nonexistent-xyz")
	if !strings.Contains(none, "no elements matching") {
		t.Errorf("find with no match should report it, got:\n%s", none)
	}
}

// fakeScreenshotDriver is a core.Driver stub for the screenshot side-channel of
// the hierarchy command; only Screenshot() is exercised.
type fakeScreenshotDriver struct {
	core.Driver
	data []byte
	err  error
}

func (f *fakeScreenshotDriver) Screenshot() ([]byte, error) { return f.data, f.err }

func TestCaptureHierarchyScreenshot(t *testing.T) {
	t.Run("writes the image and creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "shot.png")
		d := &fakeScreenshotDriver{data: []byte("\x89PNG fake")}

		if err := captureHierarchyScreenshot(d, path); err != nil {
			t.Fatalf("captureHierarchyScreenshot() error = %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read written screenshot: %v", err)
		}
		if string(got) != "\x89PNG fake" {
			t.Errorf("wrote %q, want the driver's bytes", got)
		}
	})

	t.Run("reports a driver failure", func(t *testing.T) {
		d := &fakeScreenshotDriver{err: errors.New("device gone")}
		if err := captureHierarchyScreenshot(d, filepath.Join(t.TempDir(), "s.png")); err == nil {
			t.Error("expected the driver error to surface")
		}
	})

	// An empty capture would otherwise write a 0-byte PNG that looks like a
	// successful screenshot until someone opens it.
	t.Run("rejects an empty capture", func(t *testing.T) {
		d := &fakeScreenshotDriver{data: nil}
		path := filepath.Join(t.TempDir(), "s.png")
		if err := captureHierarchyScreenshot(d, path); err == nil {
			t.Error("expected an error when the driver returned no image data")
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Error("no file should be written when the capture was empty")
		}
	})
}
