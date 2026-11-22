package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// A test made of runFlows of other files: the first logs in and keeps the
// user in output, the second reads it.
var suiteFiles = map[string]string{
	"checkout.yaml": `appId: x
---
- runFlow: login.yaml
- runFlow:
    file: cart.yaml
    env:
      ITEM: socks
`,
	"login.yaml": `appId: x
env:
  USER: alice
---
- evalScript: ${output.user = USER}
- tapOn: Log in
`,
	"cart.yaml": `appId: x
---
- inputText: ${output.user + ' buys ' + ITEM}
`,
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func keys(m map[string]any) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// Maestro runs a test file as one flow, whatever its steps: a runFlow's
// steps run nested in it (Orchestra.kt runFlowCommand, runSubFlow), what one
// sub-flow keeps in output the next one reads (the JS engine's output
// binding lives as long as the flow), and the file is one test in the
// reports. A file made only of runFlows was split into a flow per runFlow,
// so the second sub-flow read output.user as undefined and the test never
// came back under its own name.
func TestRunFlowOnlyFileIsOneFlow(t *testing.T) {
	result, seen, out := runFiles(t, suiteFiles, "checkout.yaml", RunnerConfig{})
	if result.Status != report.StatusPassed || result.TotalFlows != 1 {
		t.Fatalf("status = %s with %d flows, want passed with 1: %+v", result.Status, result.TotalFlows, result.FlowResults)
	}
	want := "tapOn Log in, inputText alice buys socks"
	if got := strings.Join(typed(seen), ", "); got != want {
		t.Errorf("the driver got %q, want %q", got, want)
	}

	// The report a reader takes: one flow under the test's name and path,
	// its runFlows with their steps nested.
	index := readJSON(t, filepath.Join(out, "report.json"))
	flows, _ := index["flows"].([]any)
	if len(flows) != 1 {
		t.Fatalf("report.json has %d flows, want 1", len(flows))
	}
	entry := flows[0].(map[string]any)
	if entry["name"] != "checkout" || !strings.HasSuffix(entry["sourceFile"].(string), "checkout.yaml") || entry["status"] != "passed" {
		t.Errorf("report.json flow = name %v, sourceFile %v, status %v; want checkout, checkout.yaml, passed",
			entry["name"], entry["sourceFile"], entry["status"])
	}
	detail := readJSON(t, filepath.Join(out, entry["dataFile"].(string)))
	commands, _ := detail["commands"].([]any)
	if len(commands) != 2 {
		t.Fatalf("the flow has %d commands, want its 2 runFlows", len(commands))
	}
	for i, wantSubs := range []int{2, 1} {
		cmd := commands[i].(map[string]any)
		subs, _ := cmd["subCommands"].([]any)
		if cmd["type"] != "runFlow" || cmd["status"] != "passed" || len(subs) != wantSubs {
			t.Errorf("command %d = %v %v with %d sub-commands, want a passed runFlow with %d", i, cmd["type"], cmd["status"], len(subs), wantSubs)
		}
	}
	junit, err := os.ReadFile(filepath.Join(out, "junit-report.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(junit), "<testcase "); n != 1 || !strings.Contains(string(junit), `name="checkout"`) {
		t.Errorf("junit-report.xml has %d testcases, want the one named checkout:\n%s", n, junit)
	}

	// The same shape as a file whose runFlows sit next to a step of its
	// own, which was never split.
	mixed := map[string]string{"mixed.yaml": suiteFiles["checkout.yaml"] + "- tapOn: Done\n"}
	for name, body := range suiteFiles {
		if name != "checkout.yaml" {
			mixed[name] = body
		}
	}
	_, _, mixedOut := runFiles(t, mixed, "mixed.yaml", RunnerConfig{})
	mixedEntry := readJSON(t, filepath.Join(mixedOut, "report.json"))["flows"].([]any)[0].(map[string]any)
	if keys(entry) != keys(mixedEntry) {
		t.Errorf("report.json flow keys = %s, a mixed file's = %s", keys(entry), keys(mixedEntry))
	}
	mixedCommands := readJSON(t, filepath.Join(mixedOut, mixedEntry["dataFile"].(string)))["commands"].([]any)
	for i := range commands {
		if a, b := keys(commands[i].(map[string]any)), keys(mixedCommands[i].(map[string]any)); a != b {
			t.Errorf("command %d keys = %s, a mixed file's = %s", i, a, b)
		}
	}
}

// As in Maestro, a failing runFlow fails the test, and the runFlows after it
// do not run.
func TestRunFlowOnlyFileStopsAtItsFirstFailure(t *testing.T) {
	result, seen, _ := runFiles(t, map[string]string{
		"main.yaml": "appId: x\n---\n- runFlow: a.yaml\n- runFlow: b.yaml\n",
		"a.yaml":    "appId: x\n---\n- tapOn: Missing\n",
		"b.yaml":    "appId: x\n---\n- tapOn: Next\n",
	}, "main.yaml", RunnerConfig{})
	if result.Status != report.StatusFailed || result.TotalFlows != 1 {
		t.Fatalf("status = %s with %d flows, want failed with 1", result.Status, result.TotalFlows)
	}
	if got := strings.Join(typed(seen), ", "); got != "tapOn Missing" {
		t.Errorf("the driver got %q, want only the failing tap", got)
	}
}

// withSuiteKey is suiteFiles with suite: true in checkout.yaml's header.
func withSuiteKey() map[string]string {
	files := map[string]string{}
	for name, body := range suiteFiles {
		files[name] = body
	}
	files["checkout.yaml"] = strings.Replace(suiteFiles["checkout.yaml"], "appId: x\n", "appId: x\nsuite: true\n", 1)
	return files
}

// suite: true in the header keeps the split: each runFlow of the file is a
// flow of its own, with the suite's appId and env and the runFlow's env, and
// no output from the one before. A part's own env wins over the suite's, and
// the runFlow's over both.
func TestSuiteKeyRunsEachFileAsAFlow(t *testing.T) {
	files := withSuiteKey()
	files["checkout.yaml"] = strings.Replace(files["checkout.yaml"], "suite: true\n",
		"suite: true\nenv:\n  SHOP: north\n  USER: bob\n  ITEM: shoes\n", 1)
	files["login.yaml"] += "- inputText: ${USER + ' at ' + SHOP}\n"
	files["cart.yaml"] += "- inputText: ${ITEM + ' at ' + SHOP}\n"
	result, seen, out := runFiles(t, files, "checkout.yaml", RunnerConfig{})
	if result.TotalFlows != 2 {
		t.Fatalf("%d flows, want 2: %+v", result.TotalFlows, result.FlowResults)
	}
	want := "tapOn Log in, inputText alice at north, inputText undefined buys socks, inputText socks at north"
	if got := strings.Join(typed(seen), ", "); got != want {
		t.Errorf("the driver got %q, want %q", got, want)
	}
	var names []string
	for _, f := range readJSON(t, filepath.Join(out, "report.json"))["flows"].([]any) {
		names = append(names, f.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "login,cart" {
		t.Errorf("report.json flows = %v, want login and cart", names)
	}
}

// suite: true on a file that cannot be split runs it as one flow, as written
// (the validator refuses such a file before a run; a library caller that
// skips it gets the file run as it is).
func TestSuiteKeyOnAFileThatCannotSplitRunsItAsOneFlow(t *testing.T) {
	files := withSuiteKey()
	files["checkout.yaml"] += "- tapOn: Done\n"
	result, seen, _ := runFiles(t, files, "checkout.yaml", RunnerConfig{})
	if result.TotalFlows != 1 || result.Status != report.StatusPassed {
		t.Fatalf("status = %s with %d flows, want passed with 1", result.Status, result.TotalFlows)
	}
	if got := strings.Join(typed(seen), ", "); got != "tapOn Log in, inputText alice buys socks, tapOn Done" {
		t.Errorf("the driver got %q, want the file's steps as written", got)
	}
}

// suite: true on a file with its own onFlowStart or onFlowComplete runs it as
// one flow, its hooks included: split, its parts would run as flows of their
// own and the hooks never (the validator refuses such a file before a run).
func TestSuiteKeyOnAFileWithHooksRunsItAsOneFlow(t *testing.T) {
	files := withSuiteKey()
	files["checkout.yaml"] = strings.Replace(files["checkout.yaml"], "suite: true\n",
		"suite: true\nonFlowStart:\n  - tapOn: Start\nonFlowComplete:\n  - tapOn: End\n", 1)
	result, seen, _ := runFiles(t, files, "checkout.yaml", RunnerConfig{})
	if result.TotalFlows != 1 || result.Status != report.StatusPassed {
		t.Fatalf("status = %s with %d flows, want passed with 1", result.Status, result.TotalFlows)
	}
	if got := strings.Join(typed(seen), ", "); got != "tapOn Start, tapOn Log in, inputText alice buys socks, tapOn End" {
		t.Errorf("the driver got %q, want the file's hooks around its steps", got)
	}
}

// The parallel runner splits a suite file only when its header asks, as the
// sequential one does. It never split one before, so a suite gave one flow
// with --parallel and two without. Without the key the file is one flow there
// too: the second runFlow reads what the first kept in output, and the JUnit
// has the one test under the file's name.
func TestParallelRunnerSplitsOnlyASuiteKeyFile(t *testing.T) {
	for _, c := range []struct {
		files map[string]string
		typed string
		names []string
	}{
		{suiteFiles, "tapOn Log in, inputText alice buys socks", []string{"checkout"}},
		{withSuiteKey(), "tapOn Log in, inputText undefined buys socks", []string{"login", "cart"}},
	} {
		dir := t.TempDir()
		for name, body := range c.files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		f, err := flow.ParseFile(filepath.Join(dir, "checkout.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var seen []flow.Step
		drv := &mockDriver{
			platformFunc: func() *core.PlatformInfo { return &core.PlatformInfo{Platform: "android", DeviceID: "dev-a"} },
			executeFunc: func(step flow.Step) *core.CommandResult {
				seen = append(seen, step)
				return &core.CommandResult{Success: true}
			},
		}
		out := t.TempDir()
		pr := NewParallelRunner([]DeviceWorker{{ID: 0, DeviceID: "dev-a", Driver: drv, Cleanup: func() {}}}, RunnerConfig{
			OutputDir: out,
			Artifacts: ArtifactNever,
			Device:    report.Device{Name: "1 device", Platform: "android"},
		})
		result, err := pr.Run(context.Background(), []flow.Flow{*f})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.TotalFlows != len(c.names) || result.Status != report.StatusPassed {
			t.Errorf("suite: %v: status = %s with %d flows, want passed with %d", f.Config.Suite, result.Status, result.TotalFlows, len(c.names))
		}
		if got := strings.Join(typed(seen), ", "); got != c.typed {
			t.Errorf("suite: %v: the driver got %q, want %q", f.Config.Suite, got, c.typed)
		}
		junit, err := os.ReadFile(filepath.Join(out, "junit-report.xml"))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, m := range regexp.MustCompile(`<testcase id="[^"]*" name="([^"]*)"`).FindAllStringSubmatch(string(junit), -1) {
			names = append(names, m[1])
		}
		if strings.Join(names, ",") != strings.Join(c.names, ",") {
			t.Errorf("suite: %v: junit-report.xml testcases = %v, want %v", f.Config.Suite, names, c.names)
		}
	}
}

// Each way a runFlow-only file goes is in the run's log: split by its key,
// one flow (the default, with the key to split it), one flow because its key
// cannot be honoured, or nested because a runFlow runs it.
func TestSuiteDecisionIsLogged(t *testing.T) {
	cannot := withSuiteKey()
	cannot["checkout.yaml"] += "- tapOn: Done\n"
	// A suite file reached by runFlow runs nested, as any runFlow's file.
	nested := withSuiteKey()
	nested["suite.yaml"] = nested["checkout.yaml"]
	nested["checkout.yaml"] = "appId: x\n---\n- runFlow: suite.yaml\n- tapOn: Done\n"
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"suite key", withSuiteKey(), "checkout.yaml has suite: true: running its 2 runFlow files as separate flows"},
		{"no key", suiteFiles, "checkout.yaml is made only of runFlow steps: running it as one flow (suite: true in its header runs each as a flow of its own)"},
		{"cannot split", cannot, "[WARN] " + "DIR/checkout.yaml has suite: true, but step 3 is tapOn, not a runFlow: running it as one flow"},
		{"reached by runFlow", nested, "[WARN] " + "DIR/suite.yaml has suite: true, but a runFlow or retry runs it: running it as one sub-flow (suite: true splits only a top-level flow file)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "maestro-runner.log")
			if err := logger.Init(logPath); err != nil {
				t.Fatal(err)
			}
			_, _, out := runFiles(t, c.files, "checkout.yaml", RunnerConfig{})
			logger.Close()
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(c.want, "DIR", filepath.Dir(out), 1)
			if !strings.Contains(string(data), want) {
				t.Errorf("the log has no %q:\n%s", want, data)
			}
		})
	}
}
