package jsengine

import (
	"reflect"
	"testing"
)

// Console output is kept for the report, per step, as well as printed.
func TestConsoleLogsAreKept(t *testing.T) {
	e := New()
	if _, err := e.Eval(`console.log("a", 1); console.warn("careful"); console.error("bad")`); err != nil {
		t.Fatal(err)
	}
	want := []string{"a 1", "WARN: careful", "ERROR: bad"}
	if got := e.TakeLogs(); !reflect.DeepEqual(got, want) {
		t.Errorf("TakeLogs() = %q, want %q", got, want)
	}
	if got := e.TakeLogs(); len(got) != 0 {
		t.Errorf("TakeLogs() after taking = %q, want none", got)
	}
}
