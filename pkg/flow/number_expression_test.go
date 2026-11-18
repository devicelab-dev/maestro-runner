package flow

import "testing"

// Maestro reads extendedWaitUntil's, waitForAnimationToEnd's and
// scrollUntilVisible's timeout, and scrollUntilVisible's speed, as text and
// evaluates them when the step runs, so a flow can set them from a variable
// or quote them. Here they are ints, and such a flow failed to parse, which
// stopped every flow of the run before a device was touched.
func TestParse_NumberFieldExpressions(t *testing.T) {
	t.Run("a ${...} is kept for the expand pass", func(t *testing.T) {
		f, err := Parse([]byte(`
- extendedWaitUntil:
    visible: "Done"
    timeout: ${PAUSE_MS}
- waitForAnimationToEnd:
    timeout: ${PAUSE_MS * 2}
- scrollUntilVisible:
    element: "Row"
    timeout: ${PAUSE_MS}
    speed: ${SPEED}
`), "test.yaml")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		wait := f.Steps[0].(*WaitUntilStep)
		if wait.TimeoutExpr != "${PAUSE_MS}" || wait.TimeoutMs != 0 {
			t.Errorf("extendedWaitUntil: TimeoutExpr=%q TimeoutMs=%d, want ${PAUSE_MS} and 0", wait.TimeoutExpr, wait.TimeoutMs)
		}
		if wait.Visible == nil || wait.Visible.Text != "Done" {
			t.Errorf("extendedWaitUntil lost its selector: %+v", wait.Visible)
		}
		anim := f.Steps[1].(*WaitForAnimationToEndStep)
		if anim.TimeoutExpr != "${PAUSE_MS * 2}" {
			t.Errorf("waitForAnimationToEnd: TimeoutExpr=%q, want ${PAUSE_MS * 2}", anim.TimeoutExpr)
		}
		scroll := f.Steps[2].(*ScrollUntilVisibleStep)
		if scroll.TimeoutExpr != "${PAUSE_MS}" || scroll.SpeedExpr != "${SPEED}" || scroll.Speed != 0 {
			t.Errorf("scrollUntilVisible: TimeoutExpr=%q SpeedExpr=%q Speed=%d", scroll.TimeoutExpr, scroll.SpeedExpr, scroll.Speed)
		}
		if scroll.Element.Text != "Row" {
			t.Errorf("scrollUntilVisible lost its element: %+v", scroll.Element)
		}
	})

	t.Run("a quoted number is the number", func(t *testing.T) {
		f, err := Parse([]byte(`
- extendedWaitUntil:
    visible: "Done"
    timeout: "3000"
- waitForAnimationToEnd:
    timeout: '1500'
- scrollUntilVisible:
    element: "Row"
    timeout: "8000"
    speed: "40"
`), "test.yaml")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := f.Steps[0].(*WaitUntilStep).TimeoutMs; got != 3000 {
			t.Errorf("extendedWaitUntil TimeoutMs = %d, want 3000", got)
		}
		if got := f.Steps[1].(*WaitForAnimationToEndStep).TimeoutMs; got != 1500 {
			t.Errorf("waitForAnimationToEnd TimeoutMs = %d, want 1500", got)
		}
		scroll := f.Steps[2].(*ScrollUntilVisibleStep)
		if scroll.TimeoutMs != 8000 || scroll.Speed != 40 || scroll.TimeoutExpr != "" || scroll.SpeedExpr != "" {
			t.Errorf("scrollUntilVisible: TimeoutMs=%d Speed=%d TimeoutExpr=%q SpeedExpr=%q, want 8000, 40 and no expressions",
				scroll.TimeoutMs, scroll.Speed, scroll.TimeoutExpr, scroll.SpeedExpr)
		}
	})

	t.Run("a number is read in decimal, a timeout's underscores dropped", func(t *testing.T) {
		// yaml.v3 reads a leading zero as octal ("0100" = 64) and "08" as a
		// float; Maestro reads these fields with toLong and parseTimeoutMs.
		// scrollUntilVisible's "3_000" is a leniency: Maestro checks it with
		// parseTimeoutMs, then runs it with toLong, which refuses the `_`
		// (Orchestra.kt scrollUntilVisible); only its extendedWaitUntil
		// drops it.
		f, err := Parse([]byte(`
- extendedWaitUntil:
    visible: "Done"
    timeout: "0100"
- waitForAnimationToEnd:
    timeout: 0100
- scrollUntilVisible:
    element: "Row"
    timeout: "3_000"
    speed: "040"
- extendedWaitUntil:
    visible: "Done"
    timeout: 08
`), "test.yaml")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := f.Steps[0].(*WaitUntilStep).TimeoutMs; got != 100 {
			t.Errorf(`timeout: "0100" = %d, want 100`, got)
		}
		if got := f.Steps[1].(*WaitForAnimationToEndStep).TimeoutMs; got != 100 {
			t.Errorf("timeout: 0100 = %d, want 100", got)
		}
		scroll := f.Steps[2].(*ScrollUntilVisibleStep)
		if scroll.TimeoutMs != 3000 || scroll.Speed != 40 {
			t.Errorf(`timeout: "3_000", speed: "040" = %d, %d; want 3000, 40`, scroll.TimeoutMs, scroll.Speed)
		}
		if got := f.Steps[3].(*WaitUntilStep).TimeoutMs; got != 8 {
			t.Errorf("timeout: 08 = %d, want 8", got)
		}
	})

	t.Run("a literal that is not a number is still refused", func(t *testing.T) {
		for _, steps := range []string{
			"- extendedWaitUntil:\n    visible: x\n    timeout: soon\n",
			"- waitForAnimationToEnd:\n    timeout: \"soon\"\n",
			"- scrollUntilVisible:\n    element: x\n    speed: 400\n",
			"- scrollUntilVisible:\n    element: x\n    speed: \"4_0\"\n",
		} {
			if _, err := Parse([]byte(steps), "test.yaml"); err == nil {
				t.Errorf("parse of %q succeeded, want an error", steps)
			}
		}
	})
}
