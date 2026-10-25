package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/server"
	"github.com/urfave/cli/v2"
)

var serverCommand = &cli.Command{
	Name:  "server",
	Usage: "Start the REST API server for remote test execution",
	Description: `Start an HTTP server that exposes session-based endpoints for
executing Maestro steps via JSON instead of YAML flow files.

Examples:
  maestro-runner server
  maestro-runner server --port 9999
  maestro-runner --platform android server`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "port",
			Usage:   "Port to listen on",
			Value:   9999,
			EnvVars: []string{"MAESTRO_SERVER_PORT"},
		},
		&cli.BoolFlag{
			Name:  "headed",
			Usage: "Show the browser window for web sessions (default is headless)",
		},
		&cli.StringFlag{
			Name:    "browser",
			Usage:   "Browser for web sessions: chrome, chromium, or path to binary",
			EnvVars: []string{"MAESTRO_BROWSER"},
		},
		&cli.StringFlag{
			Name:    "window-size",
			Aliases: []string{"screen-size"},
			Usage:   "Browser viewport for web sessions as WxH, e.g. 390x844 (default 1280x800)",
			EnvVars: []string{"MAESTRO_WINDOW_SIZE"},
		},
		&cli.StringFlag{
			Name:    "user-data-dir",
			Usage:   "Chrome user-data-dir for a persistent profile across web sessions",
			EnvVars: []string{"MAESTRO_USER_DATA_DIR"},
		},
	},
	Action: runServer,
}

func runServer(c *cli.Context) error {
	// Helper to get flag value from current or parent context
	getString := func(name string) string {
		if c.IsSet(name) {
			return c.String(name)
		}
		if c.Lineage()[1] != nil {
			return c.Lineage()[1].String(name)
		}
		return c.String(name)
	}
	getBool := func(name string) bool {
		if c.IsSet(name) {
			return c.Bool(name)
		}
		if c.Lineage()[1] != nil {
			return c.Lineage()[1].Bool(name)
		}
		return c.Bool(name)
	}

	port := c.Int("port")
	verbose := getBool("verbose")
	_ = verbose

	// Initialize logging
	if err := logger.Init("maestro-server.log"); err != nil {
		fmt.Printf("Warning: Failed to initialize logger: %v\n", err)
	}
	defer logger.Close()

	// Create server with a per-session driver factory. The factory closes over
	// the flag readers so an appium/browser flag on the command line (or in the
	// environment) configures every session it creates.
	srv := server.New(newSessionDriverFactory(getString, getBool))

	// Create HTTP server
	addr := fmt.Sprintf(":%d", port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		fmt.Println("\nShutting down server...")
		srv.ShutdownAll()
		if err := httpServer.Shutdown(context.Background()); err != nil {
			logger.Error("Server shutdown error: %v", err)
		}
	}()

	fmt.Printf("maestro-runner server listening on %s\n", addr)
	fmt.Printf("  POST   /session              - Create a new session\n")
	fmt.Printf("  POST   /session/{id}/execute  - Execute a step\n")
	fmt.Printf("  GET    /session/{id}/screenshot - Take screenshot\n")
	fmt.Printf("  GET    /session/{id}/source    - Get view hierarchy\n")
	fmt.Printf("  GET    /session/{id}/device-info - Get device info\n")
	fmt.Printf("  DELETE /session/{id}           - Delete session\n")
	fmt.Printf("  GET    /status                 - Server status\n")
	fmt.Println()

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// newSessionDriverFactory builds the driver factory the REST server calls for
// every POST /session. getString and getBool read the server command's flags
// (and the inherited global flags), so an operator starts one server with
// --appium-url/--browser once and every session it hands out uses it.
//
// Platform dispatch mirrors cli.CreateDriver: the JSON step protocol reuses the
// same drivers, so a session behaves like a `test` run on that platform.
func newSessionDriverFactory(getString func(string) string, getBool func(string) bool) func(server.SessionRequest) (core.Driver, func(), error) {
	return func(req server.SessionRequest) (core.Driver, func(), error) {
		platform := strings.ToLower(req.PlatformName)

		cfg := &RunConfig{
			Platform: platform,
			Driver:   req.Driver,
			// For web sessions appId is the URL to open — the same field the
			// CLI fills from a flow's `url:` header.
			AppID: req.AppID,
			// Inherit server-level flags.
			AppFile:     getString("app-file"),
			AppiumURL:   getString("appium-url"),
			CapsFile:    getString("caps"),
			TeamID:      getString("team-id"),
			Browser:     getString("browser"),
			Headed:      getBool("headed"),
			WindowSize:  getString("window-size"),
			UserDataDir: getString("user-data-dir"),
		}
		if req.DeviceID != "" {
			cfg.Devices = []string{req.DeviceID}
		}

		switch platform {
		case "android":
			return CreateAndroidDriver(cfg)
		case "ios":
			return CreateIOSDriver(cfg)
		case "web":
			return CreateWebDriver(cfg)
		default:
			return nil, nil, fmt.Errorf("unsupported platform: %s", req.PlatformName)
		}
	}
}
