package cdp

import (
	"errors"
	"testing"
)

func TestTransientEvalErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("element not found"), false},
		{"context destroyed", errors.New("Execution context was destroyed"), true},
		{"cannot find context", errors.New("Cannot find context with specified id"), true},
		{"navigated or closed", errors.New("Inspected target navigated or closed"), true},
		{"stale node", errors.New("Node with given id does not belong to the document"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transientEvalErr(c.err); got != c.want {
				t.Errorf("transientEvalErr(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
