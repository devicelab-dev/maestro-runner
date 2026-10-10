package cli

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Maestro's `maestro hierarchy` prints its own tree format, and tools parse
// it: Expo's image-comparison server finds a view by attributes["resource-id"]
// and crops it by attributes["bounds"]. --format maestro prints that format.

// maestroTreeNode is Maestro's TreeNode as `maestro hierarchy` prints it:
// attributes with empty and "false" values removed, the five flags only when
// true, children always present.
type maestroTreeNode struct {
	Attributes map[string]string `json:"attributes"`
	Children   []maestroTreeNode `json:"children"`
	Clickable  *bool             `json:"clickable,omitempty"`
	Enabled    *bool             `json:"enabled,omitempty"`
	Focused    *bool             `json:"focused,omitempty"`
	Checked    *bool             `json:"checked,omitempty"`
	Selected   *bool             `json:"selected,omitempty"`
}

// hierarchyFormat is the output format: --format when given, otherwise
// "maestro" when this binary runs as `maestro` (installed in Maestro's place,
// where callers expect Maestro's output) and "tree" otherwise.
func hierarchyFormat(flag string) (string, error) {
	switch flag {
	case "tree", "maestro":
		return flag, nil
	case "":
		if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "maestro" {
			return "maestro", nil
		}
		return "tree", nil
	}
	return "", fmt.Errorf("unknown --format %q (use tree or maestro)", flag)
}

// formatMaestroHierarchy renders a driver's raw hierarchy in Maestro's format.
// Android UIAutomator XML maps attribute by attribute as Maestro's Android
// driver does; other sources go through the normalized tree.
func formatMaestroHierarchy(raw []byte) (string, error) {
	var root maestroTreeNode
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '<' {
		var x rawXML
		if err := xml.Unmarshal(trimmed, &x); err != nil {
			return "", fmt.Errorf("parse hierarchy XML: %w", err)
		}
		if strings.EqualFold(x.XMLName.Local, "hierarchy") || hasNodeChildren(x) {
			root = maestroFromAndroidXML(x)
			return marshalMaestroTree(root)
		}
	}
	n, err := parseHierarchy(raw)
	if err != nil {
		return "", err
	}
	return marshalMaestroTree(maestroFromNode(n))
}

func marshalMaestroTree(root maestroTreeNode) (string, error) {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// maestroAndroidAttrs maps UIAutomator XML attributes to Maestro's names, in
// the order Maestro's AndroidDriver.mapHierarchy reads them.
var maestroAndroidAttrs = []struct{ xml, maestro string }{
	{"text", "text"},
	{"content-desc", "accessibilityText"},
	{"hintText", "hintText"},
	{"hint-text", "hintText"},
	{"resource-id", "resource-id"},
	{"clickable", "clickable"},
	{"bounds", "bounds"},
	{"enabled", "enabled"},
	{"focused", "focused"},
	{"checked", "checked"},
	{"scrollable", "scrollable"},
	{"selected", "selected"},
	{"class", "class"},
	{"important-for-accessibility", "important-for-accessibility"},
	{"error", "error"},
}

func maestroFromAndroidXML(n rawXML) maestroTreeNode {
	attrs := map[string]string{}
	has := func(name string) (string, bool) {
		for _, a := range n.Attrs {
			if a.Name.Local == name {
				return a.Value, true
			}
		}
		return "", false
	}
	for _, m := range maestroAndroidAttrs {
		if v, ok := has(m.xml); ok && v != "" && v != "false" {
			attrs[m.maestro] = v
		}
	}
	if class, _ := has("class"); class == "android.widget.Toast" {
		attrs["ignoreBoundsFiltering"] = "true"
	}
	node := maestroTreeNode{Attributes: attrs, Children: []maestroTreeNode{}}
	flag := func(name string) *bool {
		if v, _ := has(name); v == "true" {
			return boolPtr(true)
		}
		return nil
	}
	node.Clickable, node.Enabled, node.Focused = flag("clickable"), flag("enabled"), flag("focused")
	node.Checked, node.Selected = flag("checked"), flag("selected")
	for _, c := range n.Children {
		node.Children = append(node.Children, maestroFromAndroidXML(c))
	}
	return node
}

// maestroFromNode builds Maestro's shape from the normalized tree (iOS, web,
// JSON snapshots): text, resource-id and bounds, the attributes tools use.
func maestroFromNode(n hNode) maestroTreeNode {
	attrs := map[string]string{}
	if n.Text != "" {
		attrs["text"] = n.Text
	}
	if n.ID != "" {
		attrs["resource-id"] = n.ID
	}
	if n.Bounds != nil {
		b := n.Bounds
		attrs["bounds"] = fmt.Sprintf("[%d,%d][%d,%d]", b.X, b.Y, b.X+b.Width, b.Y+b.Height)
	}
	node := maestroTreeNode{Attributes: attrs, Children: []maestroTreeNode{}}
	if n.Enabled == nil || *n.Enabled {
		node.Enabled = boolPtr(true)
	}
	if n.Focused != nil && *n.Focused {
		node.Focused = boolPtr(true)
	}
	if n.Checked != nil && *n.Checked {
		node.Checked = boolPtr(true)
	}
	if n.Selected != nil && *n.Selected {
		node.Selected = boolPtr(true)
	}
	for _, c := range n.Children {
		node.Children = append(node.Children, maestroFromNode(c))
	}
	return node
}
