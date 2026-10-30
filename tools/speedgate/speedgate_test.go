package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

func ms(v int64) *int64 { return &v }

func cmd(typ string, d int64) report.Command {
	return report.Command{Type: typ, Status: report.StatusPassed, Duration: ms(d)}
}

// writeReport writes a fixture report directory: report.json plus one flow
// file per command list, laid out as the runner writes them.
func writeReport(t *testing.T, driver, platform string, flows ...[]report.Command) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	index := report.Index{
		Status:        report.StatusPassed,
		Device:        report.Device{Platform: platform},
		MaestroRunner: report.RunnerInfo{Driver: driver},
	}
	for i, cmds := range flows {
		id := "flow-00" + string(rune('0'+i))
		dataFile := filepath.Join("flows", id+".json")
		index.Flows = append(index.Flows, report.FlowEntry{ID: id, DataFile: dataFile, Status: report.StatusPassed})
		writeJSON(t, filepath.Join(dir, dataFile), report.FlowDetail{ID: id, Commands: cmds})
	}
	writeJSON(t, filepath.Join(dir, "report.json"), index)
	return dir
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// suite is one run's commands: three taps, two inputs, a nested repeat, and
// steps that must not count (failed, no duration).
func suite(tap, input int64) []report.Command {
	failed := cmd("tapOn", 10000)
	failed.Status = report.StatusFailed
	noDur := report.Command{Type: "tapOn", Status: report.StatusPassed}
	repeat := report.Command{
		Type: "repeat", Status: report.StatusPassed, Duration: ms(99999),
		SubCommands: []report.Command{cmd("tapOn", tap)},
	}
	return []report.Command{
		cmd("launchApp", 2000),
		cmd("tapOn", tap), cmd("tapOn", tap+10),
		cmd("inputText", input), cmd("inputText", input),
		repeat, failed, noDur,
	}
}

func TestLoadSamplesCountsPassedLeafSteps(t *testing.T) {
	r1 := writeReport(t, "devicelab", "android", suite(300, 500))
	r2 := writeReport(t, "devicelab", "android", suite(300, 500))
	s, err := LoadSamples([]string{r1, filepath.Join(r2, "report.json")})
	if err != nil {
		t.Fatal(err)
	}
	if s.Runs != 2 || s.Driver != "devicelab" || s.Platform != "android" {
		t.Fatalf("runs/driver/platform = %d/%s/%s", s.Runs, s.Driver, s.Platform)
	}
	if got := len(s.ByType["tapOn"]); got != 6 { // 3 per run: 2 top-level + 1 in repeat
		t.Errorf("tapOn samples = %d, want 6", got)
	}
	if _, ok := s.ByType["repeat"]; ok {
		t.Error("container step counted")
	}
	for _, d := range s.ByType["tapOn"] {
		if d == 10000 {
			t.Error("failed step counted")
		}
	}
}

func TestLoadSamplesRefusesMixedDrivers(t *testing.T) {
	r1 := writeReport(t, "devicelab", "android", suite(300, 500))
	r2 := writeReport(t, "uiautomator2", "android", suite(300, 500))
	if _, err := LoadSamples([]string{r1, r2}); err == nil {
		t.Fatal("expected an error pooling two drivers")
	}
}

func TestLoadSamplesMissingReport(t *testing.T) {
	if _, err := LoadSamples([]string{filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Fatal("expected an error for a missing report")
	}
}

func TestMedian(t *testing.T) {
	cases := []struct {
		in   []int64
		want int64
	}{
		{nil, 0},
		{[]int64{7}, 7},
		{[]int64{3, 1, 2}, 2},
		{[]int64{4, 1, 3, 2}, 2}, // lower middle
	}
	for _, c := range cases {
		if got := median(c.in); got != c.want {
			t.Errorf("median(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestBuildBaselineDropsRareTypes(t *testing.T) {
	s := &Samples{Runs: 1, ByType: map[string][]int64{
		"tapOn":     {100, 200, 300},
		"launchApp": {2000},
	}}
	b := BuildBaseline(s, 3)
	if b.Commands["tapOn"].MedianMs != 200 || b.Commands["tapOn"].Samples != 3 {
		t.Errorf("tapOn = %+v", b.Commands["tapOn"])
	}
	if _, ok := b.Commands["launchApp"]; ok {
		t.Error("type under min-samples baselined")
	}
}

func TestCompareThresholds(t *testing.T) {
	base := &Baseline{Commands: map[string]BaselineEntry{
		"tapOn":     {MedianMs: 1000},
		"inputText": {MedianMs: 100},
		"swipe":     {MedianMs: 1000},
		"back":      {MedianMs: 1000},
		"gone":      {MedianMs: 500},
		"rare":      {MedianMs: 100},
	}}
	s := &Samples{ByType: map[string][]int64{
		"tapOn":     {1300, 1300, 1300}, // +30%, +300ms: regressed
		"inputText": {140, 140, 140},    // +40% but only +40ms: ok
		"swipe":     {1150, 1150, 1150}, // +150ms but only +15%: ok
		"back":      {500, 500, 500},    // faster: ok
		"rare":      {900},              // too few samples
		"new":       {10, 10, 10},       // not in baseline
	}}
	results := Compare(base, s, Thresholds{Pct: 20, AbsMs: 50, MinSamples: 3})
	got := map[string]Result{}
	for _, r := range results {
		got[r.Type] = r
	}
	if !got["tapOn"].Regressed {
		t.Error("tapOn should regress")
	}
	for _, typ := range []string{"inputText", "swipe", "back", "rare", "new", "gone"} {
		if got[typ].Regressed {
			t.Errorf("%s should not regress", typ)
		}
	}
	if got["rare"].Unjudged == "" || got["new"].Unjudged != notInBaseline || !got["gone"].MissingCur {
		t.Errorf("unjudged reasons wrong: rare=%q new=%q gone=%+v", got["rare"].Unjudged, got["new"].Unjudged, got["gone"])
	}
	if Regressions(results) != 1 {
		t.Errorf("regressions = %d, want 1", Regressions(results))
	}
	if results[0].Type != "back" {
		t.Errorf("results not sorted: first = %s", results[0].Type)
	}
}

func TestRunWriteThenCompare(t *testing.T) {
	baseline := filepath.Join(t.TempDir(), "sub", "baseline.json")
	runs := []string{
		writeReport(t, "devicelab", "android", suite(300, 500)),
		writeReport(t, "devicelab", "android", suite(310, 500)),
	}
	var out, errb bytes.Buffer
	if code := run(append([]string{"--baseline", baseline, "--write-baseline"}, runs...), &out, &errb); code != 0 {
		t.Fatalf("write-baseline exit %d: %s", code, errb.String())
	}
	b, err := ReadBaseline(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if b.Runs != 2 || b.Driver != "devicelab" || b.Commands["tapOn"].MedianMs == 0 {
		t.Fatalf("baseline = %+v", b)
	}
	if _, ok := b.Commands["launchApp"]; ok {
		t.Error("launchApp has 2 samples, below the default 3")
	}

	out.Reset()
	if code := run(append([]string{"--baseline", baseline}, runs...), &out, &errb); code != 0 {
		t.Fatalf("same runs should pass, exit %d\n%s", code, out.String())
	}

	slow := writeReport(t, "devicelab", "android", suite(900, 500))
	out.Reset()
	if code := run([]string{"--baseline", baseline, slow}, &out, &errb); code != 1 {
		t.Fatalf("slow run should fail with 1, got %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "REGRESSED") {
		t.Errorf("output lacks REGRESSED:\n%s", out.String())
	}
}

func TestRunUsageAndMissingBaseline(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 2 {
		t.Errorf("no args: exit %d, want 2", code)
	}
	r := writeReport(t, "devicelab", "android", suite(300, 500))
	errb.Reset()
	code := run([]string{"--baseline", filepath.Join(t.TempDir(), "none.json"), r}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "--write-baseline") {
		t.Errorf("missing baseline: exit %d, stderr %q", code, errb.String())
	}
}

func TestRunWarnsOnDriverMismatch(t *testing.T) {
	baseline := filepath.Join(t.TempDir(), "b.json")
	if err := WriteBaseline(baseline, &Baseline{Version: 1, Driver: "wda", Platform: "ios", Commands: map[string]BaselineEntry{}}); err != nil {
		t.Fatal(err)
	}
	r := writeReport(t, "devicelab", "android", suite(300, 500))
	var out, errb bytes.Buffer
	run([]string{"--baseline", baseline, r}, &out, &errb)
	if !strings.Contains(errb.String(), "warning: baseline is ios/wda") {
		t.Errorf("no mismatch warning: %q", errb.String())
	}
}
