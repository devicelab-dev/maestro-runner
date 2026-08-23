package executor

import (
	"fmt"
	"strings"

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

	message := r.Error.Error()

	// Use message from result if available — but preserve the underlying cause
	// so debugging can see why findElement actually rejected.
	if r.Message != "" {
		if causeMsg := r.Error.Error(); causeMsg != "" && causeMsg != r.Message {
			message = fmt.Sprintf("%s (cause: %s)", r.Message, causeMsg)
		} else {
			message = r.Message
		}
	}

	errType := classifyError(message)

	return &report.Error{
		Type:    errType,
		Message: message,
	}
}

// enrichErrorWithCDP adds CDP socket context to an error when a DevTools socket is detected.
// This indicates a WebView or browser is present with debugging enabled.
func enrichErrorWithCDP(errInfo *report.Error, cdp *core.CDPInfo) {
	detail := fmt.Sprintf("CDP socket available: %s (WebView or browser detected with DevTools enabled)", cdp.Socket)
	if errInfo.Details != "" {
		errInfo.Details += "\n" + detail
	} else {
		errInfo.Details = detail
	}
}

// enrichErrorWithWebView adds WebView context to an error when a WebView is detected but CDP is not available.
// This tells the user that their element is likely inside a WebView whose content is invisible
// to native UI automation — and explains what they need to do about it.
func enrichErrorWithWebView(errInfo *report.Error, wv *core.WebViewInfo) {
	var detail string
	if wv.Type == "browser" {
		detail = fmt.Sprintf(
			"A browser (%s) is visible on screen. Elements inside the browser are rendered by a web engine "+
				"and are not accessible through native UI automation. "+
				"To interact with web content, Chrome DevTools Protocol (CDP) is needed but is currently not available.",
			wv.PackageName,
		)
	} else {
		detail = fmt.Sprintf(
			"A WebView (%s) is visible on screen in %s. Elements inside the WebView are rendered by a web engine "+
				"and are not accessible through native UI automation. "+
				"To interact with web content, enable Chrome DevTools Protocol (CDP) by adding "+
				"WebView.setWebContentsDebuggingEnabled(true) in the app's code.",
			wv.ClassName, wv.PackageName,
		)
	}
	if errInfo.Details != "" {
		errInfo.Details += "\n" + detail
	} else {
		errInfo.Details = detail
	}
}

// classifyError determines the error type from the message.
// Types: assertion, timeout, element_not_found, app_crash, network, unknown
func classifyError(msg string) string {
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "not found"):
		return "element_not_found"
	case strings.Contains(lower, "keyboard is covering") || strings.Contains(lower, "keyboard is open"):
		return "element_not_found"
	case strings.Contains(lower, "not visible") || strings.Contains(lower, "not displayed"):
		return "assertion"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return "timeout"
	case strings.Contains(lower, "crash") || strings.Contains(lower, "not responding") || strings.Contains(lower, "not installed"):
		return "app_crash"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "refused") || strings.Contains(lower, "unreachable"):
		return "network"
	default:
		return "unknown"
	}
}
