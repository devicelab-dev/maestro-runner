// Package telemetry sends one anonymous usage event per maestro-runner run.
//
// The event holds counts, versions and fixed-list values only: no machine or
// user id, no flow or file names, selectors, app ids, URLs, environment
// values or error messages. Nothing is written on the user's machine.
// DO_NOT_TRACK or MAESTRO_RUNNER_NO_TELEMETRY turn it off; with
// MAESTRO_RUNNER_TELEMETRY_DEBUG the event is printed instead of sent. See
// docs: https://open.devicelab.dev/maestro-runner/docs/telemetry
//
// It is separate from the update check, which is unchanged.
package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// DocsURL is where the event's fields are listed.
const DocsURL = "https://open.devicelab.dev/maestro-runner/docs/telemetry"

// endpoint receives the event (a Cloudflare Worker; see the
// maestro-runner-telemetry project). MAESTRO_RUNNER_TELEMETRY_URL overrides
// it, for tests.
var endpoint = "https://open.devicelab.dev/api/maestro-runner/telemetry"

// sendTimeout bounds the whole send: telemetry never holds up a run.
const sendTimeout = 2 * time.Second

// Event is the one record sent per run. Its fields and allowed values match
// the Worker's (src/event.js there); the Worker rejects anything else.
type Event struct {
	RunnerVersion      string         `json:"runner_version"`
	Command            string         `json:"command"`
	Flags              []string       `json:"flags,omitempty"`
	ProjectID          string         `json:"project_id"`
	CI                 string         `json:"ci"`
	HostOS             string         `json:"host_os"`
	HostArch           string         `json:"host_arch"`
	Platform           string         `json:"platform,omitempty"`
	Driver             string         `json:"driver,omitempty"`
	DeviceKind         string         `json:"device_kind,omitempty"`
	OSMajor            string         `json:"os_major"`
	Devices            int            `json:"devices"`
	FlowsTotal         int            `json:"flows_total"`
	FlowsPassed        int            `json:"flows_passed"`
	FlowsFailed        int            `json:"flows_failed"`
	FlowsPassedOnRetry int            `json:"flows_passed_on_retry"`
	StepsTotal         int            `json:"steps_total"`
	FailureKinds       map[string]int `json:"failure_kinds"`
	RunOutcome         string         `json:"run_outcome"`
	DurationS          int            `json:"duration_s"`
}

// Enabled reports whether telemetry may be sent: not when DO_NOT_TRACK or
// MAESTRO_RUNNER_NO_TELEMETRY is set (to anything but "0" or "false").
func Enabled() bool {
	return !envSet("DO_NOT_TRACK") && !envSet("MAESTRO_RUNNER_NO_TELEMETRY")
}

func envSet(name string) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	v = strings.ToLower(strings.TrimSpace(v))
	return v != "0" && v != "false"
}

// Run is what the CLI knows about a run, beyond its report.
type Run struct {
	Version   string
	Command   string
	FlagNames []string // names only, as typed: never values
	Platform  string   // android, ios, web
	Driver    string
	Cloud     bool     // an Appium cloud provider ran it
	Devices   int      // devices used
	FlowPaths []string // top-level flow files, relative to the folder given (for the project id)
	Outcome   string   // completed, setup_failed, crashed, interrupted
	Duration  time.Duration
}

// Build assembles the event for a run from its report index (nil when the
// run produced no report, e.g. it crashed before running flows).
func Build(run Run, index *report.Index, reportDir string) Event {
	ev := Event{
		RunnerVersion: versionToken(run.Version),
		Command:       oneOf(run.Command, "test", "hierarchy"),
		Flags:         flagNames(run.FlagNames),
		ProjectID:     ProjectID(run.FlowPaths),
		CI:            DetectCI(),
		HostOS:        oneOf(runtime.GOOS, "darwin", "linux", "windows"),
		HostArch:      oneOf(runtime.GOARCH, "amd64", "arm64"),
		Platform:      oneOf(run.Platform, "android", "ios", "web"),
		Driver:        oneOf(run.Driver, "devicelab", "uiautomator2", "appium", "wda", "devicelab-legacy", "browser"),
		Devices:       run.Devices,
		FailureKinds:  map[string]int{},
		RunOutcome:    oneOf(run.Outcome, "completed", "setup_failed", "crashed", "interrupted"),
		DurationS:     int(run.Duration.Round(time.Second) / time.Second),
	}
	if ev.Platform == "web" {
		ev.DeviceKind = "browser"
	} else if run.Cloud {
		ev.DeviceKind = "cloud"
	}
	if index == nil {
		return ev
	}
	// Without --platform the platform is found from the device after the run
	// starts, so the run config has none: take it, and the driver, from the
	// report. Device kind and OS version depend on the platform.
	if ev.Platform == "" {
		p := index.Device.Platform
		for i := range index.Flows {
			if d := index.Flows[i].Device; p == "" && d != nil {
				p = d.Platform
			}
		}
		ev.Platform = oneOf(p, "android", "ios", "web")
	}
	if ev.Driver == "" {
		ev.Driver = oneOf(index.MaestroRunner.Driver, "devicelab", "uiautomator2", "appium", "wda", "devicelab-legacy", "browser")
	}
	// A multi-device run (local --parallel, several --device, Appium cloud
	// sessions) records the device of each flow: count those, and report a
	// kind or OS version only when they all agree ("mixed" otherwise).
	devices, kinds, versions := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range index.Flows {
		if d := index.Flows[i].Device; d != nil {
			id := d.ID
			if d.SessionID != "" {
				id = "session:" + d.SessionID
			}
			devices[id] = true
			kinds[deviceKind(ev.Platform, *d)] = true
			versions[osMajor(ev.Platform, d.OSVersion)] = true
		}
	}
	if len(devices) == 0 {
		kinds[deviceKind(ev.Platform, index.Device)] = true
		versions[osMajor(ev.Platform, index.Device.OSVersion)] = true
	}
	if len(devices) > ev.Devices {
		ev.Devices = len(devices)
	}
	if ev.DeviceKind == "" {
		ev.DeviceKind = only(kinds)
	}
	ev.OSMajor = only(versions)
	for i := range index.Flows {
		f := &index.Flows[i]
		ev.FlowsTotal++
		ev.StepsTotal += f.Commands.Total
		switch f.Status {
		case report.StatusPassed:
			ev.FlowsPassed++
			if f.Attempts > 1 {
				ev.FlowsPassedOnRetry++
			}
		case report.StatusFailed:
			ev.FlowsFailed++
			ev.FailureKinds[failureKind(reportDir, f)]++
		}
	}
	return ev
}

// Send posts the event, bounded by sendTimeout, ignoring every error. With
// MAESTRO_RUNNER_TELEMETRY_DEBUG set it prints the event instead.
func Send(ev Event) {
	SendWithin(ev, sendTimeout)
}

// SendWithin is Send with its own bound (an interrupted run waits less).
func SendWithin(ev Event, limit time.Duration) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if envSet("MAESTRO_RUNNER_TELEMETRY_DEBUG") {
		fmt.Fprintf(os.Stderr, "telemetry (debug, not sent): %s\n", body)
		return
	}
	url := endpoint
	if u := os.Getenv("MAESTRO_RUNNER_TELEMETRY_URL"); u != "" {
		url = u
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "maestro-runner")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	_ = resp.Body.Close()
}

// Notice is the line printed after a local run while telemetry is on. It is
// printed every time: nothing is saved to remember it was shown.
func Notice() string {
	return fmt.Sprintf("Anonymous usage stats are on (counts only; no names, paths or machine id): %s\n  Turn off: MAESTRO_RUNNER_NO_TELEMETRY=1 (or DO_NOT_TRACK=1)", DocsURL)
}

// ProjectID is a one-way hash of the flow files a run was pointed at: the
// top-level flow files found there (before tag filtering), as paths relative
// to the folder given with "/" separators, sorted and hashed together. It
// tells projects apart without naming them: the same suite gives the same id
// on every machine, OS and CI runner, through edits to its flows, with no
// git or environment lookups. It changes when a flow file is added, removed
// or renamed, and a subset run (one file, a smoke folder) has its own id.
// Empty without flows.
func ProjectID(flowPaths []string) string {
	paths := make([]string, 0, len(flowPaths))
	seen := map[string]bool{}
	for _, p := range flowPaths {
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	sort.Strings(paths)
	sum := sha256.Sum256([]byte("maestro-runner/flows\n" + strings.Join(paths, "\n")))
	return hex.EncodeToString(sum[:])
}

// DetectCI names the CI service from its environment, "none" when not on CI.
func DetectCI() string {
	known := []struct{ env, name string }{
		{"GITHUB_ACTIONS", "github-actions"},
		{"GITLAB_CI", "gitlab"},
		{"BITRISE_IO", "bitrise"},
		{"CIRCLECI", "circleci"},
		{"BUILDKITE", "buildkite"},
		{"JENKINS_URL", "jenkins"},
		{"TF_BUILD", "azure-pipelines"},
		{"TRAVIS", "travis"},
		{"CODEBUILD_BUILD_ID", "codebuild"},
		{"TEAMCITY_VERSION", "teamcity"},
		{"EAS_BUILD", "eas"},
		{"CM_BUILD_ID", "codemagic"},
	}
	for _, k := range known {
		if os.Getenv(k.env) != "" {
			return k.name
		}
	}
	if envSet("CI") {
		return "other"
	}
	return "none"
}

// only returns the one value in set, "mixed" for several, or "" for none.
// A blank value (a device whose kind or OS could not be read) is not a
// different value: it is skipped.
func only(set map[string]bool) string {
	delete(set, "")
	switch len(set) {
	case 0:
		return ""
	case 1:
		for v := range set {
			return v
		}
	}
	return "mixed"
}

func deviceKind(platform string, d report.Device) string {
	switch {
	case platform == "android" && d.IsSimulator:
		return "emulator"
	case platform == "ios" && d.IsSimulator:
		return "simulator"
	case platform == "android" || platform == "ios":
		if platform == "android" && strings.HasPrefix(d.ID, "emulator-") {
			return "emulator"
		}
		return "real"
	}
	return "unknown"
}

var leadingNumber = regexp.MustCompile(`^\d{1,3}`)

func osMajor(platform, version string) string {
	if platform != "android" && platform != "ios" {
		return ""
	}
	n := leadingNumber.FindString(strings.TrimSpace(version))
	if n == "" {
		return ""
	}
	return platform + " " + n
}

// failureKind is the type of the first failed step of a failed flow (the
// report's classification), limited to the Worker's list.
func failureKind(reportDir string, f *report.FlowEntry) string {
	if f.DataFile == "" {
		return "unknown"
	}
	detail, err := report.ReadFlowDetail(filepath.Join(reportDir, f.DataFile))
	if err != nil {
		return "unknown"
	}
	cmd := firstFailed(detail.Commands)
	if cmd == nil || cmd.Error == nil {
		return "unknown"
	}
	switch cmd.Error.Type {
	case "assertion", "timeout", "element_not_found", "app_crash", "network", "unknown":
		return cmd.Error.Type
	}
	return "other"
}

func firstFailed(cmds []report.Command) *report.Command {
	for i := range cmds {
		if cmds[i].Status != report.StatusFailed {
			continue
		}
		if sub := firstFailed(cmds[i].SubCommands); sub != nil {
			return sub
		}
		return &cmds[i]
	}
	return nil
}

var flagName = regexp.MustCompile(`^--?[a-z][a-z0-9-]{0,40}$`)

// flagNames keeps well-formed flag names (never values), sorted, unique.
func flagNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		// A flag is reported under its aliases too; one-letter names are
		// those aliases (-p for --platform), so only the full names count.
		if len(strings.TrimLeft(n, "-")) < 2 {
			continue
		}
		if !strings.HasPrefix(n, "-") {
			n = "--" + n
		}
		if flagName.MatchString(n) && !seen[n] && len(out) < 40 {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

var versionPattern = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}(\.\d{1,4})?$`)

func versionToken(v string) string {
	v = strings.TrimPrefix(v, "v")
	if versionPattern.MatchString(v) {
		return v
	}
	return "dev"
}

func oneOf(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	if v == "" {
		return ""
	}
	return "other"
}
