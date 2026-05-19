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
	headless := !cfg.Headed
	printSetupStep("Launching browser...")
	logger.Info("Creating web driver (headless=%v)", headless)

	driver, err := cdpdriver.New(cdpdriver.Config{
		Headless:    headless,
		URL:         cfg.AppID,
		Browser:     cfg.Browser,
		UserDataDir: cfg.UserDataDir,
	})
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
