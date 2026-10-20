package cli

import "testing"

func TestIsNewerRelease(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"1.1.30", "1.1.29", true},
		{"1.1.30", "v1.1.29", true},
		{"1.2.0", "1.1.29", true},
		{"2.0.0", "1.9.9", true},
		{"1.1.10", "1.1.9", true}, // numeric, not string, order
		{"1.1.29", "1.1.29", false},
		{"1.1.28", "1.1.29", false}, // the backwards nag
		{"1.1.9", "1.1.10", false},
		// git describe builds sit after their base tag.
		{"1.1.29", "v1.1.29-22-gf72df97", false},
		{"1.1.29", "v1.1.29-22-gf72df97-dirty", false},
		{"1.1.29", "v1.1.29-dirty", false},
		{"1.1.30", "v1.1.29-22-gf72df97", true},
		// a pre-release comes before its release
		{"1.1.30", "1.1.30-rc.1", true},
		{"1.1.30-rc.1", "1.1.30", false},
		// unreadable on either side: no notice
		{"", "1.1.29", false},
		{"1.1.30", "dev", false},
		{"1.1.30", "f72df97", false},
		{"latest", "1.1.29", false},
	} {
		if got := isNewerRelease(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewerRelease(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestUpdateCheckOff(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set", map[string]string{}, false},
		{"opt-out", map[string]string{"MAESTRO_RUNNER_NO_UPDATE_CHECK": "1"}, true},
		{"opt-out true", map[string]string{"MAESTRO_RUNNER_NO_UPDATE_CHECK": "true"}, true},
		{"opt-out 0", map[string]string{"MAESTRO_RUNNER_NO_UPDATE_CHECK": "0"}, false},
		{"opt-out false", map[string]string{"MAESTRO_RUNNER_NO_UPDATE_CHECK": " FALSE "}, false},
		// CI only quiets the notice; the check itself still runs.
		{"CI", map[string]string{"CI": "true"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok }
			if got := updateCheckOff(lookup); got != tc.want {
				t.Errorf("updateCheckOff(%v) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestEnvOn(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"unset", map[string]string{}, false},
		{"true", map[string]string{"CI": "true"}, true},
		{"empty", map[string]string{"CI": ""}, true},
		{"false", map[string]string{"CI": "false"}, false},
		{"0", map[string]string{"CI": " 0 "}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok }
			if got := envOn(lookup, "CI"); got != tc.want {
				t.Errorf("envOn(%v, CI) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestUpgradeCommand(t *testing.T) {
	const npm = "npm i -g maestro-runner@latest"
	const curl = "curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash"
	for _, tc := range []struct{ exe, want string }{
		{"/usr/local/lib/node_modules/maestro-runner/node_modules/@devicelab/maestro-runner-darwin-arm64/bin/maestro-runner", npm},
		{"/home/ci/.npm/_npx/abc123/node_modules/@devicelab/maestro-runner-linux-x64/bin/maestro-runner", npm},
		{"/work/app/node_modules/@devicelab/maestro-runner-linux-arm64/bin/maestro-runner", npm},
		{"/Users/me/.maestro-runner/bin/maestro-runner", curl},
		{"/work/node_modules/maestro-runner-fork/bin/maestro-runner", curl},
		{"", curl},
	} {
		if got := upgradeCommand(tc.exe); got != tc.want {
			t.Errorf("upgradeCommand(%q) = %q, want %q", tc.exe, got, tc.want)
		}
	}
}
