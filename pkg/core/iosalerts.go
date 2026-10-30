package core

import (
	"sort"
	"strings"
)

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

// IOSPromptButtonAction is the rule both iOS drivers answer a system
// permission prompt by, on an English label: "accept" for a label starting
// with "Allow" ("Allow", "Allow While Using App", "Allow Once", "Allow Full
// Access", App Tracking Transparency's "Allow"), "dismiss" for "Don't Allow"
// (either apostrophe) or "Ask App Not to Track", "" for anything else, so an
// "OK" or "Continue" is never answered.
func IOSPromptButtonAction(label string) string {
	l := strings.ToLower(strings.TrimSpace(label))
	switch {
	case strings.HasPrefix(l, "allow"):
		return "accept"
	case strings.HasPrefix(l, "don") && strings.Contains(l, "allow"), strings.HasPrefix(l, "ask app not"):
		return "dismiss"
	}
	return ""
}

// IOSPromptLabels is how a device in another language words the prompt
// buttons IOSPromptButtonAction knows in English: each localized label
// carries the action of the English label it translates. The translations
// are iOS's own (the prompt string tables of the simulator runtime), not
// guesses, so "Erlauben" accepts and "Nicht erlauben" dismisses on a German
// simulator. The zero value knows English only.
type IOSPromptLabels struct {
	actions map[string]string // normalized localized label -> "accept" / "dismiss"
}

// NewIOSPromptLabels builds the vocabulary from pairs of an English prompt
// string and the same key's value in a preferred language. A pair counts only
// when its English side is a whole button label the English rule answers
// (a prompt title such as "Allow “%@” to use your location?", or a message
// sentence, is not a button).
// A translation that already reads as an English label keeps its English
// meaning, and one that translates labels of both actions answers neither.
func NewIOSPromptLabels(pairs [][2]string) IOSPromptLabels {
	seen := map[string]map[string]bool{}
	for _, p := range pairs {
		english, localized := p[0], normalizePromptLabel(p[1])
		action := IOSPromptButtonAction(english)
		if action == "" || localized == "" || !isButtonLabel(english) ||
			IOSPromptButtonAction(localized) != "" {
			continue
		}
		if seen[localized] == nil {
			seen[localized] = map[string]bool{}
		}
		seen[localized][action] = true
	}
	actions := map[string]string{}
	for label, set := range seen {
		if len(set) != 1 {
			continue
		}
		for action := range set {
			actions[label] = action
		}
	}
	return IOSPromptLabels{actions: actions}
}

// Action is the action a prompt button's label answers: the English rule
// first, then the device language's translations, matched as a whole label.
func (v IOSPromptLabels) Action(label string) string {
	if a := IOSPromptButtonAction(label); a != "" {
		return a
	}
	return v.actions[normalizePromptLabel(label)]
}

// Labels lists the localized labels that answer action, sorted, as they are
// matched (lower case, trimmed).
func (v IOSPromptLabels) Labels(action string) []string {
	var out []string
	for label, a := range v.actions {
		if a == action {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// isButtonLabel tells a prompt's button label from its title or message,
// which can start with "Allow" too: those carry the app's name (%@), ask a
// question or end in a full stop.
func isButtonLabel(s string) bool {
	s = strings.TrimSpace(s)
	return !strings.ContainsAny(s, "%?") && !strings.HasSuffix(s, ".")
}

func normalizePromptLabel(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
