package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
	"github.com/devicelab-dev/maestro-runner/pkg/flutter"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// createDevicelabIOSDriver constructs the default iOS driver (--driver devicelab):
// the prebuilt devicelab-ios-agent (drivers/ios/devicelab-ios-agent/) on a
// booted simulator. The agent stays up between runs; a later run re-attaches
// to it instead of starting it again. A physical iPhone goes to
// createDevicelabIOSDeviceDriver.
func createDevicelabIOSDriver(cfg *RunConfig) (core.Driver, func(), error) {
	udid := getFirstDevice(cfg)
	if udid == "" {
		printSetupStep("Finding iOS simulator...")
		var err error
		udid, err = findBootedSimulator()
		if err != nil || udid == "" {
			// No simulator booted: a connected iPhone, as the WDA path does.
			if dev, derr := findConnectedDevice(); derr == nil && dev != "" {
				printSetupSuccess(fmt.Sprintf("Found device: %s", dev))
				return createDevicelabIOSDeviceDriver(cfg, dev)
			}
			return nil, nil, fmt.Errorf("no iOS device found: boot a simulator or connect an iPhone (--driver devicelab, the iOS default)")
		}
		printSetupSuccess(fmt.Sprintf("Found simulator: %s", udid))
	}
	if !isIOSSimulator(udid) {
		return createDevicelabIOSDeviceDriver(cfg, udid)
	}
	if err := waitForSimulatorBoot(udid, simulatorBootWait); err != nil {
		return nil, nil, err
	}

	if cfg.AppFile != "" && !cfg.NoAppInstall {
		printSetupStep(fmt.Sprintf("Installing app: %s", cfg.AppFile))
		if err := installIOSApp(udid, cfg.AppFile, true); err != nil {
			return nil, nil, fmt.Errorf("install app failed: %w", err)
		}
		printSetupSuccess("App installed")
	}

	// Before the agent and the app start, so both launch with them.
	dlios.ApplySimulatorPrefs(udid)

	ctx := context.Background()
	printSetupStep("Starting devicelab iOS agent...")
	agent, client, err := dlios.StartAgent(ctx, dlios.AgentOptions{UDID: udid, ReadyTimeout: 600 * time.Second})
	if err != nil {
		return nil, nil, fmt.Errorf("devicelab iOS agent: %w", err)
	}
	printSetupSuccess(fmt.Sprintf("Agent ready on port %d (%s)", agent.Port(), agent.Mode()))
	release := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		agent.Release(stopCtx, client)
	}

	deviceInfo, err := getIOSDeviceInfo(udid)
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("get device info: %w", err)
	}

	// Screen size in points from the agent's own view of the display.
	screenW, screenH := 0, 0
	if resp, err := client.Call(ctx, "snapshot", &dlios.Args{MaxNodes: 1}); err == nil && resp.Data != nil {
		screenW, screenH = int(resp.Data.ScreenW), int(resp.Data.ScreenH)
	}

	appVersion, appBuild := "", ""
	if cfg.AppID != "" {
		appVersion, appBuild = getIOSAppVersionAndBuild(udid, cfg.AppID)
	}
	if appVersion == "" && appBuild == "" && cfg.AppFile != "" {
		appVersion, appBuild = readBundleVersionAndBuild(cfg.AppFile)
	}

	info := &core.PlatformInfo{
		Platform:     "ios",
		OSVersion:    deviceInfo.OSVersion,
		DeviceName:   deviceInfo.Name,
		DeviceID:     udid,
		IsSimulator:  true,
		ScreenWidth:  screenW,
		ScreenHeight: screenH,
		AppID:        cfg.AppID,
		AppVersion:   appVersion,
		AppBuild:     appBuild,
	}

	drv := dlios.NewDriver(client, info, udid)
	if cfg.AppID != "" {
		drv.SetAppID(cfg.AppID)
	}
	if cfg.TypingFrequency > 0 {
		_ = drv.SetTypingFrequency(cfg.TypingFrequency)
	}
	cleanup := func() {
		drv.Close()
		release()
	}

	// The Flutter VM Service fallback, as on the WDA and legacy paths.
	var driver core.Driver = drv
	if !cfg.NoFlutterFallback {
		fw := flutter.WrapIOS(drv, nil, udid, cfg.AppID)
		driver = fw
		inner := cleanup
		cleanup = func() {
			if fd, ok := fw.(*flutter.FlutterDriver); ok {
				fd.Close()
			}
			inner()
		}
	}
	return driver, cleanup, nil
}

// simulatorBootWait bounds the wait for a simulator that is still booting.
// GitHub's macOS runners took 3-5 minutes to finish a boot.
var simulatorBootWait = 15 * time.Minute

// simctlBootstatus runs `simctl bootstatus` (no -b: a shut-down simulator is
// not booted by it); a variable so tests can stand in for it.
var simctlBootstatus = func(ctx context.Context, udid string) ([]byte, error) {
	return exec.CommandContext(ctx, "xcrun", "simctl", "bootstatus", udid).CombinedOutput()
}

// waitForSimulatorBoot waits for a simulator to finish booting. `simctl boot`
// returns, and simctl lists the simulator as Booted, minutes before the boot
// has finished on a slow machine (GitHub: boot 7-14s, bootstatus another
// 173s); a CI step that booted and moved on handed over a simulator where
// installing the app hung. On a fully booted simulator bootstatus answers at
// once ("already booted"); a shut-down one is left alone, as before.
func waitForSimulatorBoot(udid string, timeout time.Duration) error {
	if state := simulatorState(udid); state != "Booting" && state != "Booted" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	out, err := simctlBootstatus(ctx, udid)
	if ctx.Err() != nil {
		return fmt.Errorf("simulator %s did not finish booting within %s", udid, timeout)
	}
	if err != nil {
		// Not worth failing the run over: the install or the agent start
		// that follows reports a simulator that is really not usable.
		logger.Warn("simctl bootstatus %s: %v: %s", udid, err, strings.TrimSpace(string(out)))
		return nil
	}
	if waited := time.Since(start); waited >= time.Second {
		printSetupSuccess(fmt.Sprintf("Simulator finished booting (waited %s)", waited.Round(time.Second)))
	}
	return nil
}

// simulatorState is the simulator's state as simctl reports it ("Booted",
// "Booting", "Shutdown", ...), or "" when it is not listed.
var simulatorState = func(udid string) string {
	out, err := exec.Command("xcrun", "simctl", "list", "devices", "-j").Output()
	if err != nil {
		return ""
	}
	return parseSimulatorState(out, udid)
}

func parseSimulatorState(listJSON []byte, udid string) string {
	var list struct {
		Devices map[string][]struct {
			UDID  string `json:"udid"`
			State string `json:"state"`
		} `json:"devices"`
	}
	if json.Unmarshal(listJSON, &list) != nil {
		return ""
	}
	for _, devices := range list.Devices {
		for _, d := range devices {
			if d.UDID == udid {
				return d.State
			}
		}
	}
	return ""
}
