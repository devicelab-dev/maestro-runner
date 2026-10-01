package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestStepTitleUsesLabel(t *testing.T) {
	labelled := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn, StepLabel: "Go to documents"}, Selector: flow.Selector{ID: "header-navigate-docs"}}
	if got := stepTitle(labelled); got != "Go to documents" {
		t.Errorf("stepTitle(labelled) = %q, want the label", got)
	}
	plain := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: "header-navigate-docs"}}
	if got, want := stepTitle(plain), plain.Describe(); got != want {
		t.Errorf("stepTitle(plain) = %q, want the description %q", got, want)
	}
}
