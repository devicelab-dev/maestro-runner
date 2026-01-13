package executor

import (
	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// commandResultToElement converts core.CommandResult to report.Element.
func commandResultToElement(r *core.CommandResult) *report.Element {
	if r == nil || r.Element == nil {
		return nil
	}

	el := r.Element
	element := &report.Element{
		Found: true,
		ID:    el.ID,
		Text:  el.Text,
		Class: el.Class,
	}

	// Convert bounds
	element.Bounds = &report.Bounds{
		X:      el.Bounds.X,
		Y:      el.Bounds.Y,
		Width:  el.Bounds.Width,
		Height: el.Bounds.Height,
	}

	return element
}

// commandResultToError converts core.CommandResult error to report.Error.
func commandResultToError(r *core.CommandResult) *report.Error {
	if r == nil || r.Error == nil {
		return nil
	}

	errType := "unknown"
	message := r.Error.Error()

	// Use message from result if available
	if r.Message != "" {
		message = r.Message
	}

	return &report.Error{
		Type:    errType,
		Message: message,
	}
}
