package cli

import (
	"fmt"
	"strconv"
	"strings"

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
	w, h := parseWindowSize(cfg.WindowSize)
	return cdpdriver.Config{
		Headless:    !cfg.Headed,
		URL:         expandRunnerVars(cfg.AppID, cfg.Env),
		Browser:     cfg.Browser,
		UserDataDir: cfg.UserDataDir,
		ViewportW:   w,
		ViewportH:   h,
	}
}

// parseWindowSize reads a WxH viewport string such as "390x844".
//
// Returns 0, 0 for anything it cannot read, which leaves the driver on its
// 1280x800 default. A malformed value is not worth failing a run over: the
// separator is easy to get wrong ("390*844", "390X844"), and silently running
// at the default beats refusing to start. "X" is accepted alongside "x" for
// that reason. Zero and negative dimensions fall back too, since CDP treats a
// zero-width override as "no override" and would leave the viewport wherever
// the browser happened to open.
func parseWindowSize(s string) (int, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == 'x' || r == 'X' })
	if len(parts) != 2 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}
