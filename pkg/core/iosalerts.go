package core

import "strings"

// IOSAlertAction resolves launchApp's permissions into how an iOS driver
// answers a system permission prompt on its own: "accept" for all-allow (no
// permissions at all included, launchApp's default), "dismiss" for all-deny,
// "" for a mixed list, which leaves every prompt to the flow.
//
// Shared by the WDA driver (its defaultAlertAction and autoClickAlertSelector)
// and the DeviceLab iOS driver, so the two answer the same flows the same way.
func IOSAlertAction(permissions map[string]string) string {
	if len(permissions) == 0 {
		return "accept"
	}

	// Check for "all" key
	if val, ok := permissions["all"]; ok && len(permissions) == 1 {
		switch strings.ToLower(val) {
		case "allow":
			return "accept"
		case "deny":
			return "dismiss"
		}
	}

	// Check if all values are the same
	var lastVal string
	for _, v := range permissions {
		lower := strings.ToLower(v)
		if lastVal == "" {
			lastVal = lower
		} else if lastVal != lower {
			return "" // Mixed permissions
		}
	}
	switch lastVal {
	case "allow":
		return "accept"
	case "deny":
		return "dismiss"
	default:
		return ""
	}
}
