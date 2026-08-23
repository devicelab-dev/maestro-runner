package cli

import (
	"fmt"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	cdpdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/browser/cdp"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// CreateWebDriver creates a browser driver using Rod + CDP.
// Exported for library use.
func CreateWebDriver(cfg *RunConfig) (core.Driver, func(), error) {
	driverConfig := buildWebDriverConfig(cfg)
	printSetupStep("Launching browser...")
	logger.Info("Creating web driver (headless=%v)", driverConfig.Headless)

	driver, err := cdpdriver.New(driverConfig)
	if err != nil {
		logger.Error("Failed to launch browser: %v", err)
		return nil, nil, fmt.Errorf("launch browser: %w", err)
	}

	printSetupSuccess("Browser launched")
	cleanup := func() {
		if err := driver.Close(); err != nil {
			logger.Debug("failed to close browser driver during cleanup: %v", err)
		}
	}
	return driver, cleanup, nil
}

// buildWebDriverConfig expands the flow header with the runner environment
// before the CDP driver's initial navigation. Expansion also happens once
// centrally when the header is resolved; repeating it here is idempotent and
// keeps this entry point correct for library callers that build a RunConfig
// themselves.
func buildWebDriverConfig(cfg *RunConfig) cdpdriver.Config {
	return cdpdriver.Config{
		Headless:    !cfg.Headed,
		URL:         expandRunnerVars(cfg.AppID, cfg.Env),
		Browser:     cfg.Browser,
		UserDataDir: cfg.UserDataDir,
	}
}
