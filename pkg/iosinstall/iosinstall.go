// Package iosinstall installs and uninstalls apps on a physical iOS device.
//
// Both operations prefer `xcrun devicectl` (Apple's CoreDevice tooling, macOS
// 14 with Xcode 15 or later) and fall back to go-ios over usbmuxd when
// devicectl is missing or fails: the zip conduit installs and the installation
// proxy uninstalls. Both are lockdown services, so neither needs the iOS 17+
// tunnel. MAESTRO_RUNNER_IOS_INSTALLER=zipconduit skips devicectl, and
// MAESTRO_RUNNER_IOS_INSTALLER=devicectl returns devicectl's error instead of
// falling back. With devicectl not installed at all, go-ios is used whatever
// the setting, with a warning when devicectl was asked for.
//
// Every devicectl call, and every go-ios install or uninstall once connected,
// is bounded by Timeout. go-ios takes no context, so a go-ios call that times
// out has its connection closed, which ends the blocked call instead of
// leaving it holding a usbmuxd connection for the rest of the run.
package iosinstall

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/zipconduit"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Path names the tool an operation went through.
type Path string

const (
	Devicectl Path = "devicectl"
	GoIOS     Path = "go-ios"
)

// Timeout bounds each devicectl or go-ios call.
const Timeout = 3 * time.Minute

// ErrTimeout is wrapped by the error of a call that ran out of time.
var ErrTimeout = errors.New("timed out")

// installerEnv picks the installer: unset tries devicectl then go-ios.
const installerEnv = "MAESTRO_RUNNER_IOS_INSTALLER"

// These are variables so tests can replace them.
var (
	devicectlAvailable = hasDevicectl
	runDevicectl       = func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "xcrun", append([]string{"devicectl"}, args...)...).CombinedOutput()
	}
	goiosInstall   = installWithGoIOS
	goiosUninstall = uninstallWithGoIOS
	timeout        = Timeout
)

// Install installs the app at appPath on the device and returns the path it
// took.
func Install(udid, appPath string) (Path, error) {
	return run("install",
		func() error {
			return devicectl("install", "device", "install", "app", "--device", udid, appPath)
		},
		func() error { return goiosInstall(udid, appPath) })
}

// Uninstall removes the app bundleID from the device and returns the path it
// took.
func Uninstall(udid, bundleID string) (Path, error) {
	return run("uninstall",
		func() error {
			return devicectl("uninstall", "device", "uninstall", "app", "--device", udid, bundleID)
		},
		func() error { return goiosUninstall(udid, bundleID) })
}

func run(op string, viaDevicectl, viaGoIOS func() error) (Path, error) {
	installer := strings.ToLower(strings.TrimSpace(os.Getenv(installerEnv)))
	var devicectlErr error
	if installer != "zipconduit" {
		if devicectlAvailable() {
			devicectlErr = viaDevicectl()
			if devicectlErr == nil {
				return Devicectl, nil
			}
			if installer == "devicectl" {
				// Forced: report devicectl's own error rather than falling back.
				return Devicectl, devicectlErr
			}
			logger.Warn("devicectl %s failed, falling back to go-ios: %v", op, devicectlErr)
		} else if installer == "devicectl" {
			logger.Warn("%s=devicectl, but xcrun devicectl is not available here; using go-ios", installerEnv)
		}
	}
	if err := viaGoIOS(); err != nil {
		if devicectlErr != nil {
			return GoIOS, fmt.Errorf("go-ios %s failed: %w (after devicectl: %v)", op, err, devicectlErr)
		}
		return GoIOS, fmt.Errorf("go-ios %s failed: %w", op, err)
	}
	return GoIOS, nil
}

func devicectl(op string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := runDevicectl(ctx, args...)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("devicectl %s %w after %v", op, ErrTimeout, timeout)
	}
	if err != nil {
		return fmt.Errorf("devicectl %s failed: %w: %s", op, err, strings.TrimSpace(string(out)))
	}
	return nil
}

var (
	devicectlOnce sync.Once
	devicectlOK   bool
)

// hasDevicectl reports, once per process, whether `xcrun devicectl` works on
// this host.
func hasDevicectl() bool {
	devicectlOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		devicectlOK = exec.CommandContext(ctx, "xcrun", "devicectl", "--version").Run() == nil
	})
	return devicectlOK
}

func installWithGoIOS(udid, appPath string) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("device %s not found: %w", udid, err)
	}
	conn, err := zipconduit.New(entry)
	if err != nil {
		return fmt.Errorf("connect to the install service: %w", err)
	}
	return runClosing(func() error { return conn.SendFile(appPath) }, func() { _ = conn.Close() })
}

func uninstallWithGoIOS(udid, bundleID string) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("device %s not found: %w", udid, err)
	}
	conn, err := installationproxy.New(entry)
	if err != nil {
		return fmt.Errorf("connect to the installation proxy: %w", err)
	}
	return runClosing(func() error { return conn.Uninstall(bundleID) }, conn.Close)
}

// runClosing runs call on a go-ios connection under withTimeout and closes the
// connection exactly once, when call returns or when it times out.
func runClosing(call func() error, closeConn func()) error {
	var once sync.Once
	closeOnce := func() { once.Do(closeConn) }
	defer closeOnce()
	return withTimeout(call, closeOnce)
}

// withTimeout runs f and gives up after timeout, calling abort, which closes
// the connection f is blocked on so its goroutine ends.
func withTimeout(f func() error, abort func()) error {
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		abort()
		return fmt.Errorf("%w after %v", ErrTimeout, timeout)
	}
}
