package device

import (
	"testing"
)

func TestParseDeviceList_Empty(t *testing.T) {
	output := "List of devices attached\n"
	devices := parseDeviceList(output)

	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestParseDeviceList_SingleDevice(t *testing.T) {
	output := `List of devices attached
RF8M33XXXXX	device
`
	devices := parseDeviceList(output)

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	if devices[0].Serial != "RF8M33XXXXX" {
		t.Errorf("expected serial RF8M33XXXXX, got %s", devices[0].Serial)
	}
	if devices[0].State != "device" {
		t.Errorf("expected state device, got %s", devices[0].State)
	}
	if devices[0].Type != "device" {
		t.Errorf("expected type device, got %s", devices[0].Type)
	}
}

func TestParseDeviceList_Emulator(t *testing.T) {
	output := `List of devices attached
emulator-5554	device
`
	devices := parseDeviceList(output)

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	if devices[0].Serial != "emulator-5554" {
		t.Errorf("expected serial emulator-5554, got %s", devices[0].Serial)
	}
	if devices[0].Type != "emulator" {
		t.Errorf("expected type emulator, got %s", devices[0].Type)
	}
}

func TestParseDeviceList_MultipleDevices(t *testing.T) {
	output := `List of devices attached
emulator-5554	device
RF8M33XXXXX	device
192.168.1.100:5555	device
`
	devices := parseDeviceList(output)

	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	// Check each device
	expected := []struct {
		serial string
		typ    string
	}{
		{"emulator-5554", "emulator"},
		{"RF8M33XXXXX", "device"},
		{"192.168.1.100:5555", "device"},
	}

	for i, e := range expected {
		if devices[i].Serial != e.serial {
			t.Errorf("device %d: expected serial %s, got %s", i, e.serial, devices[i].Serial)
		}
		if devices[i].Type != e.typ {
			t.Errorf("device %d: expected type %s, got %s", i, e.typ, devices[i].Type)
		}
	}
}

func TestParseDeviceList_OfflineDevice(t *testing.T) {
	output := `List of devices attached
emulator-5554	offline
RF8M33XXXXX	unauthorized
`
	devices := parseDeviceList(output)

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	if devices[0].State != "offline" {
		t.Errorf("expected state offline, got %s", devices[0].State)
	}
	if devices[1].State != "unauthorized" {
		t.Errorf("expected state unauthorized, got %s", devices[1].State)
	}
}

func TestParseDeviceList_ExtraWhitespace(t *testing.T) {
	output := `List of devices attached

emulator-5554	device

`
	devices := parseDeviceList(output)

	if len(devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(devices))
	}
}

func TestDefaultUIAutomator2Config(t *testing.T) {
	cfg := DefaultUIAutomator2Config()

	if cfg.SocketPath != "" {
		t.Errorf("expected empty SocketPath, got %s", cfg.SocketPath)
	}
	if cfg.LocalPort != 0 {
		t.Errorf("expected LocalPort 0, got %d", cfg.LocalPort)
	}
	if cfg.DevicePort != 6790 {
		t.Errorf("expected DevicePort 6790, got %d", cfg.DevicePort)
	}
	if cfg.Timeout.Seconds() != 30 {
		t.Errorf("expected Timeout 30s, got %v", cfg.Timeout)
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := findFreePort(6001, 7001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port < 6001 || port > 7001 {
		t.Errorf("expected port in range 6001-7001, got %d", port)
	}
}

func TestFindFreePort_InvalidRange(t *testing.T) {
	// Use a range that's likely all taken (system ports)
	_, err := findFreePort(1, 1)
	if err == nil {
		t.Error("expected error for invalid range")
	}
}

func TestErrNoDevices(t *testing.T) {
	err := ErrNoDevices
	if err.Error() != "no Android devices connected" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
