package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

func TestIsSecretName(t *testing.T) {
	secret := []string{
		"PASSWORD", "password", "userPassword", "USER_PASSWORD", "PASSWD", "PASS", "DB_PASS",
		"SECRET", "clientSecret", "TOKEN", "authToken", "ACCESS_TOKENS", "APIKEY", "apikey",
		"API_KEY", "apiKey", "APIKey", "KEY", "PIN", "userPIN", "PIN_CODE", "OTP", "OTP_CODE",
		"CREDENTIAL", "awsCredentials", "my-secret", "pass2",
	}
	plain := []string{
		"MONKEY", "KEYBOARD", "PINNED", "SPINNER", "PASSENGER", "COMPASS", "TOKENIZER",
		"USERNAME", "EMAIL", "TEXT", "OPTION", "", "_",
	}
	for _, n := range secret {
		if !isSecretName(n) {
			t.Errorf("isSecretName(%q) = false, want true", n)
		}
	}
	for _, n := range plain {
		if isSecretName(n) {
			t.Errorf("isSecretName(%q) = true, want false", n)
		}
	}
}

func TestMaskSecrets(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariables(map[string]string{
		"PASSWORD": "hunter2",
		"API_KEY":  "hunter2-and-more", // contains PASSWORD's value: masked whole
		"PIN":      "42",               // too short to mask
		"USERNAME": "alice",            // not a secret
		"OTP":      "",                 // empty
	})

	tests := []struct{ in, want string }{
		{"Entered text: hunter2", "Entered text: ***"},
		{"Entered text: xhunter2x", "Entered text: x***x"},
		{"Entered text: hunter2-and-more", "Entered text: ***"},
		{"Entered text: alice 42", "Entered text: alice 42"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := se.MaskSecrets(tt.in); got != tt.want {
			t.Errorf("MaskSecrets(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMaskTypedResult(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("PASSWORD", "hunter2")
	fr := &FlowRunner{script: se}

	cause := errors.New("server said hunter2 was bad")
	res := fr.maskTypedResult(&flow.InputTextStep{}, &core.CommandResult{
		Success: false, Error: cause, Message: "Failed to type hunter2",
	})
	if res.Message != "Failed to type ***" {
		t.Errorf("Message = %q", res.Message)
	}
	if res.Error.Error() != "server said *** was bad" {
		t.Errorf("Error = %q", res.Error.Error())
	}
	if !errors.Is(res.Error, cause) {
		t.Error("masked error no longer wraps the driver's error")
	}

	// Only typing steps are masked.
	res = fr.maskTypedResult(&flow.TapOnStep{}, &core.CommandResult{Message: "hunter2"})
	if res.Message != "hunter2" {
		t.Errorf("tapOn message = %q, want it untouched", res.Message)
	}
}

// End to end: a password typed with inputText, from the flow's env: and from
// -e, top level and inside a runFlow, reaches the driver as it is and none of
// the reports, the console callbacks or the step names.
func TestRunner_MasksTypedSecrets(t *testing.T) {
	tmpDir := t.TempDir()
	const flowPW, extToken, badPW, shortPin = "flowpw777", "exttok999", "failpw888", "12"

	var mu sync.Mutex
	var typed, console []string
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			s, ok := step.(*flow.InputTextStep)
			if !ok {
				return &core.CommandResult{Success: true}
			}
			mu.Lock()
			typed = append(typed, s.Text)
			mu.Unlock()
			if strings.Contains(s.Text, badPW) {
				return &core.CommandResult{
					Success: false,
					Error:   fmt.Errorf("field rejected %s", s.Text),
					Message: "Failed to input text: " + s.Text,
				}
			}
			return &core.CommandResult{Success: true, Message: "Entered text: " + s.Text}
		},
	}
	record := func(desc, errMsg string) {
		mu.Lock()
		console = append(console, desc+" | "+errMsg)
		mu.Unlock()
	}
	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Artifacts:     ArtifactNever,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
		Env:           map[string]string{"EXT_TOKEN": extToken},
		OnStepComplete: func(_ int, desc string, _ bool, _ int64, errMsg string) {
			record(desc, errMsg)
		},
		OnNestedStep: func(_ int, desc string, _ bool, _ int64, errMsg string) {
			record(desc, errMsg)
		},
	})

	input := func(raw string) *flow.InputTextStep {
		return &flow.InputTextStep{BaseStep: flow.BaseStep{StepType: flow.StepInputText}, Text: raw, RawText: raw}
	}
	flows := []flow.Flow{{
		SourcePath: "login.yaml",
		Config: flow.Config{Name: "login", Env: map[string]string{
			"PASSWORD": flowPW, "BAD_PASSWORD": badPW, "PIN": shortPin,
		}},
		Steps: []flow.Step{
			input("${PASSWORD}"),
			input("${EXT_TOKEN}"),
			input("${PIN}"),
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Steps:    []flow.Step{input("${PASSWORD}"), input("${BAD_PASSWORD}")},
			},
		},
	}}

	if _, err := runner.Run(context.Background(), flows); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{flowPW, extToken, shortPin, flowPW, badPW}
	if strings.Join(typed, ",") != strings.Join(want, ",") {
		t.Errorf("driver typed %v, want %v", typed, want)
	}

	for _, line := range console {
		for _, s := range []string{flowPW, extToken, badPW} {
			if strings.Contains(line, s) {
				t.Errorf("console line %q shows a secret", line)
			}
		}
	}
	joined := strings.Join(console, "\n")
	if !strings.Contains(joined, "***") {
		t.Errorf("console never shows the mask:\n%s", joined)
	}
	if !strings.Contains(joined, shortPin) {
		t.Errorf("a value shorter than 3 characters should not be masked:\n%s", joined)
	}

	found := 0
	err := filepath.Walk(tmpDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		found++
		for _, s := range []string{flowPW, extToken, badPW} {
			if strings.Contains(string(b), s) {
				t.Errorf("%s contains secret %q: %s", filepath.Base(p), s, grepLine(string(b), s))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("no report files written")
	}
	b, err := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "***") {
		t.Error("report.json should carry the masked failure message")
	}
}

func grepLine(b, s string) string {
	for _, l := range strings.Split(b, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}
