package cloud

import (
	"errors"
	"testing"
)

type fakeProvider struct {
	name        string
	flowStarts  int
	reportCalls int
	extractSeen string
	err         error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) ExtractMeta(sessionID string, _ map[string]interface{}, meta map[string]string) {
	f.extractSeen = sessionID
	meta[f.name] = sessionID
}
func (f *fakeProvider) OnRunStart(map[string]string, int) error { return f.err }
func (f *fakeProvider) OnFlowStart(map[string]string, int, int, string, string) error {
	f.flowStarts++
	return f.err
}
func (f *fakeProvider) OnFlowEnd(map[string]string, *FlowResult) error { return f.err }
func (f *fakeProvider) ReportResult(string, map[string]string, *TestResult) error {
	f.reportCalls++
	return f.err
}

func TestComposite_NilAndSingle(t *testing.T) {
	if Composite() != nil {
		t.Error("empty Composite should be nil")
	}
	if Composite(nil, nil) != nil {
		t.Error("all-nil Composite should be nil")
	}
	p := &fakeProvider{name: "a"}
	if got := Composite(nil, p); got != Provider(p) {
		t.Error("single non-nil member should be returned unwrapped")
	}
}

func TestComposite_FansOutToAll(t *testing.T) {
	a := &fakeProvider{name: "a"}
	b := &fakeProvider{name: "b"}
	c := Composite(a, b)

	meta := map[string]string{}
	c.ExtractMeta("sess-1", nil, meta)
	if meta["a"] != "sess-1" || meta["b"] != "sess-1" {
		t.Errorf("ExtractMeta should reach both members: %v", meta)
	}
	if err := c.OnFlowStart(meta, 0, 1, "flow", "f.yaml"); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	_ = c.ReportResult("url", meta, &TestResult{})
	if a.flowStarts != 1 || b.flowStarts != 1 {
		t.Errorf("OnFlowStart not fanned out: a=%d b=%d", a.flowStarts, b.flowStarts)
	}
	if a.reportCalls != 1 || b.reportCalls != 1 {
		t.Errorf("ReportResult not fanned out: a=%d b=%d", a.reportCalls, b.reportCalls)
	}
	if c.Name() != "a+b" {
		t.Errorf("Name() = %q, want a+b", c.Name())
	}
}

func TestComposite_AggregatesErrorsAndKeepsGoing(t *testing.T) {
	boom := errors.New("boom")
	a := &fakeProvider{name: "a", err: boom}
	b := &fakeProvider{name: "b"} // healthy
	c := Composite(a, b)

	err := c.OnFlowStart(map[string]string{}, 0, 1, "flow", "f.yaml")
	if !errors.Is(err, boom) {
		t.Errorf("expected aggregated error to contain boom, got %v", err)
	}
	if b.flowStarts != 1 {
		t.Error("a failing member must not stop the others")
	}
}
