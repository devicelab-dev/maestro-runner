package core

import "strings"

// iosPermissionAction resolves one `name: value` pair from a flow into the
// simctl privacy action and service to run.
//
// Most permissions are a plain three-way allow / deny / unset. Location is not:
// iOS distinguishes "while using the app" from "always", so it takes
// `always`, `inuse`, `never` and `unset`, and the two services behind them are
// different — `location` for in-use, `location-always` for background. Maestro
// accepts the same words, and a flow written against it must behave the same
// here.
//
// Returns ok=false for a value this permission does not accept. Callers report
// that rather than skipping it: silently ignoring `location: never` is what
// made this look like a broken feature instead of a rejected value — the
// permission was reset to "not determined" and the app asked the user, which is
// the exact opposite of what the flow said (#147).
func IOSPrivacyAction(service, value string) (action string, resolvedService string, ok bool) {
	v := strings.ToLower(strings.TrimSpace(value))

	if service == "location-always" || service == "location" {
		switch v {
		case "always", "allow":
			return "grant", "location-always", true
		case "inuse", "wheninuse", "when-in-use":
			return "grant", "location", true
		case "never", "deny":
			return "revoke", "location-always", true
		case "unset":
			return "reset", "location-always", true
		default:
			return "", "", false
		}
	}

	switch v {
	case "allow":
		return "grant", service, true
	case "deny":
		return "revoke", service, true
	case "unset":
		return "reset", service, true
	default:
		return "", "", false
	}
}

// IOSPrivacyServices maps a flow permission name to the iOS privacy service
// names simctl uses. An empty result means iOS offers no host-side control
// over that permission — `notifications` and `faceid` are the two — and the
// caller should say so rather than sending simctl a name it rejects. Shared by the WDA and DeviceLab iOS drivers so the two
// cannot drift apart the way the Android copies did (#148).
func IOSPrivacyServices(shortcut string) []string {
	switch strings.ToLower(shortcut) {
	case "location", "location-always":
		return []string{"location-always"}
	case "camera":
		return []string{"camera"}
	case "contacts":
		return []string{"contacts"}
	case "phone":
		return []string{"contacts"} // iOS doesn't have separate phone permission
	case "microphone":
		return []string{"microphone"}
	case "photos":
		return []string{"photos"}
	case "medialibrary", "media-library":
		// simctl spells this one with a hyphen; "medialibrary" is rejected.
		return []string{"media-library"}
	case "calendar":
		return []string{"calendar"}
	case "reminders":
		return []string{"reminders"}
	case "notifications":
		// simctl privacy has no notifications service — iOS does not expose
		// notification authorisation to the host. Reported by the caller
		// rather than sent to simctl, which rejects it.
		return nil
	case "bluetooth":
		return []string{"bluetooth-peripheral"}
	case "health":
		return []string{"health"}
	case "homekit":
		return []string{"homekit"}
	case "motion":
		return []string{"motion"}
	case "speech":
		return []string{"speech-recognition"}
	case "siri":
		return []string{"siri"}
	case "faceid":
		// Not a simctl privacy service either. Biometric enrolment is
		// controlled through the Simulator UI, not the privacy database.
		return nil
	default:
		// Assume it's already a valid service name
		return []string{shortcut}
	}
}

// hasAllValue checks if all permission values match the given value.
