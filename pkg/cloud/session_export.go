// Package cloud — Appium session exporter.
//
// SessionExporter is an always-on (opt-in) Provider that publishes the live
// Appium session(s) to a single well-known JSON file, so external tools
// (visual testing, observability, debuggers) can attach to the running
// session(s) without polling report artifacts. See issue #91.
//
// Design notes:
//   - One in-memory registry per file path is the source of truth (mutex-
//     guarded). The file is a rendered snapshot, rewritten in full on every
//     change via temp-file + atomic rename — readers never see a partial file.
//   - The registry is a process-global singleton keyed by path, so the
//     per-worker exporter instances used in parallel runs all funnel into one
//     registry → one consistent file (no clobbering).
//   - Entries are keyed by device, so a new session per flow updates in place
//     and parallel workers get distinct entries.
package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// registry singletons keyed by absolute file path.
var (
	registriesMu sync.Mutex
	registries   = map[string]*sessionRegistry{}
)

func registryFor(path string) *sessionRegistry {
	registriesMu.Lock()
	defer registriesMu.Unlock()
	if r, ok := registries[path]; ok {
		return r
	}
	r := &sessionRegistry{path: path, sessions: map[string]sessionEntry{}}
	registries[path] = r
	return r
}

type sessionEntry struct {
	DeviceID  string `json:"deviceId,omitempty"`
	SessionID string `json:"sessionId"`
	AppiumURL string `json:"appiumUrl"`
	Flow      string `json:"flow,omitempty"`
	Status    string `json:"status"` // "active" | "closed"
	UpdatedAt string `json:"updatedAt"`
}

type registryDoc struct {
	SchemaVersion int            `json:"schemaVersion"`
	UpdatedAt     string         `json:"updatedAt"`
	Sessions      []sessionEntry `json:"sessions"`
}

type sessionRegistry struct {
	mu       sync.Mutex
	path     string
	sessions map[string]sessionEntry // keyed by deviceID (falls back to sessionID)
}

func entryKey(e sessionEntry) string {
	if e.DeviceID != "" {
		return e.DeviceID
	}
	return e.SessionID
}

// set inserts or updates an entry and atomically rewrites the file.
func (r *sessionRegistry) set(e sessionEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.UpdatedAt = nowUTC()
	r.sessions[entryKey(e)] = e
	return r.flush()
}

// markClosed flips an entry to "closed" (kept in the file as a tombstone so a
// reader can tell a session ended vs. was never there). Returns nil if absent.
func (r *sessionRegistry) markClosed(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.sessions[key]
	if !ok {
		return nil
	}
	e.Status = "closed"
	e.UpdatedAt = nowUTC()
	r.sessions[key] = e
	return r.flush()
}

// flush renders the whole registry and atomically replaces the file.
// Caller must hold r.mu.
func (r *sessionRegistry) flush() error {
	doc := registryDoc{SchemaVersion: 1, UpdatedAt: nowUTC()}
	for _, e := range r.sessions {
		doc.Sessions = append(doc.Sessions, e)
	}
	sort.Slice(doc.Sessions, func(i, j int) bool {
		return entryKey(doc.Sessions[i]) < entryKey(doc.Sessions[j])
	})
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(r.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path) // atomic on the same filesystem
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// SessionExporter publishes live Appium sessions to a well-known file.
// Multiple instances pointing at the same path share one registry singleton.
type SessionExporter struct {
	reg *sessionRegistry
}

// NewSessionExporter returns an exporter writing to path (shared per path).
func NewSessionExporter(path string) *SessionExporter {
	return &SessionExporter{reg: registryFor(path)}
}

func (e *SessionExporter) Name() string { return "session-export" }

// ExtractMeta guarantees the session id is in meta even when no cloud provider
// populated it (the exporter is the canonical source for self-managed sessions).
func (e *SessionExporter) ExtractMeta(sessionID string, _ map[string]interface{}, meta map[string]string) {
	if sessionID != "" {
		meta[MetaSessionID] = sessionID
	}
}

func (e *SessionExporter) OnRunStart(map[string]string, int) error { return nil }

// OnFlowStart publishes the current session for this device/flow. Fires before
// the flow's first step, so external tools can attach in time. A new session
// per flow updates the same device entry in place.
func (e *SessionExporter) OnFlowStart(meta map[string]string, _, _ int, name, _ string) error {
	sid := meta[MetaSessionID]
	if sid == "" {
		return nil // nothing to publish yet
	}
	return e.reg.set(sessionEntry{
		DeviceID:  meta[MetaDeviceID],
		SessionID: sid,
		AppiumURL: meta[MetaAppiumURL],
		Flow:      name,
		Status:    "active",
	})
}

func (e *SessionExporter) OnFlowEnd(map[string]string, *FlowResult) error { return nil }

// ReportResult marks the device's session closed at the end of the run.
func (e *SessionExporter) ReportResult(_ string, meta map[string]string, _ *TestResult) error {
	key := meta[MetaDeviceID]
	if key == "" {
		key = meta[MetaSessionID]
	}
	if key == "" {
		return nil
	}
	return e.reg.markClosed(key)
}
