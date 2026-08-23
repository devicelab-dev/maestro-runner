package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// An iOS crash report (.ips) is two JSON documents concatenated with a newline
// between them: a short header naming the process, and the payload holding the
// actual diagnosis. Nothing about that is guessable from the file extension, so
// splitting on the first newline is the whole trick.
//
// Everything below works without a dSYM. Symbolication buys you function names;
// it is not needed to answer the questions a test run asks — which app died,
// when, and why — so a missing dSYM must never suppress the report.

// IOSCrashReport is the part of an .ips a test run can act on.
type IOSCrashReport struct {
	BundleID       string    // the app that died
	ProcessName    string    // executable name, present even when bundleID is not
	AppVersion     string    // marketing version, when the header carries one
	OSVersion      string    // train and build, e.g. "iPhone OS 18.6 (22G86)"
	Timestamp      time.Time // zero when unparseable — never a reason to drop a report
	Exception      string    // e.g. EXC_BAD_ACCESS, EXC_CRASH, EXC_RESOURCE
	Signal         string    // e.g. SIGSEGV, SIGABRT
	Reason         string    // termination reason: watchdog, memory limit, and so on
	Namespace      string    // termination namespace, e.g. SIGNAL, ASSERTION, JETSAM
	FaultingBinary string    // resolved via usedImages, so the culprit has a name
	IsJetsam       bool      // killed for memory rather than having crashed
}

// ipsHeader is the first document.
type ipsHeader struct {
	BundleID   string `json:"bundleID"`
	AppVersion string `json:"app_version"`
	Name       string `json:"name"`
	OSVersion  string `json:"os_version"`
	Timestamp  string `json:"timestamp"`
}

// ipsPayload is the second, and is deliberately partial — the format carries far
// more than a test runner has any use for.
type ipsPayload struct {
	Exception struct {
		Type    string `json:"type"`
		Signal  string `json:"signal"`
		Subtype string `json:"subtype"`
	} `json:"exception"`
	Termination struct {
		Namespace string `json:"namespace"`
		Reason    string `json:"reason"`
		Indicator string `json:"indicator"`
	} `json:"termination"`
	FaultingThread *int `json:"faultingThread"`
	Threads        []struct {
		Frames []struct {
			ImageIndex int `json:"imageIndex"`
		} `json:"frames"`
	} `json:"threads"`
	UsedImages []struct {
		Name string `json:"name"`
	} `json:"usedImages"`
	ProcName    string `json:"procName"`
	CaptureTime string `json:"captureTime"`
	OSVersion   struct {
		Train string `json:"train"`
		Build string `json:"build"`
	} `json:"osVersion"`
}

// ParseIPSReport reads an iOS .ips crash report.
//
// It is tolerant by design: a report that only parses halfway is still worth
// surfacing, because "the app died and here is roughly when" beats silence. Only
// a file whose header will not parse at all is rejected.
func ParseIPSReport(data []byte) (*IOSCrashReport, error) {
	headerRaw, payloadRaw, found := bytes.Cut(data, []byte("\n"))
	if !found {
		// Some reports carry only the header line.
		headerRaw = data
	}

	var header ipsHeader
	if err := json.Unmarshal(bytes.TrimSpace(headerRaw), &header); err != nil {
		return nil, fmt.Errorf("parse .ips header: %w", err)
	}

	report := &IOSCrashReport{
		BundleID:    header.BundleID,
		ProcessName: header.Name,
		AppVersion:  header.AppVersion,
		OSVersion:   header.OSVersion,
		Timestamp:   parseIPSTime(header.Timestamp),
	}

	var payload ipsPayload
	if len(bytes.TrimSpace(payloadRaw)) == 0 ||
		json.Unmarshal(bytes.TrimSpace(payloadRaw), &payload) != nil {
		return report, nil // header-only report; still useful
	}

	report.Exception = payload.Exception.Type
	report.Signal = payload.Exception.Signal
	report.Namespace = payload.Termination.Namespace
	report.Reason = payload.Termination.Reason
	if report.ProcessName == "" {
		report.ProcessName = payload.ProcName
	}
	if report.Timestamp.IsZero() {
		report.Timestamp = parseIPSTime(payload.CaptureTime)
	}
	if report.OSVersion == "" && payload.OSVersion.Train != "" {
		report.OSVersion = strings.TrimSpace(payload.OSVersion.Train + " (" + payload.OSVersion.Build + ")")
	}
	report.IsJetsam = isJetsamKill(report)
	report.FaultingBinary = faultingBinary(&payload)
	return report, nil
}

// Summary renders a one-line explanation for a flow failure.
func (r *IOSCrashReport) Summary() string {
	who := r.BundleID
	if who == "" {
		who = r.ProcessName
	}
	if who == "" {
		who = "an app"
	}

	switch {
	case r.IsJetsam:
		return fmt.Sprintf("%s was terminated for memory (jetsam)", who)
	case r.Reason != "":
		return fmt.Sprintf("%s crashed: %s", who, r.Reason)
	case r.Exception != "" && r.Signal != "":
		return fmt.Sprintf("%s crashed: %s (%s)", who, r.Exception, r.Signal)
	case r.Exception != "":
		return fmt.Sprintf("%s crashed: %s", who, r.Exception)
	default:
		return fmt.Sprintf("%s crashed", who)
	}
}

// isJetsamKill reports whether the process was killed to reclaim memory rather
// than having crashed. That distinction is the whole value of the report for a
// test run: an OOM kill is a binary, non-flaky fact about the app, where a
// memory *threshold* never is.
func isJetsamKill(r *IOSCrashReport) bool {
	if strings.EqualFold(r.Namespace, "JETSAM") {
		return true
	}
	haystack := strings.ToLower(r.Reason + " " + r.Exception)
	return strings.Contains(haystack, "jetsam") ||
		strings.Contains(haystack, "memory limit") ||
		strings.Contains(haystack, "per-process-limit")
}

// faultingBinary resolves the faulting thread's topmost frame to the image that
// owns it, which names the culprit without needing a dSYM.
func faultingBinary(p *ipsPayload) string {
	if p.FaultingThread == nil {
		return ""
	}
	i := *p.FaultingThread
	if i < 0 || i >= len(p.Threads) || len(p.Threads[i].Frames) == 0 {
		return ""
	}
	img := p.Threads[i].Frames[0].ImageIndex
	if img < 0 || img >= len(p.UsedImages) {
		return ""
	}
	return p.UsedImages[img].Name
}

// parseIPSTime accepts the formats Apple has used for .ips timestamps, and
// returns the zero time rather than an error — a report with an unreadable
// timestamp is still a report.
func parseIPSTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.99 -0700",
		"2006-01-02 15:04:05.99 Z0700",
		"2006-01-02 15:04:05 -0700",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
