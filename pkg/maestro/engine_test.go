package maestro

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

// paramsOf decodes a request's params into a map for comparison.
func paramsOf(t *testing.T, req Request) map[string]interface{} {
	t.Helper()
	raw, _ := json.Marshal(req.Params)
	m := map[string]interface{}{}
	if string(raw) != "null" {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("params of %s: %v", req.Method, err)
		}
	}
	return m
}

// The flow runner's calls send exactly what they sent before the opt-in
// params existed, so an agent takes its old code path for every one of them.
func TestExistingCallsSendNoOptInParams(t *testing.T) {
	got := map[string]map[string]interface{}{}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		got[req.Method] = paramsOf(t, req)
		switch req.Method {
		case "UI.snapshot":
			return SourceResult{XML: "<hierarchy/>"}
		case "UI.waitForSettle":
			return map[string]bool{"settled": true}
		case "UI.treeHash":
			return map[string]int64{"hash": 7}
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if _, err := adapter.Snapshot(0); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.WaitForSettle(2000, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.TreeHash(); err != nil {
		t.Fatal(err)
	}
	if err := adapter.LaunchApp("com.example", nil); err != nil {
		t.Fatal(err)
	}

	want := map[string]map[string]interface{}{
		"UI.snapshot":      {"waitForIdleMs": float64(0)},
		"UI.waitForSettle": {"timeout": float64(2000), "quiet": float64(300)},
		"UI.treeHash":      {},
		"Device.launchApp": {"appId": "com.example"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("params sent:\n got  %v\n want %v", got, want)
	}
}

func TestSnapshotWithSendsOnlyTheFlagsSet(t *testing.T) {
	var sent map[string]interface{}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		sent = paramsOf(t, req)
		return SourceResult{XML: `<hierarchy keyboard="false"/>`}
	})
	defer cleanup()

	xml, err := adapter.SnapshotWith(SnapshotOptions{WindowInfo: true})
	if err != nil {
		t.Fatal(err)
	}
	if xml != `<hierarchy keyboard="false"/>` {
		t.Errorf("xml = %q", xml)
	}
	want := map[string]interface{}{"waitForIdleMs": float64(0), "windowInfo": true}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("params = %v, want %v", sent, want)
	}
}

func TestGetForeground(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.getForeground" {
			t.Errorf("method = %s", req.Method)
		}
		return map[string]interface{}{"package": "com.android.settings", "windowId": 12,
			"keyguardLocked": false, "screenOn": true}
	})
	defer cleanup()

	fg, err := adapter.GetForeground()
	if err != nil {
		t.Fatal(err)
	}
	want := Foreground{Package: "com.android.settings", WindowID: 12, ScreenOn: true}
	if *fg != want {
		t.Errorf("got %+v, want %+v", *fg, want)
	}
}

func TestLaunchAppWait(t *testing.T) {
	var sent map[string]interface{}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		sent = paramsOf(t, req)
		return map[string]interface{}{"foreground": "com.example", "windowId": 3, "waitedMs": 812}
	})
	defer cleanup()

	res, err := adapter.LaunchAppWait("com.example", map[string]interface{}{"k": "v"}, 8*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Foreground != "com.example" || res.WaitedMs != 812 || res.WindowID != 3 {
		t.Errorf("result = %+v", res)
	}
	want := map[string]interface{}{"appId": "com.example", "waitForForegroundMs": float64(8000),
		"arguments": map[string]interface{}{"k": "v"}}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("params = %v, want %v", sent, want)
	}
}

func TestLaunchAppWaitErrors(t *testing.T) {
	cases := []struct {
		name string
		err  *ErrorPayload
		want *LaunchError
	}{
		{
			name: "not in front",
			err: &ErrorPayload{Code: "not_foreground", Message: "com.example is not in front",
				Data: json.RawMessage(`{"foreground":"com.android.systemui","keyguardLocked":false,"screenOn":true,"topWindowTitle":"Use USB for"}`)},
			want: &LaunchError{Code: "not_foreground", Message: "com.example is not in front",
				Foreground: "com.android.systemui", ScreenOn: true, TopWindowTitle: "Use USB for"},
		},
		{
			name: "locked",
			err: &ErrorPayload{Code: "screen_locked", Message: "the screen is locked",
				Data: json.RawMessage(`{"foreground":"com.android.systemui","keyguardLocked":true,"screenOn":true}`)},
			want: &LaunchError{Code: "screen_locked", Message: "the screen is locked", Foreground: "com.android.systemui", KeyguardLocked: true, ScreenOn: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter, cleanup := adapterWithMock(t, func(req Request) interface{} { return tc.err })
			defer cleanup()

			_, err := adapter.LaunchAppWait("com.example", nil, time.Second)
			var le *LaunchError
			if !errors.As(err, &le) {
				t.Fatalf("err = %v (%T), want *LaunchError", err, err)
			}
			if *le != *tc.want {
				t.Errorf("got %+v, want %+v", *le, *tc.want)
			}
		})
	}
}

func TestLaunchAppWaitOtherErrorPassesThrough(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		return &ErrorPayload{Code: "app_not_found", Message: "No launch intent"}
	})
	defer cleanup()

	_, err := adapter.LaunchAppWait("com.example", nil, time.Second)
	var ep *ErrorPayload
	if !errors.As(err, &ep) || ep.Code != "app_not_found" {
		t.Errorf("err = %v, want the app_not_found ErrorPayload", err)
	}
}

func TestWaitForSettleWith(t *testing.T) {
	var sent map[string]interface{}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		sent = paramsOf(t, req)
		return map[string]interface{}{"settled": true, "elapsedMs": 412, "reads": 5, "lastChangeMs": 112, "hash": 99}
	})
	defer cleanup()

	res, err := adapter.WaitForSettleWith(SettleOptions{TimeoutMs: 3000, QuietMs: 300, Scope: ScopeApp, IgnoreTextChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	want := SettleResult{Settled: true, ElapsedMs: 412, Reads: 5, LastChangeMs: 112, Hash: 99}
	if *res != want {
		t.Errorf("got %+v, want %+v", *res, want)
	}
	wantParams := map[string]interface{}{"timeout": float64(3000), "quiet": float64(300), "scope": "app", "ignoreTextChanges": true}
	if !reflect.DeepEqual(sent, wantParams) {
		t.Errorf("params = %v, want %v", sent, wantParams)
	}
}

func TestTreeHashWithDefaults(t *testing.T) {
	var sent map[string]interface{}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		sent = paramsOf(t, req)
		return map[string]int64{"hash": -5}
	})
	defer cleanup()

	h, err := adapter.TreeHashWith("", ModeLayout)
	if err != nil {
		t.Fatal(err)
	}
	if h != uint64(0xFFFFFFFFFFFFFFFB) {
		t.Errorf("hash = %d", h)
	}
	want := map[string]interface{}{"scope": "active", "mode": "layout"}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("params = %v, want %v", sent, want)
	}
}

func TestObserveJSON(t *testing.T) {
	var sent map[string]interface{}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		sent = paramsOf(t, req)
		return map[string]interface{}{
			"settle":        map[string]interface{}{"settled": true, "reads": 2},
			"xml":           "<hierarchy/>",
			"foreground":    map[string]interface{}{"package": "com.example", "screenOn": true},
			"keyboard":      map[string]interface{}{"visible": true, "top": 1650},
			"hasScreenshot": false,
		}
	})
	defer cleanup()

	res, img, err := adapter.Observe(ObserveOptions{Settle: &SettleOptions{TimeoutMs: 3000, QuietMs: 300, Scope: ScopeApp}})
	if err != nil {
		t.Fatal(err)
	}
	if img != nil {
		t.Errorf("image = %v, want nil", img)
	}
	if res.XML != "<hierarchy/>" || res.Foreground.Package != "com.example" || !res.Keyboard.Visible ||
		res.Keyboard.Top != 1650 || res.Settle == nil || !res.Settle.Settled {
		t.Errorf("result = %+v", res)
	}
	want := map[string]interface{}{"settle": map[string]interface{}{"timeoutMs": float64(3000), "quietMs": float64(300), "scope": "app"}}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("params = %v, want %v", sent, want)
	}
}

// observeFrame builds the agent's one-frame reply: id, JSON length, JSON, JPEG.
func observeFrame(id int64, resp Response, image []byte) []byte {
	js, _ := json.Marshal(resp)
	f := make([]byte, 12, 12+len(js)+len(image))
	binary.BigEndian.PutUint64(f[:8], uint64(id))
	binary.BigEndian.PutUint32(f[8:12], uint32(len(js)))
	f = append(f, js...)
	return append(f, image...)
}

func TestObserveWithScreenshot(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		p := paramsOf(t, req)
		if p["screenshot"] != true || p["screenshotMaxEdge"] != float64(800) {
			t.Errorf("params = %v", p)
		}
		raw, _ := json.Marshal(map[string]interface{}{"xml": "<hierarchy/>", "hasScreenshot": true})
		return binaryFrame(observeFrame(req.ID, Response{ID: req.ID, Result: raw}, jpeg))
	})
	defer cleanup()

	res, img, err := adapter.Observe(ObserveOptions{Screenshot: true, ScreenshotMaxEdge: 800})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasScreenshot || res.XML != "<hierarchy/>" {
		t.Errorf("result = %+v", res)
	}
	if string(img) != string(jpeg) {
		t.Errorf("image = %v, want %v", img, jpeg)
	}
}

func TestParseObserveBadFrames(t *testing.T) {
	for name, data := range map[string][]byte{
		"short":       {0, 0},
		"length over": {0, 0, 0, 50, '{', '}'},
	} {
		if _, _, err := parseObserve(&Response{BinaryData: data}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestSessionCapabilities(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		return map[string]interface{}{"sessionId": "s", "capabilities": []string{CapUIObserve, CapDeviceGetForeground}}
	})
	defer cleanup()

	s, err := adapter.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(CapUIObserve) || !s.Has(CapDeviceGetForeground) || s.Has(CapSnapshotWindowInfo) {
		t.Errorf("capabilities = %v", s.Capabilities)
	}

	old := SessionResult{}
	if old.Has(CapUIObserve) {
		t.Error("an agent that lists nothing has no capability")
	}
}
