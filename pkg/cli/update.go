package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const updateCheckURL = "https://open.devicelab.dev/api/maestro-runner/updates"

// updateNotice receives the update message from the background check.
var updateNotice = make(chan string, 1)

type updateResponse struct {
	LatestVersion string `json:"latest_version"`
}

// startUpdateCheck kicks off a background update check.
// Call printUpdateNotice() later to print the result.
// MAESTRO_RUNNER_NO_UPDATE_CHECK skips it. On CI the check still runs but
// prints nothing: a CI job installs whatever version its script pins, and no
// one reads the notice there.
func startUpdateCheck() {
	if updateCheckOff(os.LookupEnv) {
		return
	}
	quiet := envOn(os.LookupEnv, "CI")
	exe, _ := os.Executable()
	ch := updateNotice
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}

		req, err := http.NewRequest("GET", updateCheckURL, nil)
		if err != nil {
			ch <- ""
			return
		}

		req.Header.Set("User-Agent", "maestro-runner")

		resp, err := client.Do(req)
		if err != nil {
			ch <- ""
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			ch <- ""
			return
		}

		var result updateResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ch <- ""
			return
		}

		if !quiet && isNewerRelease(result.LatestVersion, Version) {
			ch <- fmt.Sprintf("\n  Update available: %s → %s\n  Run: %s\n", Version, result.LatestVersion, upgradeCommand(exe))
		} else {
			ch <- ""
		}
	}()
}

// printUpdateNotice prints the update message if one is available.
func printUpdateNotice() {
	select {
	case msg := <-updateNotice:
		if msg != "" {
			fmt.Print(msg)
		}
	default:
		// Check not finished yet, don't block
	}
}

// updateCheckOff reports whether MAESTRO_RUNNER_NO_UPDATE_CHECK turns the
// update check off.
func updateCheckOff(lookup func(string) (string, bool)) bool {
	return envOn(lookup, "MAESTRO_RUNNER_NO_UPDATE_CHECK")
}

// envOn reports whether name is set to anything but "0" or "false", as
// telemetry reads its own opt-outs.
func envOn(lookup func(string) (string, bool), name string) bool {
	v, ok := lookup(name)
	v = strings.ToLower(strings.TrimSpace(v))
	return ok && v != "0" && v != "false"
}

// npmPlatformDir is part of the path every npm install of the binary has:
// npm/build-npm.sh ships it in @devicelab/maestro-runner-<os>-<arch>, under
// node_modules wherever npm, npx or a project install puts it.
const npmPlatformDir = "/node_modules/@devicelab/maestro-runner-"

// upgradeCommand is the command that updates the install the binary at exe
// came from: npm's for an npm install, the install script's otherwise.
func upgradeCommand(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if strings.Contains(filepath.ToSlash(exe), npmPlatformDir) {
		return "npm i -g maestro-runner@latest"
	}
	return "curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash"
}

var (
	releaseVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-.*)?$`)
	// describeSuffix is what `git describe --tags` adds to a build made
	// commits after a tag: -22-gf72df97.
	describeSuffix = regexp.MustCompile(`^-\d+-g[0-9a-f]+$`)
)

// releaseOrder places a version among releases: major, minor, patch, then -1
// for a pre-release (1.1.30-rc.1 comes before 1.1.30), 0 for the release
// itself, 1 for a `git describe` build of commits after it. A -dirty suffix
// is ignored. ok is false for anything else ("dev", a bare commit hash).
func releaseOrder(v string) (order [4]int, ok bool) {
	m := releaseVersion.FindStringSubmatch(strings.TrimSuffix(strings.TrimSpace(v), "-dirty"))
	if m == nil {
		return order, false
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return order, false
		}
		order[i] = n
	}
	switch {
	case m[4] == "":
	case describeSuffix.MatchString(m[4]):
		order[3] = 1
	default:
		order[3] = -1
	}
	return order, true
}

// isNewerRelease reports whether latest is strictly newer than current, the
// only case worth an update notice. A build ahead of the latest release, or
// a version either side can't read, gets none: that would advise going back.
func isNewerRelease(latest, current string) bool {
	l, ok := releaseOrder(latest)
	if !ok {
		return false
	}
	c, ok := releaseOrder(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
