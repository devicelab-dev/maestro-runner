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

	// Create server with driver factory
	srv := server.New(func(req server.SessionRequest) (core.Driver, func(), error) {
		platform := strings.ToLower(req.PlatformName)

		cfg := &RunConfig{
			Platform: platform,
			Driver:   req.Driver,
			AppID:    req.AppID,
		}
		if req.DeviceID != "" {
			cfg.Devices = []string{req.DeviceID}
		}

		// Inherit global flags
		cfg.AppFile = getString("app-file")
		cfg.AppiumURL = getString("appium-url")
		cfg.CapsFile = getString("caps")
		cfg.TeamID = getString("team-id")

		switch platform {
		case "android":
			return CreateAndroidDriver(cfg)
		case "ios":
			return CreateIOSDriver(cfg)
		default:
			return nil, nil, fmt.Errorf("unsupported platform: %s", req.PlatformName)
		}
	})

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
