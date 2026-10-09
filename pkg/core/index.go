package core

import (
	"fmt"
	"math"
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
