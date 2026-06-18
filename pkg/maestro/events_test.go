// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package maestro

import (
	"encoding/json"
	"testing"
	"time"
)

func TestKeyboardTracker(t *testing.T) {
	server, pushEvent := newMockWSServerWithPush(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	tracker := NewKeyboardTracker(client)

	// Before any event, should return nil
	if info := tracker.GetKeyboardInfo(); info != nil {
		t.Errorf("expected nil before first event, got %+v", info)
	}

	// Establish connection
	_, _ = client.Call("Session.status", nil)

	// Push keyboard visible event
	pushEvent(Event{
		Event:  "Input.keyboardStateChanged",
		Params: json.RawMessage(`{"visible":true,"bounds":{"x":0,"y":1200,"width":1080,"height":720}}`),
	})

	// Wait for event to be processed
	time.Sleep(200 * time.Millisecond)

	info := tracker.GetKeyboardInfo()
	if info == nil {
		t.Fatal("expected keyboard info, got nil")
	}
	if !info.Visible {
		t.Error("expected visible=true")
	}
	if info.Bounds == nil {
		t.Fatal("expected bounds, got nil")
	}
	if info.Bounds.Y != 1200 {
		t.Errorf("expected bounds.Y=1200, got %d", info.Bounds.Y)
	}

	// Push keyboard hidden event
	pushEvent(Event{
		Event:  "Input.keyboardStateChanged",
		Params: json.RawMessage(`{"visible":false}`),
	})

	time.Sleep(200 * time.Millisecond)

	info = tracker.GetKeyboardInfo()
	if info == nil {
		t.Fatal("expected keyboard info, got nil")
	}
	if info.Visible {
		t.Error("expected visible=false")
	}
}

func TestCDPTracker(t *testing.T) {
	server, pushEvent := newMockWSServerWithPush(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	tracker := NewCDPTracker(client)

	// Before any event, should return nil
	if state := tracker.Latest(); state != nil {
		t.Errorf("expected nil before first event, got %+v", state)
	}

	// Establish connection
	_, _ = client.Call("Session.status", nil)

	// Push CDP available event
	pushEvent(Event{
		Event:  "UI.cdpStateChanged",
		Params: json.RawMessage(`{"available":true,"socket":"webview_devtools_remote_12345"}`),
	})

	time.Sleep(200 * time.Millisecond)

	state := tracker.Latest()
	if state == nil {
		t.Fatal("expected CDP state, got nil")
	}
	if !state.Available {
		t.Error("expected available=true")
	}
	if state.Socket != "webview_devtools_remote_12345" {
		t.Errorf("expected socket=webview_devtools_remote_12345, got %s", state.Socket)
	}

	// Push CDP down event
	pushEvent(Event{
		Event:  "UI.cdpStateChanged",
		Params: json.RawMessage(`{"available":false}`),
	})

	time.Sleep(200 * time.Millisecond)

	state = tracker.Latest()
	if state == nil {
		t.Fatal("expected CDP state, got nil")
	}
	if state.Available {
		t.Error("expected available=false")
	}
}

func TestKeyboardTrackerConcurrency(t *testing.T) {
	server, pushEvent := newMockWSServerWithPush(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	tracker := NewKeyboardTracker(client)

	// Establish connection
	_, _ = client.Call("Session.status", nil)

	// Rapid-fire events — should not race
	for i := 0; i < 10; i++ {
		visible := i%2 == 0
		data, _ := json.Marshal(KeyboardInfo{Visible: visible})
		pushEvent(Event{
			Event:  "Input.keyboardStateChanged",
			Params: json.RawMessage(data),
		})
	}

	time.Sleep(300 * time.Millisecond)

	// Should have some info (exact state depends on ordering)
	info := tracker.GetKeyboardInfo()
	if info == nil {
		t.Fatal("expected keyboard info after events")
	}
}
