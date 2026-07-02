package devicelab_ios

import (
	"context"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Driver implements core.Driver using the devicelab-ios-runner XCUITest
// server. The Driver owns no on-device state; every command carries the
// active app bundle id and the runner side caches XCUIApplication for the
// requested bundle.
type Driver struct {
	client *Client
	info   *core.PlatformInfo
	udid   string
	appID  string

	// Parent context for element-finding operations.
	ctx context.Context

	// Tunable timeouts (ms). 0 means "use the driver default."
	findTimeout         int
	optionalFindTimeout int
	idleTimeout         int
	typingFrequency     int

	// lastTappedIdentifier / lastTappedX / lastTappedY remember the most
	// recent successful tap so handleInputText can pass those coordinates
	// into the runner's type command. This matches agent-device's PR #298
	// fix ("Fix iOS runner fill for tapped secure text fields") — the
	// runner uses textInputAt(x, y) to find the editable element at the
	// tapped point BEFORE falling back to the `hasKeyboardFocus == 1`
	// predicate, which can throw an NSException for SecureTextField on
	// some iOS sim configs. Result: typing into RN password fields stops
	// silently dropping characters.
	lastTappedIdentifier string
	lastTappedX          float64
	lastTappedY          float64
	lastTapHasCoords     bool

	// Snapshot cache. snapshotMatching reuses a recent snapshot when
	// successive selector resolutions happen close together (tapOn →
	// inputText, assertVisible chains). Invalidated on every interaction
	// step so the cache never lags real screen state.
	snapshotCache     []SnapshotNode
	snapshotCacheTime time.Time

	// Runtime — owned by setup.go; the Driver only reads it for orderly
	// shutdown.
	runner *RunnerHandle
}

// NewDriver constructs a Driver. The Client must already point at a running
// runner. The RunnerHandle (returned by Setup) is held so Close can stop
// the underlying xcodebuild process when the flow ends.
func NewDriver(client *Client, info *core.PlatformInfo, udid string, runner *RunnerHandle) *Driver {
	return &Driver{
		client: client,
		info:   info,
		udid:   udid,
		runner: runner,
	}
}

// SetAppID lets the CLI tell the driver which bundle to send on every
// command. Mirrors the appium driver's setup. If not set, commands extract
// the bundle id from the current flow step.
func (d *Driver) SetAppID(bundleID string) { d.appID = bundleID }

// Close ends the runner process. Idempotent.
func (d *Driver) Close() error {
	if d.runner == nil {
		return nil
	}
	err := d.runner.Stop()
	d.runner = nil
	return err
}

// ---------- core.Driver ----------

// Execute dispatches a flow step. The actual handlers live in commands.go.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	return d.executeStep(step)
}

// Screenshot returns inline PNG bytes via host-side simctl capture.
func (d *Driver) Screenshot() ([]byte, error) {
	return d.captureScreenshot()
}

// Hierarchy returns the JSON-encoded snapshot tree. The runner already
// returns flat SnapshotNodes; we serialize them as-is for the reporter to
// consume (it doesn't need the parent-child tree form).
func (d *Driver) Hierarchy() ([]byte, error) {
	return d.hierarchyJSON()
}

// GetState returns minimal current state. We don't track orientation /
// keyboard / clipboard separately on every call; the report includes what
// the runner gave us during the most recent snapshot.
func (d *Driver) GetState() *core.StateSnapshot {
	return &core.StateSnapshot{}
}

// GetPlatformInfo returns the cached PlatformInfo built during setup.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo { return d.info }

// SetFindTimeout sets the default element find timeout in ms.
func (d *Driver) SetFindTimeout(ms int) { d.findTimeout = ms }

// SetOptionalFindTimeout sets the timeout for optional elements (where
// absence is acceptable). Default 2000ms when unset.
func (d *Driver) SetOptionalFindTimeout(ms int) { d.optionalFindTimeout = ms }

// SetWaitForIdleTimeout sets the wait-for-idle timeout in ms. 0 disables.
// Not propagated to the runner today; reserved for future settings command.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	d.idleTimeout = ms
	return nil
}

// SetContext sets the parent context used by element-find polling loops.
func (d *Driver) SetContext(ctx context.Context) { d.ctx = ctx }

// ---------- Optional interfaces ----------

// SetTypingFrequency implements core.TypingFrequencyConfigurer.
func (d *Driver) SetTypingFrequency(freq int) error {
	d.typingFrequency = freq
	return nil
}

// EnsureSession implements core.SessionEnsurer — but the runner has no
// session concept (stateless app binding), so this is a no-op. Kept so the
// runner harness still calls it before flow start, mirroring the WDA path.
func (d *Driver) EnsureSession(appID string) error {
	if appID != "" {
		d.appID = appID
	}
	return nil
}

// ---------- helpers ----------

func (d *Driver) parentContext() context.Context {
	if d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

func (d *Driver) currentBundleID() string { return d.appID } //nolint:unused

// callTimeout returns a context with the find timeout applied (or a sane
// default of 10s if unset).
func (d *Driver) callTimeout() (context.Context, context.CancelFunc) {
	ms := d.findTimeout
	if ms <= 0 {
		ms = 10_000
	}
	return context.WithTimeout(d.parentContext(), time.Duration(ms)*time.Millisecond)
}
