package core

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// IndexOutOfRange reports whether a selector's index names no element among
// count matches. `index: 1` with a single match used to fall back to the first
// match, so "there is no second row" failed and a tap hit the wrong row (#188).
// Negative indexes count from the end. An empty or non-numeric index is not
// out of range: those keep their existing handling.
func IndexOutOfRange(count int, index string) bool {
	i, err := strconv.Atoi(strings.TrimSpace(index))
	if err != nil {
		return false
	}
	if i < 0 {
		i += count
	}
	return i < 0 || i >= count
}

// ParseIndex reads a selector's index as Maestro does: any finite number,
// truncated ("2.0" is 2), so a value computed in JavaScript still works.
// Anything else fails: a non-numeric index used to fall back to the first
// match without a word, so `index: ${ROW}` with ROW unset ("undefined")
// tapped the first row and the step passed.
func ParseIndex(index string) (int, error) {
	s := strings.TrimSpace(index)
	if i, err := strconv.Atoi(s); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid index %q: index must be a whole number; "+
			"if it comes from a variable, make sure the variable resolves to a number", index)
	}
	return int(f), nil
}

// IndexError is nil when index is empty or names one of count matches, else
// why not: not a number, or no element at that position. Negative indexes
// count from the end.
func IndexError(count int, index string) error {
	if strings.TrimSpace(index) == "" {
		return nil
	}
	i, err := ParseIndex(index)
	if err != nil {
		return err
	}
	if i < 0 {
		i += count
	}
	if i < 0 || i >= count {
		return fmt.Errorf("no element at index %s: %d match(es)", strings.TrimSpace(index), count)
	}
	return nil
}

// SortByPosition returns elems ordered as Maestro orders matches before it
// applies a selector's index: top to bottom, then left to right. An element
// with no bounds (all zero) goes last; ties keep their incoming order.
// Ordering clickable matches first instead made `index: 0` pick a clickable
// navigation-bar button over the text view above it (#254).
func SortByPosition[T any](elems []T, bounds func(T) Bounds) []T {
	sorted := append([]T(nil), elems...)
	key := func(e T) (int, int) {
		b := bounds(e)
		if b == (Bounds{}) {
			return math.MaxInt, math.MaxInt
		}
		return b.Y, b.X
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		yi, xi := key(sorted[i])
		yj, xj := key(sorted[j])
		if yi != yj {
			return yi < yj
		}
		return xi < xj
	})
	return sorted
}

// DeepestMatches drops every match that contains another match, as Maestro
// does before it applies an index (Filters.deepestMatchingElement): a tab
// whose description is "Home" yields to the "Home" label inside it, so
// `index: 1` counts on to the next "Home" on screen. parent returns an
// element's parent, or the zero value at the root.
func DeepestMatches[T comparable](elems []T, parent func(T) T) []T {
	var zero T
	isMatch := make(map[T]bool, len(elems))
	for _, e := range elems {
		isMatch[e] = true
	}
	contains := make(map[T]bool)
	for _, e := range elems {
		for p := parent(e); p != zero; p = parent(p) {
			if isMatch[p] {
				contains[p] = true
			}
		}
	}
	if len(contains) == 0 {
		return elems
	}
	kept := make([]T, 0, len(elems)-len(contains))
	for _, e := range elems {
		if !contains[e] {
			kept = append(kept, e)
		}
	}
	return kept
}

// PickAnchor narrows a relative selector's anchors to the one the anchor's own
// index names (`childOf: {id: card, index: 1}`). As in Maestro
// (Filters.INDEX_COMPARATOR), the index counts the anchors by their place on
// screen: top to bottom, then left to right. Without an index every anchor is
// kept and the first one with matches wins. An index that is not a number or
// names no anchor is an error naming that index. The index used to be dropped,
// so `childOf` the second card searched the first one.
func PickAnchor[E any](anchors []E, index string, bounds func(E) Bounds) ([]E, error) {
	if strings.TrimSpace(index) == "" || len(anchors) == 0 {
		return anchors, nil
	}
	if err := IndexError(len(anchors), index); err != nil {
		return nil, fmt.Errorf("anchor element not found: %w", err)
	}
	onScreen := SortByPosition(anchors, bounds)
	i, _ := ParseIndex(index)
	if i < 0 {
		i += len(onScreen)
	}
	return []E{onScreen[i]}, nil
}
