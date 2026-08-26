package devicelab_ios

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// handleSetPermissions applies permissions mid-flow on a simulator.
//
// The step was parsed and then never dispatched here, so a flow using it
// aborted before reaching the device (#148). Simulators take permissions
// through `simctl privacy`, the same mechanism the WDA driver uses; a real
// device has no equivalent, and saying so beats a silent no-op.
func (d *Driver) handleSetPermissions(step *flow.SetPermissionsStep) *core.CommandResult {
	if d.info == nil || !d.info.IsSimulator {
		return &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("unsupported on a real device"),
			Message: "setPermissions needs a simulator — on a real iOS device, permission dialogs must be handled in the flow",
		}
	}
	if d.udid == "" {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("no udid"), Message: "setPermissions: no simulator udid"}
	}

	appID := step.AppID
	if appID == "" {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("no appId"), Message: "setPermissions needs an appId"}
	}
	if len(step.Permissions) == 0 {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("no permissions"), Message: "setPermissions needs at least one permission"}
	}

	var applied int
	var failures []string
	for name, value := range step.Permissions {
		services := core.IOSPrivacyServices(name)
		if len(services) == 0 {
			// iOS exposes no host-side control over this one. Saying so is
			// the honest answer; failing the step would block a flow over
			// something no driver can deliver.
			logger.Warn("setPermissions: iOS has no host-side control over %q — skipping", name)
			continue
		}
		for _, service := range services {
			action, resolved, ok := core.IOSPrivacyAction(service, value)
			if !ok {
				logger.Warn("setPermissions: ignoring unsupported value %q for permission %q", value, name)
				continue
			}
			cmd := exec.Command("xcrun", "simctl", "privacy", d.udid, action, resolved, appID)
			if out, err := cmd.CombinedOutput(); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s", resolved, strings.TrimSpace(string(out))))
				continue
			}
			applied++
		}
	}

	if len(failures) > 0 {
		return &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("some permissions failed"),
			Message: fmt.Sprintf("Permissions: %d applied, failures: %s", applied, strings.Join(failures, "; ")),
		}
	}
	return &core.CommandResult{Success: true, Message: fmt.Sprintf("Permissions updated: %d", applied)}
}
