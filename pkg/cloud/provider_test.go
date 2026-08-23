package cloud

import "testing"

func TestDetect_NoMatch_ReturnsNil(t *testing.T) {
	// Reset registry for isolated test
	mu.Lock()
	saved := factories
	factories = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		factories = saved
		mu.Unlock()
	}()

	if p := Detect("http://localhost:4723"); p != nil {
		t.Errorf("expected nil, got %q", p.Name())
	}
}

func TestDetect_MatchesProvider(t *testing.T) {
	mu.Lock()
	saved := factories
	factories = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		factories = saved
		mu.Unlock()
	}()

	Register(func(url string) Provider {
		if url == "https://example.com" {
			return &testProvider{name: "Example"}
		}
		return nil
	})

	p := Detect("https://example.com")
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if p.Name() != "Example" {
		t.Errorf("expected Example, got %q", p.Name())
	}
}

func TestDetect_FirstMatchWins(t *testing.T) {
	mu.Lock()
	saved := factories
	factories = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		factories = saved
		mu.Unlock()
	}()

	Register(func(url string) Provider { return &testProvider{name: "First"} })
	Register(func(url string) Provider { return &testProvider{name: "Second"} })

	p := Detect("anything")
	if p == nil || p.Name() != "First" {
		t.Errorf("expected First, got %v", p)
	}
}

func TestDetect_SkipsNilFactory(t *testing.T) {
	mu.Lock()
	saved := factories
	factories = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		factories = saved
		mu.Unlock()
	}()

	Register(func(url string) Provider { return nil })
	Register(func(url string) Provider { return &testProvider{name: "Fallback"} })

	p := Detect("anything")
	if p == nil || p.Name() != "Fallback" {
		t.Errorf("expected Fallback, got %v", p)
	}
}

// testProvider is a minimal Provider for registry tests.
type testProvider struct {
	name string
}

func (t *testProvider) Name() string { return t.name }
func (t *testProvider) ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string) {
}
func (t *testProvider) OnRunStart(meta map[string]string, totalFlows int) error { return nil }
func (t *testProvider) OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error {
	return nil
}
func (t *testProvider) OnFlowEnd(meta map[string]string, result *FlowResult) error { return nil }
func (t *testProvider) ReportResult(appiumURL string, meta map[string]string, result *TestResult) error {
	return nil
}

// TestProvider_Lifecycle_FiresInOrder verifies the full lifecycle sequence:
// ExtractMeta → OnRunStart → OnFlowStart → OnFlowEnd (per flow) → ReportResult.
func TestProvider_Lifecycle_FiresInOrder(t *testing.T) {
	type event struct {
		kind   string
		idx    int
		name   string
		passed bool
	}
	var events []event

	p := &lifecycleProvider{
		extract: func() { events = append(events, event{kind: "extract"}) },
		runStart: func(total int) {
			events = append(events, event{kind: "run", idx: total})
		},
		flowStart: func(flowIdx int, name string) {
			events = append(events, event{kind: "flowStart", idx: flowIdx, name: name})
		},
		flowEnd: func(name string, passed bool) {
			events = append(events, event{kind: "flowEnd", name: name, passed: passed})
		},
		report: func(passed bool) { events = append(events, event{kind: "report", passed: passed}) },
	}

	// Simulate the CLI lifecycle sequence for two flows.
	meta := map[string]string{}
	p.ExtractMeta("sess-1", nil, meta)
	if err := p.OnRunStart(meta, 2); err != nil {
		t.Fatalf("OnRunStart: %v", err)
	}
	if err := p.OnFlowStart(meta, 0, 2, "A", "a.yaml"); err != nil {
		t.Fatalf("OnFlowStart A: %v", err)
	}
	if err := p.OnFlowEnd(meta, &FlowResult{Name: "A", File: "a.yaml", Passed: true}); err != nil {
		t.Fatalf("OnFlowEnd A: %v", err)
	}
	if err := p.OnFlowStart(meta, 1, 2, "B", "b.yaml"); err != nil {
		t.Fatalf("OnFlowStart B: %v", err)
	}
	if err := p.OnFlowEnd(meta, &FlowResult{Name: "B", File: "b.yaml", Passed: false}); err != nil {
		t.Fatalf("OnFlowEnd B: %v", err)
	}
	if err := p.ReportResult("http://x", meta, &TestResult{Passed: false}); err != nil {
		t.Fatalf("ReportResult: %v", err)
	}

	want := []event{
		{kind: "extract"},
		{kind: "run", idx: 2},
		{kind: "flowStart", idx: 0, name: "A"},
		{kind: "flowEnd", name: "A", passed: true},
		{kind: "flowStart", idx: 1, name: "B"},
		{kind: "flowEnd", name: "B", passed: false},
		{kind: "report", passed: false},
	}
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d (%+v)", len(events), len(want), events)
	}
	for i, e := range events {
		if e != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, e, want[i])
		}
	}
}

type lifecycleProvider struct {
	extract   func()
	runStart  func(total int)
	flowStart func(flowIdx int, name string)
	flowEnd   func(name string, passed bool)
	report    func(passed bool)
}

func (p *lifecycleProvider) Name() string { return "lifecycle" }
func (p *lifecycleProvider) ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string) {
	p.extract()
}
func (p *lifecycleProvider) OnRunStart(meta map[string]string, totalFlows int) error {
	p.runStart(totalFlows)
	return nil
}
func (p *lifecycleProvider) OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error {
	p.flowStart(flowIdx, name)
	return nil
}
func (p *lifecycleProvider) OnFlowEnd(meta map[string]string, result *FlowResult) error {
	p.flowEnd(result.Name, result.Passed)
	return nil
}
func (p *lifecycleProvider) ReportResult(appiumURL string, meta map[string]string, result *TestResult) error {
	p.report(result.Passed)
	return nil
}
