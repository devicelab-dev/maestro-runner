package cli

import (
	"encoding/json"
	"os"
	"testing"
)

const sampleAndroidXML = `<hierarchy rotation="0">
  <node class="android.widget.FrameLayout" text="" content-desc="" resource-id="" bounds="[0,0][1080,2400]" enabled="true" clickable="false" focused="false" checked="false" selected="false">
    <node class="android.widget.EditText" text="Hi" hint-text="Email" content-desc="field" resource-id="header-Appearance" bounds="[10,20][110,70]" enabled="true" clickable="true" focused="true" scrollable="false"/>
  </node>
</hierarchy>`

// The shape Expo's image-comparison server reads: attributes["resource-id"]
// and attributes["bounds"], nested under children.
type expoViewNode struct {
	Attributes map[string]string `json:"attributes"`
	Children   []expoViewNode    `json:"children"`
}

func findByResourceID(n expoViewNode, id string) *expoViewNode {
	if n.Attributes["resource-id"] == id {
		return &n
	}
	for _, c := range n.Children {
		if f := findByResourceID(c, id); f != nil {
			return f
		}
	}
	return nil
}

func TestFormatMaestroHierarchyAndroid(t *testing.T) {
	out, err := formatMaestroHierarchy([]byte(sampleAndroidXML))
	if err != nil {
		t.Fatal(err)
	}
	var root expoViewNode
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	field := findByResourceID(root, "header-Appearance")
	if field == nil {
		t.Fatalf("no node with resource-id header-Appearance:\n%s", out)
	}
	want := map[string]string{
		"text": "Hi", "accessibilityText": "field", "hintText": "Email", "resource-id": "header-Appearance",
		"bounds": "[10,20][110,70]", "clickable": "true", "enabled": "true", "focused": "true",
		"class": "android.widget.EditText",
	}
	for k, v := range want {
		if field.Attributes[k] != v {
			t.Errorf("attributes[%q] = %q, want %q", k, field.Attributes[k], v)
		}
	}
	if _, ok := field.Attributes["scrollable"]; ok {
		t.Error(`"false" values must be dropped, as Maestro does`)
	}
	var raw struct {
		Children []struct {
			Attributes map[string]string `json:"attributes"`
			Clickable  *bool             `json:"clickable"`
			Enabled    *bool             `json:"enabled"`
		} `json:"children"`
	}
	_ = json.Unmarshal([]byte(out), &raw)
	if len(raw.Children) != 1 || raw.Children[0].Clickable != nil || raw.Children[0].Enabled == nil {
		t.Errorf("flags: want enabled only on the frame, got %+v", raw.Children)
	}
	if _, ok := raw.Children[0].Attributes["text"]; ok {
		t.Error("empty values must be dropped")
	}
}

func TestHierarchyFormat(t *testing.T) {
	old := os.Args[0]
	t.Cleanup(func() { os.Args[0] = old })
	for argv0, want := range map[string]string{
		"/home/me/.maestro/bin/maestro": "maestro",
		"/usr/local/bin/maestro-runner": "tree",
	} {
		os.Args[0] = argv0
		if got, _ := hierarchyFormat(""); got != want {
			t.Errorf("argv0 %q: format %q, want %q", argv0, got, want)
		}
	}
	os.Args[0] = "/home/me/.maestro/bin/maestro"
	if got, _ := hierarchyFormat("tree"); got != "tree" {
		t.Errorf("--format tree must win, got %q", got)
	}
	if _, err := hierarchyFormat("xml"); err == nil {
		t.Error("unknown format accepted")
	}
}
