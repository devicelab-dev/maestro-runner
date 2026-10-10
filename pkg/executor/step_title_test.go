package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestStepTitleUsesLabel(t *testing.T) {
	labelled := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn, StepLabel: "Go to documents"}, Selector: flow.Selector{ID: "header-navigate-docs"}}
	fr := &FlowRunner{script: NewScriptEngine()}
	if _, got := fr.stepNames(labelled); got != "Go to documents" {
		t.Errorf("stepNames(labelled) = %q, want the label", got)
	}
	plain := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: "header-navigate-docs"}}
	if _, got := fr.stepNames(plain); got != plain.Describe() {
		t.Errorf("stepNames(plain) title = %q, want the description %q", got, plain.Describe())
	}
}
