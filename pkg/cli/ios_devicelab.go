package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	dliosdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// createDevicelabIOSDriver constructs an iOS driver backed by the
// devicelab-ios-runner XCUITest server. Phase 4 supports simulator only.
//
// Lifecycle:
//  1. Resolve a booted iOS simulator UDID
//  2. Optionally install the AUT app (cfg.AppFile)
//  3. Resolve the runner artifacts directory (env var or dev default)
//  4. Launch the runner via devicelab_ios.Setup
//  5. Query the runner for screen size to populate PlatformInfo
//  6. Build and return the Driver + cleanup
func createDevicelabIOSDriver(cfg *RunConfig) (core.Driver, func(), error) {
	udid := getFirstDevice(cfg)
	if udid == "" {
		printSetupStep("Finding iOS simulator...")
		var err error
		udid, err = findBootedSimulator()
		if err != nil || udid == "" {
			return nil, nil, fmt.Errorf("devicelab iOS driver requires a booted simulator (Phase 4 does not support real devices yet)")
		}
		printSetupSuccess(fmt.Sprintf("Found simulator: %s", udid))
	}

	if !isIOSSimulator(udid) {
		return nil, nil, fmt.Errorf("devicelab iOS driver only supports simulators in Phase 4; %s appears to be a physical device", udid)
	}

	if cfg.AppFile != "" && !cfg.NoAppInstall {
		printSetupStep(fmt.Sprintf("Installing app: %s", cfg.AppFile))
		if err := installIOSApp(udid, cfg.AppFile, true); err != nil {
			return nil, nil, fmt.Errorf("install app failed: %w", err)
		}
		printSetupSuccess("App installed")
	}

	// $DEVICELAB_IOS_RUNNER_ARTIFACTS_DIR overrides the bundled-source path
	// for local development (point at a derived-data dir built manually).
	// Otherwise: build from the vendored source on first run, cache for
	// subsequent runs (same UX as the WDA driver).
	ctx := context.Background()
	artifactsDir := os.Getenv("DEVICELAB_IOS_RUNNER_ARTIFACTS_DIR")
	if artifactsDir == "" {
		var err error
		printSetupStep("Resolving devicelab iOS runner build...")
		artifactsDir, err = dliosdriver.EnsureBuilt(ctx, udid)
		if err != nil {
			return nil, nil, fmt.Errorf("devicelab iOS runner: %w", err)
		}
		printSetupSuccess(fmt.Sprintf("Runner build: %s", filepath.Base(artifactsDir)))
	} else if _, err := os.Stat(filepath.Join(artifactsDir, "Build/Products")); err != nil {
		return nil, nil, fmt.Errorf("DEVICELAB_IOS_RUNNER_ARTIFACTS_DIR points at %s but no Build/Products dir is present", artifactsDir)
	}

	printSetupStep("Starting devicelab iOS runner...")
	logger.Info("Launching devicelab-ios-runner from %s on simulator %s", artifactsDir, udid)
	logger.Info("Runner log: %s", filepath.Join(artifactsDir, "logs", "runner.log"))

	client, runner, err := dliosdriver.Setup(ctx, dliosdriver.SetupOptions{
		ArtifactsDir:  artifactsDir,
		SimulatorUDID: udid,
		// CI macos-latest can take several minutes to spin up an XCTest
		// runner via xcodebuild test-without-building (we've seen >250s
		// on successful startups under load). 600s matches upstream
		// maestro's MAESTRO_DRIVER_STARTUP_TIMEOUT default — sized for
		// the slowest CI path. Local runs answer the uptime ping well
		// before the timeout so they pay nothing for the larger budget.
		ReadyTimeout: 600 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("runner setup failed: %w", err)
	}
	printSetupSuccess(fmt.Sprintf("Runner port: %d", runner.Port()))

	deviceInfo, err := getIOSDeviceInfo(udid)
	if err != nil {
		_ = dliosdriver.GracefulShutdown(ctx, client, runner)
		return nil, nil, fmt.Errorf("get device info: %w", err)
	}

	// Query the interaction frame from the runner — that gives the
	// reference width/height of the host's canvas (no appBundleId, so the
	// runner falls back to its own host app rather than trying to
	// activate cfg.AppID which may be an unexpanded `${APP_ID}` template).
	// Best-effort; PlatformInfo can ship with zeros if this fails.
	var screenW, screenH int
	if data, err := client.Call(ctx, dliosdriver.Command{
		Command: dliosdriver.CmdInteractionFrame,
	}); err == nil && data != nil {
		if data.ReferenceWidth != nil {
			screenW = int(*data.ReferenceWidth)
		}
		if data.ReferenceHeight != nil {
			screenH = int(*data.ReferenceHeight)
		}
	}

	platformInfo := &core.PlatformInfo{
		Platform:     "ios",
		OSVersion:    deviceInfo.OSVersion,
		DeviceName:   deviceInfo.Name,
		DeviceID:     udid,
		IsSimulator:  true,
		ScreenWidth:  screenW,
		ScreenHeight: screenH,
		AppID:        cfg.AppID,
	}

	drv := dliosdriver.NewDriver(client, platformInfo, udid, runner)
	if cfg.AppID != "" {
		drv.SetAppID(cfg.AppID)
	}
	if cfg.TypingFrequency > 0 {
		_ = drv.SetTypingFrequency(cfg.TypingFrequency)
	}

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = dliosdriver.GracefulShutdown(shutdownCtx, client, runner)
	}

	// Silence unused-import warnings if logger doesn't appear elsewhere.
	_ = strings.ToLower
	return drv, cleanup, nil
}
