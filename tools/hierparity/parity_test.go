package main

import (
	"bytes"
	"strings"
	"testing"
)

const testDump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?><hierarchy rotation="0">
<node text="" content-desc="" resource-id="" bounds="[0,0][1080,2340]">
<node text="Network and internet" resource-id="android:id/title" bounds="[198,864][708,938]" />
<node text="" content-desc="Search settings" resource-id="" bounds="[44,620][1036,763]" />
<node text="Apps" resource-id="android:id/title" bounds="[198,1348][324,1422]" />
<node text="Clipped" resource-id="" bounds="[198,2316][683,2274]" />
</node></hierarchy>`

// The agent's read: every window, the status bar's icons under a SystemUI
// node with no id of their own; "Apps" is lost.
const testAgent = `{"type":"Root","children":[
 {"type":"FrameLayout","id":"com.android.systemui:id/status_bar","bounds":{"x":0,"y":0,"width":1080,"height":136},
  "children":[{"type":"ImageView","text":"Clock notification:","bounds":{"x":504,"y":30,"width":41,"height":106}}]},
 {"type":"FrameLayout","bounds":{"x":0,"y":0,"width":1080,"height":2340},"children":[
  {"type":"TextView","id":"android:id/title","text":"Network and internet","bounds":{"x":198,"y":866,"width":510,"height":72}},
  {"type":"LinearLayout","text":"Search settings","bounds":{"x":44,"y":620,"width":992,"height":143}},
  {"type":"TextView","text":"Only here","bounds":{"x":0,"y":1600,"width":300,"height":60}}
 ]}]}`

func TestCompare(t *testing.T) {
	agent, err := agentElements(strings.NewReader(testAgent))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range agent {
		if e.Label == "Clock notification:" {
			t.Error("a SystemUI icon was counted")
		}
	}
	ref, window, err := dumpElements(strings.NewReader(testDump))
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) != 3 {
		t.Fatalf("reference elements = %d, want 3 (the clipped one left out)", len(ref))
	}
	res := compare(ref, window, agent, 0.5)
	if res.Matched != 2 || len(res.Missing) != 1 || res.Missing[0].Label != "Apps" {
		t.Errorf("matched %d, missing %v; want 2 matched and Apps missing", res.Matched, res.Missing)
	}
	if len(res.Extra) != 1 || res.Extra[0].Label != "Only here" {
		t.Errorf("extra %v, want Only here", res.Extra)
	}
	if res.Recall < 0.66 || res.Recall > 0.67 || res.Precision < 0.66 || res.Precision > 0.67 {
		t.Errorf("recall %.3f precision %.3f, want 2/3 each", res.Recall, res.Precision)
	}
}

// The same label far from where the dump has it is not the same element.
func TestCompareNeedsOverlap(t *testing.T) {
	ref := []element{{Label: "OK", Rect: rect{0, 0, 100, 50}}}
	agent := []element{{Label: "OK", Rect: rect{0, 500, 100, 50}}}
	if res := compare(ref, rect{0, 0, 1080, 2340}, agent, 0.5); res.Matched != 0 {
		t.Error("matched an element in another place")
	}
}

func TestRunExitStatus(t *testing.T) {
	dir := t.TempDir()
	if err := saveReads(dir, []byte(testAgent), []byte(testDump)); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--agent", dir + "/agent.json", "--dump", dir + "/dump.xml"}, &out, &errOut); code != 1 {
		t.Errorf("2/3 recall: exit %d, want 1\n%s", code, out.String())
	}
	if code := run([]string{"--agent", dir + "/agent.json", "--dump", dir + "/dump.xml", "--min-recall", "0.6", "--min-precision", "0.6"}, &out, &errOut); code != 0 {
		t.Errorf("thresholds 0.6: exit %d, want 0", code)
	}
	if code := run(nil, &out, &errOut); code != 2 {
		t.Errorf("no arguments: exit %d, want 2", code)
	}
}
