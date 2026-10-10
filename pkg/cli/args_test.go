package cli

import (
	"reflect"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestNormalizeArgs(t *testing.T) {
	cmds := []string{"maestro-runner"}
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "options after the flow path",
			in:   []string{"test", "flow.yaml", "--format", "junit", "-e", "APP_ID=x", "--flatten"},
			want: []string{"test", "--format", "junit", "-e", "APP_ID=x", "--flatten", "flow.yaml"},
		},
		{
			name: "options between flow paths",
			in:   []string{"test", "a.yaml", "--output", "out", "b.yaml"},
			want: []string{"test", "--output", "out", "a.yaml", "b.yaml"},
		},
		{
			name: "global option after test moves before it",
			in:   []string{"test", "--device", "emu-1", "--no-reinstall-driver", "flow.yaml"},
			want: []string{"--device", "emu-1", "test", "--no-reinstall-driver", "flow.yaml"},
		},
		{
			name: "global options before test stay",
			in:   []string{"--platform", "android", "test", "flow.yaml", "--output=out"},
			want: []string{"--platform", "android", "test", "--output=out", "flow.yaml"},
		},
		{
			name: "root run without the test command",
			in:   []string{"flow.yaml", "-e", "A=1"},
			want: []string{"-e", "A=1", "flow.yaml"},
		},
		{
			name: "already in order",
			in:   []string{"--platform", "ios", "test", "-e", "A=1", "flows/"},
			want: []string{"--platform", "ios", "test", "-e", "A=1", "flows/"},
		},
		{
			name: "after -- nothing moves",
			in:   []string{"test", "flow.yaml", "--", "--not-a-flag"},
			want: []string{"test", "flow.yaml", "--", "--not-a-flag"},
		},
		{
			name: "other commands are left alone",
			in:   []string{"hierarchy", "--depth", "5"},
			want: []string{"hierarchy", "--depth", "5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeArgs(append(cmds, tt.in...), GlobalFlags, testCommand.Flags,
				[]*cli.Command{testCommand, hierarchyCommand})
			if !reflect.DeepEqual(got[1:], tt.want) {
				t.Errorf("normalizeArgs(%q)\n got %q\nwant %q", tt.in, got[1:], tt.want)
			}
		})
	}
}

func TestResolveMaestroOutput(t *testing.T) {
	tests := []struct {
		format, output, testDir, debugDir string
		wantDir, wantFile                 string
		wantErr                           bool
	}{
		{"", "out", "", "", "out", "", false},
		{"", "", "", "", "", "", false},
		{"junit", "/tmp/r.xml", "", "", "", "/tmp/r.xml", false},
		{"JUNIT", "", "", "", "", "report.xml", false},
		{"html", "", "art", "", "art", "report.html", false},
		{"junit", "r.xml", "", "dbg", "dbg", "r.xml", false},
		{"noop", "", "art", "", "art", "", false},
		{"xml", "", "", "", "", "", true},
	}
	for _, tt := range tests {
		dir, file, err := resolveMaestroOutput(tt.format, tt.output, tt.testDir, tt.debugDir)
		if (err != nil) != tt.wantErr || dir != tt.wantDir || file != tt.wantFile {
			t.Errorf("resolveMaestroOutput(%q, %q, %q, %q) = %q, %q, %v; want %q, %q, err=%v",
				tt.format, tt.output, tt.testDir, tt.debugDir, dir, file, err, tt.wantDir, tt.wantFile, tt.wantErr)
		}
	}
}

// Maestro takes the platform from --device; Expo's harness relies on it.
func TestInferPlatformFromDevice(t *testing.T) {
	isIOS := isIOSDeviceID
	t.Cleanup(func() { isIOSDeviceID = isIOS })
	isIOSDeviceID = func(id string) bool { return id == "4E3A6BB3-F417-4829-8DF5-0EA652541F40" }

	tests := []struct {
		platform, device, want string
	}{
		{"", "4E3A6BB3-F417-4829-8DF5-0EA652541F40", "ios"},
		{"", "emulator-5554", ""},
		{"android", "4E3A6BB3-F417-4829-8DF5-0EA652541F40", "android"},
		{"", "", ""},
	}
	for _, tt := range tests {
		cfg := &RunConfig{Platform: tt.platform}
		if tt.device != "" {
			cfg.Devices = []string{tt.device}
		}
		inferPlatformFromDevice(cfg)
		if cfg.Platform != tt.want {
			t.Errorf("platform %q, device %q: got %q, want %q", tt.platform, tt.device, cfg.Platform, tt.want)
		}
	}
	if isIOS("emulator-5554") {
		t.Error("an Android emulator id is not an iOS device")
	}
}
