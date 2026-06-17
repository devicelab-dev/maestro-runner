// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

// Package flutter provides Flutter VM Service integration for element finding fallback.
package flutter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// SemanticsNode represents a node in the Flutter semantics tree.
type SemanticsNode struct {
	ID         int
	Rect       Rect
	Label      string
	Identifier string
	Hint       string
	Value      string
	Flags      []string
	Actions    []string
	Children   []*SemanticsNode
}

// Rect represents a rectangle in logical pixels (LTRB format).
type Rect struct {
	Left, Top, Right, Bottom float64
}

// ToBounds converts a logical-pixel Rect to physical-pixel core.Bounds using the pixel ratio.
func (r Rect) ToBounds(pixelRatio float64) core.Bounds {
	return core.Bounds{
		X:      int(r.Left * pixelRatio),
		Y:      int(r.Top * pixelRatio),
		Width:  int((r.Right - r.Left) * pixelRatio),
		Height: int((r.Bottom - r.Top) * pixelRatio),
	}
}

var (
	reNodeID     = regexp.MustCompile(`SemanticsNode#(\d+)`)
	reRect       = regexp.MustCompile(`Rect\.fromLTRB\(\s*([\d.]+),\s*([\d.]+),\s*([\d.]+),\s*([\d.]+)\s*\)`)
	rePixelRatio = regexp.MustCompile(`scaled by\s+(\d+\.?\d*)x`)
	reIdentifier = regexp.MustCompile(`identifier:\s*"(.*)"`)
	reFlags      = regexp.MustCompile(`flags:\s*(.+)`)
	reActions    = regexp.MustCompile(`actions:\s*(.+)`)

	// Multiline-aware regexes for block-level parsing (label/hint/value can span lines)
	reMultiLabel = regexp.MustCompile(`(?s)label:[^"]*"(.*?)"`)
	reMultiHint  = regexp.MustCompile(`(?s)hint:[^"]*"(.*?)"`)
	reMultiValue = regexp.MustCompile(`(?s)value:[^"]*"(.*?)"`)
)

// ParseSemanticsTree parses the text output of debugDumpSemanticsTreeInTraversalOrder
// into a tree of SemanticsNode and extracts the pixel ratio.
func ParseSemanticsTree(dump string) (root *SemanticsNode, pixelRatio float64, err error) {
	lines := strings.Split(dump, "\n")
	if len(lines) == 0 {
		return nil, 0, fmt.Errorf("empty semantics dump")
	}

	pixelRatio = 1.0
	if m := rePixelRatio.FindStringSubmatch(dump); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			pixelRatio = v
		}
	}

	// Parse nodes with their indentation depths, accumulating all lines per node.
	type nodeEntry struct {
		depth int
		node  *SemanticsNode
		lines []string
	}
	var entries []nodeEntry

	for _, line := range lines {
		m := reNodeID.FindStringSubmatch(line)
		if m == nil {
			// This line contains properties for the current node
			if len(entries) > 0 {
				entries[len(entries)-1].lines = append(entries[len(entries)-1].lines, line)
			}
			continue
		}

		id, _ := strconv.Atoi(m[1])
		node := &SemanticsNode{ID: id}

		// Determine depth by counting leading spaces/special chars
		depth := indentDepth(line)
		entries = append(entries, nodeEntry{depth: depth, node: node, lines: []string{line}})
	}

	if len(entries) == 0 {
		return nil, 0, fmt.Errorf("no semantics nodes found in dump")
	}

	// Parse each node's accumulated block (handles multiline labels/hints/values)
	for _, e := range entries {
		parseNodeBlock(strings.Join(e.lines, "\n"), e.node)
	}

	// Build tree from indentation
	root = entries[0].node
	// Stack tracks the path from root to current parent
	type stackEntry struct {
		depth int
		node  *SemanticsNode
	}
	stack := []stackEntry{{depth: entries[0].depth, node: root}}

	for i := 1; i < len(entries); i++ {
		e := entries[i]
		// Pop stack until we find a parent with lesser depth
		for len(stack) > 1 && stack[len(stack)-1].depth >= e.depth {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].node
		parent.Children = append(parent.Children, e.node)
		stack = append(stack, stackEntry{depth: e.depth, node: e.node})
	}

	// Compute accurate pixel ratio from Node #0 (physical) / Node #1 (logical)
	// if both start at origin. This is more precise than the "scaled by Nx" annotation
	// which may be rounded (e.g., reports 2.8x when actual ratio is 2.75).
	if len(entries) >= 2 {
		rootRect := entries[0].node.Rect
		childRect := entries[1].node.Rect
		if rootRect.Left == 0 && rootRect.Top == 0 && rootRect.Right > 0 &&
			childRect.Left == 0 && childRect.Top == 0 && childRect.Right > 0 {
			computed := rootRect.Right / childRect.Right
			if computed > 1.1 && computed < 5.0 {
				pixelRatio = computed
			}
		}
	}

	return root, pixelRatio, nil
}

// parseNodeBlock extracts properties from a node's accumulated text block.
// Handles multiline label/hint/value that span multiple lines in the dump.
func parseNodeBlock(block string, node *SemanticsNode) {
	if m := reRect.FindStringSubmatch(block); m != nil {
		node.Rect.Left, _ = strconv.ParseFloat(m[1], 64)
		node.Rect.Top, _ = strconv.ParseFloat(m[2], 64)
		node.Rect.Right, _ = strconv.ParseFloat(m[3], 64)
		node.Rect.Bottom, _ = strconv.ParseFloat(m[4], 64)
	}
	if m := reIdentifier.FindStringSubmatch(block); m != nil {
		node.Identifier = m[1]
	}

	// Use multiline-aware regexes for label/hint/value (they can span lines)
	if m := reMultiLabel.FindStringSubmatch(block); m != nil {
		node.Label = cleanMultilineValue(m[1])
	}
	if m := reMultiHint.FindStringSubmatch(block); m != nil {
		node.Hint = cleanMultilineValue(m[1])
	}
	if m := reMultiValue.FindStringSubmatch(block); m != nil {
		node.Value = cleanMultilineValue(m[1])
	}

	// Flags and actions are always single-line, parse line by line
	for _, line := range strings.Split(block, "\n") {
		if m := reFlags.FindStringSubmatch(line); m != nil {
			parts := strings.Split(m[1], ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					node.Flags = append(node.Flags, p)
				}
			}
		}
		if m := reActions.FindStringSubmatch(line); m != nil {
			parts := strings.Split(m[1], ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					node.Actions = append(node.Actions, p)
				}
			}
		}
	}
}

// cleanMultilineValue removes tree-drawing characters and normalizes whitespace
// from a multiline label/hint/value captured from the semantics dump.
func cleanMultilineValue(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		// Strip tree-drawing characters from the left
		line = strings.TrimLeft(line, " \t│├└─╎╏┊┆")
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// indentDepth returns the number of leading whitespace/tree-drawing characters.
func indentDepth(line string) int {
	depth := 0
	for _, ch := range line {
		switch ch {
		case ' ', '│', '├', '└', '─', '╎', '╏', '┊', '┆':
			depth++
		default:
			return depth
		}
	}
	return depth
}

// FindByIdentifier finds all nodes whose Identifier matches the given string.
func FindByIdentifier(root *SemanticsNode, id string) []*SemanticsNode {
	var results []*SemanticsNode
	walkTree(root, func(n *SemanticsNode) {
		if n.Identifier == id {
			results = append(results, n)
		}
	})
	return results
}

// FindByLabel finds all nodes whose Label contains the given text.
func FindByLabel(root *SemanticsNode, text string) []*SemanticsNode {
	var results []*SemanticsNode
	walkTree(root, func(n *SemanticsNode) {
		if n.Label != "" && strings.Contains(n.Label, text) {
			results = append(results, n)
		}
	})
	return results
}

// FindByHint finds all nodes whose Hint contains the given text.
func FindByHint(root *SemanticsNode, hint string) []*SemanticsNode {
	var results []*SemanticsNode
	walkTree(root, func(n *SemanticsNode) {
		if n.Hint != "" && strings.Contains(n.Hint, hint) {
			results = append(results, n)
		}
	})
	return results
}

// HasFlag checks if a semantics node has the given flag.
func HasFlag(n *SemanticsNode, flag string) bool {
	for _, f := range n.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

func walkTree(node *SemanticsNode, fn func(*SemanticsNode)) {
	if node == nil {
		return
	}
	fn(node)
	for _, child := range node.Children {
		walkTree(child, fn)
	}
}
