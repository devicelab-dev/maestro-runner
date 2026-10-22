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
