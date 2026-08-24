package cli

import (
	"strings"
	"testing"
)

func TestFormatDeviceTableGroupsByPlatform(t *testing.T) {
	entries := []DeviceEntry{
		{Platform: "android", Kind: "emulator", ID: "emulator-5554", State: "device", Ready: true},
		{Platform: "ios", Kind: "simulator", ID: "ABC-123", Name: "iPhone 16 Pro", OSVersion: "18.2", State: "Booted", Ready: true},
	}

	out := formatDeviceTable(entries, "")

	for _, want := range []string{"Android", "emulator-5554", "iOS", "iPhone 16 Pro", "Booted (iOS 18.2)"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if android, ios := strings.Index(out, "Android"), strings.Index(out, "iOS"); android > ios {
		t.Errorf("Android group should come before iOS:\n%s", out)
	}
}

func TestFormatDeviceTableMarksOnlyReadyEntries(t *testing.T) {
	entries := []DeviceEntry{
		{Platform: "android", Kind: "device", ID: "ready-serial", State: "device", Ready: true},
		{Platform: "android", Kind: "device", ID: "offline-serial", State: "offline"},
	}

	out := formatDeviceTable(entries, "")

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "ready-serial") && !strings.Contains(line, "●"):
			t.Errorf("a ready device should be marked: %q", line)
		case strings.Contains(line, "offline-serial") && strings.Contains(line, "●"):
			t.Errorf("an offline device must not be marked ready: %q", line)
		}
	}
}

func TestNoDevicesHintIsPlatformSpecific(t *testing.T) {
	androidOnly := noDevicesHint("android")
	if !strings.Contains(androidOnly, "adb devices") {
		t.Errorf("android hint should mention adb:\n%s", androidOnly)
	}
	if strings.Contains(androidOnly, "simulator") {
		t.Errorf("--platform android should not suggest simulators:\n%s", androidOnly)
	}
	if !strings.Contains(androidOnly, "doctor") {
		t.Errorf("the hint should point at doctor:\n%s", androidOnly)
	}
}

func TestDescribeStateOmitsVersionWhenUnknown(t *testing.T) {
	if got := describeState(DeviceEntry{Platform: "android", State: "device"}); got != "device" {
		t.Errorf("got %q, want bare state when no OS version is known", got)
	}
	if got := describeState(DeviceEntry{Platform: "ios", State: "Booted", OSVersion: "18.2"}); got != "Booted (iOS 18.2)" {
		t.Errorf("got %q, want the iOS label capitalised correctly", got)
	}
}

func TestFilterByPlatform(t *testing.T) {
	entries := []DeviceEntry{
		{Platform: "android", ID: "a"},
		{Platform: "ios", ID: "b"},
		{Platform: "android", ID: "c"},
	}
	got := filterByPlatform(entries, "android")
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("got %+v, want the two android entries in order", got)
	}
}
