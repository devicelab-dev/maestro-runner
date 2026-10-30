package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// rect is an element's place on screen.
type rect struct{ X, Y, W, H int }

func (r rect) area() int {
	if r.W <= 0 || r.H <= 0 {
		return 0
	}
	return r.W * r.H
}

// iou is the overlap of two rects over their union, 0 to 1.
func iou(a, b rect) float64 {
	x1, y1 := max(a.X, b.X), max(a.Y, b.Y)
	x2, y2 := min(a.X+a.W, b.X+b.W), min(a.Y+a.H, b.Y+b.H)
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	inter := (x2 - x1) * (y2 - y1)
	union := a.area() + b.area() - inter
	if union <= 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// element is a labelled element: its text, or its content-desc when it has
// no text, and where it is.
type element struct {
	Label string
	ID    string
	Rect  rect
}

// agentTree is the shape `maestro-runner hierarchy` prints (pkg/cli hNode).
type agentTree struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Text   string `json:"text"`
	Bounds *struct {
		X      int `json:"x"`
		Y      int `json:"y"`
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"bounds"`
	Children []agentTree `json:"children"`
}

// agentElements reads the labelled elements of a `maestro-runner hierarchy`
// tree. Its text is the node's text, or its content-desc when it has none.
func agentElements(r io.Reader) ([]element, error) {
	var root agentTree
	if err := json.NewDecoder(r).Decode(&root); err != nil {
		return nil, fmt.Errorf("parse agent hierarchy: %w", err)
	}
	var out []element
	var walk func(n agentTree, systemUI bool)
	walk = func(n agentTree, systemUI bool) {
		// The status and navigation bars are SystemUI windows, which
		// uiautomator dump leaves out; their icons carry no id of their own.
		systemUI = systemUI || strings.HasPrefix(n.ID, systemUIPrefix)
		if label := strings.TrimSpace(n.Text); label != "" && n.Bounds != nil && !systemUI {
			e := element{Label: label, ID: n.ID, Rect: rect{n.Bounds.X, n.Bounds.Y, n.Bounds.Width, n.Bounds.Height}}
			if e.Rect.area() > 0 {
				out = append(out, e)
			}
		}
		for _, c := range n.Children {
			walk(c, systemUI)
		}
	}
	walk(root, false)
	return out, nil
}

const systemUIPrefix = "com.android.systemui:"

type dumpNode struct {
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []dumpNode `xml:"node"`
}

func (n dumpNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

var boundsRe = regexp.MustCompile(`^\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]$`)

func parseBounds(s string) (rect, bool) {
	m := boundsRe.FindStringSubmatch(s)
	if m == nil {
		return rect{}, false
	}
	v := make([]int, 4)
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return rect{v[0], v[1], v[2] - v[0], v[3] - v[1]}, true
}

// dumpElements reads the labelled elements of a `uiautomator dump`, and the
// bounds of its root window, which is the active window only.
func dumpElements(r io.Reader) ([]element, rect, error) {
	var doc struct {
		Nodes []dumpNode `xml:"node"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, rect{}, fmt.Errorf("parse uiautomator dump: %w", err)
	}
	var out []element
	var screen rect
	var walk func(n dumpNode)
	walk = func(n dumpNode) {
		b, ok := parseBounds(n.attr("bounds"))
		if ok && b.area() > screen.area() {
			screen = b
		}
		label := strings.TrimSpace(n.attr("text"))
		if label == "" {
			label = strings.TrimSpace(n.attr("content-desc"))
		}
		// An element with no area (bounds clipped to nothing at the
		// screen's edge) is not on screen; leave it out.
		if label != "" && ok && b.area() > 0 {
			out = append(out, element{Label: label, ID: n.attr("resource-id"), Rect: b})
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range doc.Nodes {
		walk(n)
	}
	return out, screen, nil
}

// result is how far the agent's elements and the reference agree.
type result struct {
	Reference, Agent, Matched int
	Recall, Precision         float64
	Missing                   []element // in the reference, not found in the agent's
	Extra                     []element // in the agent's (within the window), not in the reference
}

// compare matches every reference element to an unused agent element with
// the same label and an IoU of at least minIoU. The agent reads every
// window (status bar, navigation bar, keyboard) and uiautomator dump only
// the active one, so precision counts only the agent's elements centred in
// the dump's window (agentElements already leaves SystemUI out).
func compare(ref []element, window rect, agent []element, minIoU float64) result {
	var inWindow []element
	for _, e := range agent {
		cx, cy := e.Rect.X+e.Rect.W/2, e.Rect.Y+e.Rect.H/2
		if cx < window.X || cy < window.Y || cx >= window.X+window.W || cy >= window.Y+window.H {
			continue
		}
		inWindow = append(inWindow, e)
	}
	used := make([]bool, len(inWindow))
	res := result{Reference: len(ref), Agent: len(inWindow)}
	for _, r := range ref {
		best, bestIoU := -1, minIoU
		for i, a := range inWindow {
			if used[i] || a.Label != r.Label {
				continue
			}
			if v := iou(a.Rect, r.Rect); v >= bestIoU {
				best, bestIoU = i, v
			}
		}
		if best < 0 {
			res.Missing = append(res.Missing, r)
			continue
		}
		used[best] = true
		res.Matched++
	}
	for i, a := range inWindow {
		if !used[i] {
			res.Extra = append(res.Extra, a)
		}
	}
	res.Recall, res.Precision = 1, 1
	if res.Reference > 0 {
		res.Recall = float64(res.Matched) / float64(res.Reference)
	}
	if res.Agent > 0 {
		res.Precision = float64(res.Matched) / float64(res.Agent)
	}
	return res
}
