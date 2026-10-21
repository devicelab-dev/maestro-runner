package devicelab_ios

import (
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// A system permission prompt the host cannot answer ahead of the launch
// (notifications: simctl privacy has no such service, and a real device has
// no simctl) is answered when it covers the app, as Maestro's own iOS driver
// does: launchApp's permissions say how, all: allow by default, so "Allow".
// Only a prompt is touched: an alert with a "Don't Allow" button, which is
// Apple's wording, never an app's own alert.

// promptActionFor returns how launchApp's permissions answer a prompt:
// "accept", "dismiss", or "" to leave it to the flow.
func promptActionFor(perms map[string]string) string {
	value, ok := perms["notifications"]
	if !ok {
		value, ok = perms["all"]
	}
	if len(perms) == 0 {
		value, ok = "allow", true
	}
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow":
		return "accept"
	case "deny":
		return "dismiss"
	}
	return ""
}

// promptButtons finds a permission prompt's two buttons among the nodes of a
// lookup that included alerts; deny is nil when there is no prompt.
func promptButtons(nodes []Node) (allow, deny *Node) {
	for i := range nodes {
		n := &nodes[i]
		if !strings.Contains(strings.ToLower(n.Type), "button") {
			continue
		}
		label := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(n.Label), "’", "'"))
		switch {
		case strings.HasPrefix(label, "don't allow"):
			deny = n
		case strings.HasPrefix(label, "allow") && allow == nil:
			allow = n
		}
	}
	return allow, deny
}

// answerPermissionPrompt taps the prompt's button when one is on screen and
// reports whether it did.
func (d *Driver) answerPermissionPrompt() bool {
	if d.promptAction == "" {
		return false
	}
	resp, err := d.call("snapshot", &Args{VisibleOnly: true, Alerts: true})
	if err != nil {
		return false
	}
	p := resp.payload()
	allow, deny := promptButtons(p.Nodes)
	if deny == nil {
		return false
	}
	target := allow
	if d.promptAction == "dismiss" {
		target = deny
	}
	if target == nil {
		return false
	}
	x, y, ok := tapPoint(*target, p.ScreenW, p.ScreenH)
	if !ok {
		return false
	}
	if err := d.act("tap", x, y, nil); err != nil {
		logger.Warn("[devicelab-ios] permission prompt: tap %q failed: %v", target.Label, err)
		return false
	}
	logger.Info("[devicelab-ios] permission prompt answered %q (launchApp permissions)", target.Label)
	return true
}
