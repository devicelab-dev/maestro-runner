// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package flutter

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// DiscoverVMServiceIOS finds the Flutter VM Service URL from iOS simulator logs.
// Returns the ws:// URL directly — no forwarding needed on simulators.
// Returns ("", nil) if not a Flutter app.
func DiscoverVMServiceIOS(udid string) (wsURL string, err error) {
	// Query the unified log for recent Flutter VM Service announcements
	cmd := exec.Command("xcrun", "simctl", "spawn", udid, "log", "show",
		"--last", "2m",
		"--predicate", `eventMessage CONTAINS "Dart VM service is listening"`,
		"--style", "compact",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("simctl log show: %w", err)
	}

	port, token := parseIOSVMServiceLog(string(out))
	if port == 0 {
		return "", nil // Not a Flutter app
	}

	wsURL = fmt.Sprintf("ws://127.0.0.1:%d/%s/ws", port, token)
	logger.Info("Flutter VM service discovered on iOS simulator: %s", wsURL)
	return wsURL, nil
}

// parseIOSVMServiceLog extracts the VM Service port and token from log output.
// Returns the most recent match (app may have restarted).
func parseIOSVMServiceLog(output string) (port int, token string) {
	var lastPort, lastToken string
	for _, line := range strings.Split(output, "\n") {
		m := reVMService.FindStringSubmatch(line)
		if m != nil {
			lastPort = m[1]
			lastToken = m[2]
		}
	}

	if lastPort == "" {
		return 0, ""
	}

	p := 0
	for _, ch := range lastPort {
		p = p*10 + int(ch-'0')
	}

	return p, lastToken
}
