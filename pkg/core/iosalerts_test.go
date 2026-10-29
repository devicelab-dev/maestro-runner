package core

import "testing"

func TestIOSAlertAction(t *testing.T) {
	for _, tc := range []struct {
		perms map[string]string
		want  string
	}{
		{nil, "accept"},
		{map[string]string{}, "accept"},
		{map[string]string{"all": "allow"}, "accept"},
		{map[string]string{"all": "Deny"}, "dismiss"},
		{map[string]string{"all": "unset"}, ""},
		{map[string]string{"camera": "allow", "location": "allow"}, "accept"},
		{map[string]string{"camera": "deny", "location": "deny"}, "dismiss"},
		{map[string]string{"camera": "allow", "location": "deny"}, ""},
		{map[string]string{"all": "allow", "camera": "deny"}, ""},
	} {
		if got := IOSAlertAction(tc.perms); got != tc.want {
			t.Errorf("IOSAlertAction(%v) = %q, want %q", tc.perms, got, tc.want)
		}
	}
}

func TestIOSPromptButtonAction(t *testing.T) {
	for label, want := range map[string]string{
		"Allow": "accept", "Allow While Using App": "accept", "allow once": "accept",
		"Don’t Allow": "dismiss", "Don't Allow": "dismiss", "Ask App Not to Track": "dismiss",
		"OK": "", "Continue": "", "Cancel": "", "Always Allow": "",
	} {
		if got := IOSPromptButtonAction(label); got != want {
			t.Errorf("IOSPromptButtonAction(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestNewIOSPromptLabels(t *testing.T) {
	v := NewIOSPromptLabels([][2]string{
		{"Allow", "Erlauben"},
		{"Don’t Allow", "Nicht erlauben"},
		{"Allow While Using App", "Beim Verwenden der App erlauben"},
		// A title, not a button.
		{"Allow “%@” to use your location?", "Darf „%@“ deinen Standort verwenden?"},
		// A message sentence, not a button.
		{"Allow this app to show a badge.", "Erlaube dieser App, ein Kennzeichen anzuzeigen."},
		// Not a label the English rule answers.
		{"OK", "Okay"},
		{"Continue", "Fortfahren"},
		// One translation claimed by both actions answers neither.
		{"Allow Once", "Zweideutig"},
		{"Don’t Allow", "Zweideutig"},
		// A translation that reads as an English label keeps its English meaning.
		{"Don’t Allow", "Allow"},
	})
	for label, want := range map[string]string{
		"Erlauben": "accept", " erlauben ": "accept", "Nicht erlauben": "dismiss",
		"Beim Verwenden der App erlauben":      "accept",
		"Darf „%@“ deinen Standort verwenden?": "", "Okay": "", "Fortfahren": "",
		"Zweideutig": "", "Erlaube dieser App, ein Kennzeichen anzuzeigen.": "", "Allow": "accept", "Allow Later": "accept", "Später erlauben": "",
	} {
		if got := v.Action(label); got != want {
			t.Errorf("Action(%q) = %q, want %q", label, got, want)
		}
	}
	if got := v.Labels("dismiss"); len(got) != 1 || got[0] != "nicht erlauben" {
		t.Errorf("Labels(dismiss) = %q", got)
	}
	if got := (IOSPromptLabels{}).Action("Erlauben"); got != "" {
		t.Errorf("zero value knows only English, got %q", got)
	}
}
