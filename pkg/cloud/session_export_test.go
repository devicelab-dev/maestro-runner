package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func readRegistry(t *testing.T, path string) registryDoc {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var doc registryDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("registry is not valid JSON: %v\n%s", err, b)
	}
	return doc
}

func meta(device, session, url string) map[string]string {
	return map[string]string{MetaDeviceID: device, MetaSessionID: session, MetaAppiumURL: url}
}

func TestSessionExporter_WritesValidEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	e := NewSessionExporter(path)

	m := meta("dev-1", "sess-1", "http://localhost:4723")
	if err := e.OnFlowStart(m, 0, 1, "login.yaml", "login.yaml"); err != nil {
		t.Fatal(err)
	}
	doc := readRegistry(t, path)
	if len(doc.Sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(doc.Sessions))
	}
	s := doc.Sessions[0]
	if s.SessionID != "sess-1" || s.AppiumURL != "http://localhost:4723" || s.DeviceID != "dev-1" || s.Flow != "login.yaml" || s.Status != "active" {
		t.Errorf("unexpected entry: %+v", s)
	}
	if doc.SchemaVersion != 1 || doc.UpdatedAt == "" || s.UpdatedAt == "" {
		t.Errorf("missing schema/timestamps: %+v / %+v", doc, s)
	}
}

func TestSessionExporter_NewSessionPerFlowUpdatesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	e := NewSessionExporter(path)

	// Same device, new session id each flow → one entry, updated.
	_ = e.OnFlowStart(meta("dev-1", "sess-A", "url"), 0, 2, "flow1", "flow1.yaml")
	_ = e.OnFlowStart(meta("dev-1", "sess-B", "url"), 1, 2, "flow2", "flow2.yaml")

	doc := readRegistry(t, path)
	if len(doc.Sessions) != 1 {
		t.Fatalf("new-session-per-flow must update in place, got %d entries", len(doc.Sessions))
	}
	if doc.Sessions[0].SessionID != "sess-B" || doc.Sessions[0].Flow != "flow2" {
		t.Errorf("entry not updated to latest session/flow: %+v", doc.Sessions[0])
	}
}

func TestSessionExporter_ParallelDevicesDistinctEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	// Two exporter instances on the SAME path (as parallel workers get) must
	// share one registry → one file with both entries, no clobbering.
	e1 := NewSessionExporter(path)
	e2 := NewSessionExporter(path)

	var wg sync.WaitGroup
	for i, e := range []*SessionExporter{e1, e2} {
		wg.Add(1)
		go func(i int, e *SessionExporter) {
			defer wg.Done()
			dev := "dev-" + string(rune('A'+i))
			for f := 0; f < 20; f++ {
				_ = e.OnFlowStart(meta(dev, dev+"-sess", "url"), f, 20, "flow", "flow.yaml")
			}
		}(i, e)
	}
	wg.Wait()

	doc := readRegistry(t, path)
	if len(doc.Sessions) != 2 {
		t.Fatalf("parallel devices should yield 2 entries, got %d: %+v", len(doc.Sessions), doc.Sessions)
	}
}

func TestSessionExporter_ReportResultMarksClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	e := NewSessionExporter(path)
	m := meta("dev-1", "sess-1", "url")
	_ = e.OnFlowStart(m, 0, 1, "flow", "flow.yaml")
	if err := e.ReportResult("url", m, &TestResult{}); err != nil {
		t.Fatal(err)
	}
	doc := readRegistry(t, path)
	if len(doc.Sessions) != 1 || doc.Sessions[0].Status != "closed" {
		t.Errorf("expected session marked closed, got %+v", doc.Sessions)
	}
}

func TestSessionExporter_NoSessionIDNoWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	e := NewSessionExporter(path)
	if err := e.OnFlowStart(meta("dev-1", "", "url"), 0, 1, "flow", "flow.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("no session id → must not create a file")
	}
}
