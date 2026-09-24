// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package maestro

import (
	"encoding/json"
	"sync"
)

// EventHandler is a callback for push events from the device driver.
type EventHandler func(params json.RawMessage)

// OnEvent registers a handler for the named event.
// Only one handler per event name is supported; a second call replaces the previous.
func (c *Client) OnEvent(event string, handler EventHandler) {
	c.events.Store(event, handler)
}

// RemoveEvent removes the handler for the named event.
func (c *Client) RemoveEvent(event string) {
	c.events.Delete(event)
}

// KeyboardTracker tracks keyboard state from push events.
// Attach it to a Client to get instant keyboard state without polling.
type KeyboardTracker struct {
	mu    sync.RWMutex
	info  KeyboardInfo
	ready bool // true after first event received
}

// NewKeyboardTracker creates a tracker and wires it to the client's
// Input.keyboardStateChanged event.
func NewKeyboardTracker(c *Client) *KeyboardTracker {
	kt := &KeyboardTracker{}
	c.OnEvent("Input.keyboardStateChanged", func(params json.RawMessage) {
		var info KeyboardInfo
		if err := json.Unmarshal(params, &info); err != nil {
			return
		}
		kt.mu.Lock()
		kt.info = info
		kt.ready = true
		kt.mu.Unlock()
	})
	return kt
}

// GetKeyboardInfo returns the latest keyboard state.
// Returns nil if no event has been received yet.
func (kt *KeyboardTracker) GetKeyboardInfo() *KeyboardInfo {
	kt.mu.RLock()
	defer kt.mu.RUnlock()
	if !kt.ready {
		return nil
	}
	info := kt.info
	return &info
}

// CDPState is the payload pushed by the Java driver's CDP socket monitor.
type CDPState struct {
	Available bool   `json:"available"`
	Socket    string `json:"socket,omitempty"`
}

// CDPTracker tracks Chrome DevTools Protocol socket availability from push events.
// The Java driver polls /proc/net/unix every 100ms (virtually free) and pushes on change.
type CDPTracker struct {
	mu    sync.RWMutex
	state CDPState
	ready bool // true after first event received
}

// NewCDPTracker creates a tracker and wires it to the client's
// UI.cdpStateChanged event.
func NewCDPTracker(c *Client) *CDPTracker {
	ct := &CDPTracker{}
	c.OnEvent("UI.cdpStateChanged", func(params json.RawMessage) {
		var state CDPState
		if err := json.Unmarshal(params, &state); err != nil {
			return
		}
		ct.mu.Lock()
		ct.state = state
		ct.ready = true
		ct.mu.Unlock()
	})
	return ct
}

// Latest returns the latest CDP socket state.
// Returns nil if no event has been received yet.
func (ct *CDPTracker) Latest() *CDPState {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if !ct.ready {
		return nil
	}
	state := ct.state
	return &state
}
