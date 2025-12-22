package device

import (
	"bytes"
	"os/exec"
	"strings"
)

// ConnectedDevice represents a device found via ADB.
type ConnectedDevice struct {
	Serial string
	State  string // "device", "offline", "unauthorized"
	Type   string // "emulator" or "device"
}

// ListDevices returns all connected Android devices.
func ListDevices() ([]ConnectedDevice, error) {
	adbPath, err := findADB()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(adbPath, "devices")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseDeviceList(stdout.String()), nil
}

// parseDeviceList parses output of "adb devices".
func parseDeviceList(output string) []ConnectedDevice {
	var devices []ConnectedDevice
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		d := ConnectedDevice{
			Serial: parts[0],
			State:  parts[1],
			Type:   "device",
		}

		// Emulators have serial like "emulator-5554"
		if strings.HasPrefix(d.Serial, "emulator-") {
			d.Type = "emulator"
		}

		devices = append(devices, d)
	}

	return devices
}

// FirstAvailable returns the first available device, or error if none.
func FirstAvailable() (*AndroidDevice, error) {
	devices, err := ListDevices()
	if err != nil {
		return nil, err
	}

	for _, d := range devices {
		if d.State == "device" {
			return New(d.Serial)
		}
	}

	return nil, ErrNoDevices
}

// ErrNoDevices is returned when no devices are connected.
var ErrNoDevices = &deviceError{"no Android devices connected"}

type deviceError struct {
	msg string
}

func (e *deviceError) Error() string {
	return e.msg
}
