// Package cloud — loopback provider.
//
// The loopback provider is a debug-only stand-in that mimics a cloud provider
// against a local Appium server. It exists so we can validate the per-worker
// hook plumbing (OnFlowStart / OnFlowEnd / ReportResult) end-to-end without
// real cloud credentials.
//
// It is double-gated to keep it from firing accidentally:
//   1. MAESTRO_CLOUD_DEBUG=1 must be set in the environment.
//   2. The Appium URL must point at localhost or 127.0.0.1.
//
// On ReportResult it writes <OutputDir>/loopback-cloud-<jobID>.json with the
// full TestResult, so a parallel run produces one file per worker that can be
// diffed to confirm per-session filtering.

package cloud

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

func init() {
	Register(newLoopback)
}

// loopbackJobCounter generates a unique synthetic jobID per session so that
// per-worker reset-and-redetect in createAppiumWorkers still produces distinct
// jobs for parallel runs.
var loopbackJobCounter int64

// loopbackEventMu serialises JSONL appends across goroutines so concurrent
// workers don't interleave bytes within a single event line.
var loopbackEventMu sync.Mutex

func newLoopback(appiumURL string) Provider {
	if os.Getenv("MAESTRO_CLOUD_DEBUG") != "1" {
		return nil
	}
	lower := strings.ToLower(appiumURL)
	if !strings.Contains(lower, "localhost") && !strings.Contains(lower, "127.0.0.1") {
		return nil
	}
	return &loopbackProvider{}
}

type loopbackProvider struct{}

func (p *loopbackProvider) Name() string { return "Loopback (debug)" }

func (p *loopbackProvider) ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string) {
	jobID := sessionID
	if jobID == "" {
		jobID = fmt.Sprintf("loopback-%d", atomic.AddInt64(&loopbackJobCounter, 1))
	}
	meta["jobID"] = jobID
	meta["type"] = "loopback"
	logger.Info("[loopback-cloud] ExtractMeta sessionID=%q jobID=%q", sessionID, jobID)
}

func (p *loopbackProvider) OnRunStart(meta map[string]string, totalFlows int) error {
	logger.Info("[loopback-cloud] OnRunStart jobID=%q totalFlows=%d", meta["jobID"], totalFlows)
	appendLoopbackEvent(meta, map[string]interface{}{
		"event":      "OnRunStart",
		"jobID":      meta["jobID"],
		"totalFlows": totalFlows,
	})
	return nil
}

func (p *loopbackProvider) OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error {
	logger.Info("[loopback-cloud] OnFlowStart jobID=%q [%d/%d] name=%q file=%q",
		meta["jobID"], flowIdx+1, totalFlows, name, file)
	appendLoopbackEvent(meta, map[string]interface{}{
		"event":      "OnFlowStart",
		"jobID":      meta["jobID"],
		"flowIdx":    flowIdx,
		"totalFlows": totalFlows,
		"name":       name,
		"file":       file,
	})
	return nil
}

func (p *loopbackProvider) OnFlowEnd(meta map[string]string, result *FlowResult) error {
	logger.Info("[loopback-cloud] OnFlowEnd jobID=%q name=%q passed=%v duration=%dms err=%q",
		meta["jobID"], result.Name, result.Passed, result.Duration, result.Error)
	appendLoopbackEvent(meta, map[string]interface{}{
		"event":    "OnFlowEnd",
		"jobID":    meta["jobID"],
		"name":     result.Name,
		"file":     result.File,
		"passed":   result.Passed,
		"duration": result.Duration,
		"error":    result.Error,
	})
	return nil
}

func (p *loopbackProvider) ReportResult(appiumURL string, meta map[string]string, result *TestResult) error {
	jobID := strings.TrimSpace(meta["jobID"])
	if jobID == "" {
		return fmt.Errorf("loopback: no jobID in meta")
	}

	flowNames := make([]string, 0, len(result.Flows))
	for _, f := range result.Flows {
		flowNames = append(flowNames, f.Name)
	}
	logger.Info("[loopback-cloud] ReportResult jobID=%q passed=%v total=%d (passed=%d failed=%d) flows=%v",
		jobID, result.Passed, result.Total, result.PassedCount, result.FailedCount, flowNames)

	appendLoopbackEvent(meta, map[string]interface{}{
		"event":       "ReportResult",
		"jobID":       jobID,
		"passed":      result.Passed,
		"total":       result.Total,
		"passedCount": result.PassedCount,
		"failedCount": result.FailedCount,
		"duration":    result.Duration,
		"flows":       flowNames,
	})

	if result.OutputDir == "" {
		return nil
	}
	if err := os.MkdirAll(result.OutputDir, 0o755); err != nil {
		return fmt.Errorf("loopback: mkdir output dir: %w", err)
	}
	path := filepath.Join(result.OutputDir, "loopback-cloud-"+sanitizeJobID(jobID)+".json")
	payload := map[string]interface{}{
		"jobID":  jobID,
		"meta":   meta,
		"result": result,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("loopback: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("loopback: write %s: %w", path, err)
	}
	logger.Info("[loopback-cloud] wrote %s", path)
	return nil
}

// appendLoopbackEvent appends one JSON line to <OutputDir>/loopback-cloud-<jobID>.events.jsonl.
// Best-effort — logs and continues on failure so a missing/unwritable OutputDir
// can't break the test run. The output dir is read from meta["outputDir"] when
// the call site provides it; ReportResult passes its own. For OnRunStart /
// OnFlowStart / OnFlowEnd we don't have OutputDir on the call, so we fall back
// to MAESTRO_LOOPBACK_OUT or skip if unset.
func appendLoopbackEvent(meta map[string]string, payload map[string]interface{}) {
	dir := strings.TrimSpace(os.Getenv("MAESTRO_LOOPBACK_OUT"))
	if dir == "" {
		dir = strings.TrimSpace(meta["outputDir"])
	}
	if dir == "" {
		return // no destination — log-only mode
	}
	jobID := strings.TrimSpace(meta["jobID"])
	if jobID == "" {
		jobID = "unknown"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Debug("[loopback-cloud] mkdir %s: %v", dir, err)
		return
	}

	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	line, err := json.Marshal(payload)
	if err != nil {
		logger.Debug("[loopback-cloud] marshal event: %v", err)
		return
	}

	path := filepath.Join(dir, "loopback-cloud-"+sanitizeJobID(jobID)+".events.jsonl")
	loopbackEventMu.Lock()
	defer loopbackEventMu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Debug("[loopback-cloud] open %s: %v", path, err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		logger.Debug("[loopback-cloud] write %s: %v", path, err)
	}
}

// sanitizeJobID produces a filesystem-safe slug from a session ID.
func sanitizeJobID(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
