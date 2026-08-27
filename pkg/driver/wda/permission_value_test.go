package wda

import "testing"

// iOS distinguishes "while using the app" from "always", so location takes a
// vocabulary the other permissions do not. A flow written against Maestro uses
// those same words and must behave identically here.
func TestIOSPermissionActionLocation(t *testing.T) {
	tests := []struct {
		value   string
		action  string
		service string
	}{
		{"always", "grant", "location-always"},
		{"allow", "grant", "location-always"},
		{"inuse", "grant", "location"},
		{"whenInUse", "grant", "location"},
		{"never", "revoke", "location-always"},
		{"deny", "revoke", "location-always"},
		{"unset", "reset", "location-always"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			action, service, ok := iosPermissionAction("location-always", tt.value)
			if !ok {
				t.Fatalf("%q should be a valid location value", tt.value)
			}
			if action != tt.action || service != tt.service {
				t.Errorf("got %s %s, want %s %s", action, service, tt.action, tt.service)
			}
		})
	}
}

func TestIOSPermissionActionOrdinaryPermissions(t *testing.T) {
	for value, want := range map[string]string{"allow": "grant", "deny": "revoke", "unset": "reset"} {
		action, service, ok := iosPermissionAction("camera", value)
		if !ok || action != want || service != "camera" {
			t.Errorf("camera %q → %s %s (ok=%v), want %s camera", value, action, service, ok, want)
		}
	}
}

// A value the permission does not accept must be reported, not skipped.
// Silently dropping `location: never` reset the permission to "not determined"
// and let the app prompt the user — the opposite of what the flow asked for.
func TestIOSPermissionActionRejectsUnknownValues(t *testing.T) {
	if _, _, ok := iosPermissionAction("camera", "never"); ok {
		t.Error("`never` is location-only and should be rejected for camera")
	}
	if _, _, ok := iosPermissionAction("location-always", "sometimes"); ok {
		t.Error("`sometimes` is not a location value")
	}
	if _, _, ok := iosPermissionAction("camera", ""); ok {
		t.Error("an empty value should be rejected")
	}
}

func TestIOSPermissionValueSupportedUsesTheShortcut(t *testing.T) {
	if !iosPermissionValueSupported("location", "never") {
		t.Error("location shortcut should accept never")
	}
	if iosPermissionValueSupported("camera", "never") {
		t.Error("camera should not accept never")
	}
	if !iosPermissionValueSupported("all", "allow") {
		t.Error("all should accept allow")
	}
	// An unrecognised name is passed through to simctl as a raw service name —
	// a deliberate escape hatch for services this list does not enumerate. It
	// is the value that gets validated here; simctl rejects a bad service with
	// an error of its own.
	if !iosPermissionValueSupported("photos-add", "allow") {
		t.Error("a raw simctl service name should still accept allow")
	}
	if iosPermissionValueSupported("photos-add", "never") {
		t.Error("a raw service name should still reject a location-only value")
	}
}
