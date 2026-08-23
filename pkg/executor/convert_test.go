package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"Element not found: text='Login'", "element_not_found"},
		{"Element not found within 5000ms", "element_not_found"},
		{"Element found but keyboard is covering it (keyboard top: 1111, element center Y: 1178)", "element_not_found"},
		{"keyboard is open — add a `- hideKeyboard` step", "element_not_found"},
		{"Element not visible after 5000ms", "assertion"},
		{"Text is not displayed", "assertion"},
		{"Operation timed out after 30s", "timeout"},
		{"Connection timed out", "timeout"},
		{"App crashed during launch", "app_crash"},
		{"Application not responding", "app_crash"},
		{"App not installed: com.example", "app_crash"},
		{"Connection refused", "network"},
		{"Host unreachable", "network"},
		{"some other error", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := classifyError(tt.msg)
			if got != tt.want {
				t.Errorf("classifyError(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestCommandResultToErrorClassification(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		if got := commandResultToError(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("nil error", func(t *testing.T) {
		r := &core.CommandResult{Success: true}
		if got := commandResultToError(r); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("element not found", func(t *testing.T) {
		r := &core.CommandResult{
			Error:   fmt.Errorf("element not found"),
			Message: "Element not found: text='Login'",
		}
		got := commandResultToError(r)
		if got == nil {
			t.Fatal("expected non-nil error")
		}
		if got.Type != "element_not_found" {
			t.Errorf("Type = %q, want %q", got.Type, "element_not_found")
		}
	})

	t.Run("keyboard covering", func(t *testing.T) {
		r := &core.CommandResult{
			Error:   fmt.Errorf("keyboard is open"),
			Message: "Element found but keyboard is covering it (keyboard top: 1111, element center Y: 1178)",
		}
		got := commandResultToError(r)
		if got == nil {
			t.Fatal("expected non-nil error")
		}
		if got.Type != "element_not_found" {
			t.Errorf("Type = %q, want %q", got.Type, "element_not_found")
		}
	})

	t.Run("uses message wrapping cause", func(t *testing.T) {
		r := &core.CommandResult{
			Error:   fmt.Errorf("raw error"),
			Message: "Connection refused to device",
		}
		got := commandResultToError(r)
		if got == nil {
			t.Fatal("expected non-nil error")
		}
		if got.Type != "network" {
			t.Errorf("Type = %q, want %q", got.Type, "network")
		}
		want := "Connection refused to device (cause: raw error)"
		if got.Message != want {
			t.Errorf("Message = %q, want %q", got.Message, want)
		}
	})
}

func TestEnrichErrorWithCDP(t *testing.T) {
	t.Run("CDP available", func(t *testing.T) {
		errInfo := &report.Error{Type: "element_not_found", Message: "not found"}
		cdp := &core.CDPInfo{
			Available: true,
			Socket:    "webview_devtools_remote_12345",
		}
		enrichErrorWithCDP(errInfo, cdp)
		if !strings.Contains(errInfo.Details, "CDP socket available: webview_devtools_remote_12345") {
			t.Errorf("expected CDP socket in details, got %q", errInfo.Details)
		}
		if !strings.Contains(errInfo.Details, "WebView or browser detected") {
			t.Errorf("expected WebView mention in details, got %q", errInfo.Details)
		}
	})

	t.Run("appends to existing details", func(t *testing.T) {
		errInfo := &report.Error{Type: "timeout", Message: "timed out", Details: "existing detail"}
		cdp := &core.CDPInfo{Available: true, Socket: "webview_devtools_remote_999"}
		enrichErrorWithCDP(errInfo, cdp)
		if !strings.HasPrefix(errInfo.Details, "existing detail\n") {
			t.Errorf("expected existing details preserved, got %q", errInfo.Details)
		}
	})
}

func TestEnrichErrorWithWebView(t *testing.T) {
	t.Run("webview type", func(t *testing.T) {
		errInfo := &report.Error{Type: "element_not_found", Message: "not found"}
		wv := &core.WebViewInfo{
			Type:        "webview",
			ClassName:   "android.webkit.WebView",
			PackageName: "com.example.app",
		}
		enrichErrorWithWebView(errInfo, wv)
		if !strings.Contains(errInfo.Details, "android.webkit.WebView") {
			t.Errorf("expected WebView class in details, got %q", errInfo.Details)
		}
		if !strings.Contains(errInfo.Details, "com.example.app") {
			t.Errorf("expected package name in details, got %q", errInfo.Details)
		}
		if !strings.Contains(errInfo.Details, "not accessible through native UI automation") {
			t.Errorf("expected explanation of why element can't be found, got %q", errInfo.Details)
		}
		if !strings.Contains(errInfo.Details, "setWebContentsDebuggingEnabled(true)") {
			t.Errorf("expected CDP enablement hint, got %q", errInfo.Details)
		}
	})

	t.Run("browser type", func(t *testing.T) {
		errInfo := &report.Error{Type: "element_not_found", Message: "not found"}
		wv := &core.WebViewInfo{
			Type:        "browser",
			PackageName: "com.android.chrome",
		}
		enrichErrorWithWebView(errInfo, wv)
		if !strings.Contains(errInfo.Details, "com.android.chrome") {
			t.Errorf("expected browser package in details, got %q", errInfo.Details)
		}
		if !strings.Contains(errInfo.Details, "not accessible through native UI automation") {
			t.Errorf("expected explanation of why element can't be found, got %q", errInfo.Details)
		}
		if !strings.Contains(errInfo.Details, "CDP") {
			t.Errorf("expected CDP mention, got %q", errInfo.Details)
		}
	})

	t.Run("appends to existing details", func(t *testing.T) {
		errInfo := &report.Error{Type: "timeout", Message: "timed out", Details: "existing detail"}
		wv := &core.WebViewInfo{Type: "webview", ClassName: "android.webkit.WebView", PackageName: "com.example.app"}
		enrichErrorWithWebView(errInfo, wv)
		if !strings.HasPrefix(errInfo.Details, "existing detail\n") {
			t.Errorf("expected existing details preserved, got %q", errInfo.Details)
		}
	})
}
