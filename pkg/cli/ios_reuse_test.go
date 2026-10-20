package cli

import (
	"testing"

	"github.com/urfave/cli/v2"
)

// --reuse-wda is a WDA option, and --wda-url, which drives a WDA something
// else started, wins over it.
func TestReuseWDAOnlyWithTheWDADriver(t *testing.T) {
	for driver, want := range map[string]bool{"wda": true, "WDA": true, "": false, "devicelab": false, "devicelab-legacy": false, "appium": false} {
		if got := reuseWDA(&RunConfig{ReuseWDA: true, Driver: driver}); got != want {
			t.Errorf("reuseWDA with --driver %q = %v, want %v", driver, got, want)
		}
	}
	if reuseWDA(&RunConfig{Driver: "wda"}) {
		t.Error("reuseWDA without the flag")
	}
	if reuseWDA(&RunConfig{ReuseWDA: true, Driver: "wda", WDAURL: "http://127.0.0.1:8100"}) {
		t.Error("reuseWDA together with --wda-url")
	}
}

func TestGlobalFlags_ReuseWDA(t *testing.T) {
	for _, f := range GlobalFlags {
		if bf, ok := f.(*cli.BoolFlag); ok && bf.Name == "reuse-wda" {
			for _, e := range bf.EnvVars {
				if e == "MAESTRO_REUSE_WDA" {
					return
				}
			}
			t.Fatalf("--reuse-wda has env vars %v, want MAESTRO_REUSE_WDA", bf.EnvVars)
		}
	}
	t.Fatal("no --reuse-wda flag in GlobalFlags")
}

// The flag and its env var reach RunConfig through buildDeviceRunConfig.
func TestBuildDeviceRunConfigReadsReuseWDA(t *testing.T) {
	run := func(t *testing.T, args ...string) bool {
		t.Helper()
		var got bool
		app := &cli.App{Flags: GlobalFlags, Action: func(c *cli.Context) error {
			cfg, err := buildDeviceRunConfig(c)
			if err != nil {
				return err
			}
			got = cfg.ReuseWDA
			return nil
		}}
		if err := app.Run(append([]string{"maestro-runner"}, args...)); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return got
	}
	t.Setenv("MAESTRO_REUSE_WDA", "")
	if run(t) {
		t.Error("ReuseWDA set with neither the flag nor the env var")
	}
	if !run(t, "--reuse-wda") {
		t.Error("--reuse-wda did not set ReuseWDA")
	}
	t.Setenv("MAESTRO_REUSE_WDA", "true")
	if !run(t) {
		t.Error("MAESTRO_REUSE_WDA did not set ReuseWDA")
	}
}
