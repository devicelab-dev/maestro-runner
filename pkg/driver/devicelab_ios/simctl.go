package devicelab_ios

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// extractRuntimeVersion finds the simulator with the given UDID in
// `simctl list devices -j` output and derives a short version string
// (e.g. "26.2") from its runtime identifier
// (com.apple.CoreSimulator.SimRuntime.iOS-26-2).
func extractRuntimeVersion(simctlJSON []byte, udid string) (string, error) {
	var parsed struct {
		Devices map[string][]struct {
			UDID string `json:"udid"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(simctlJSON, &parsed); err != nil {
		return "", fmt.Errorf("parse simctl list: %w", err)
	}
	for runtime, devices := range parsed.Devices {
		for _, dev := range devices {
			if dev.UDID == udid {
				return runtimeToShortVersion(runtime), nil
			}
		}
	}
	return "", fmt.Errorf("simulator %s not found in simctl list", udid)
}

var runtimePattern = regexp.MustCompile(`iOS-(\d+)-(\d+)`)

func runtimeToShortVersion(runtime string) string {
	if m := runtimePattern.FindStringSubmatch(runtime); len(m) == 3 {
		return m[1] + "." + m[2]
	}
	return "unknown"
}
