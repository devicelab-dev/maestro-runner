package devicelab_ios

import "testing"

func TestPromptActionFor(t *testing.T) {
	for _, tc := range []struct {
		perms map[string]string
		want  string
	}{
		{nil, "accept"},
		{map[string]string{"all": "allow"}, "accept"},
		{map[string]string{"all": "deny"}, "dismiss"},
		{map[string]string{"notifications": "deny", "all": "allow"}, "dismiss"},
		{map[string]string{"camera": "allow"}, ""},
		{map[string]string{"notifications": "unset"}, ""},
	} {
		if got := promptActionFor(tc.perms); got != tc.want {
			t.Errorf("promptActionFor(%v) = %q, want %q", tc.perms, got, tc.want)
		}
	}
}

// Only an alert with Apple's "Don't Allow" is a prompt; an app's own "Allow"
// button alone is left alone.
func TestPromptButtons(t *testing.T) {
	prompt := []Node{{Type: "StaticText", Label: "“App” Would Like to Send You Notifications"},
		{Type: "Button", Label: "Don’t Allow"}, {Type: "Button", Label: "Allow"}}
	if allow, deny := promptButtons(prompt); allow == nil || deny == nil || allow.Label != "Allow" {
		t.Fatalf("prompt: allow %v deny %v", allow, deny)
	}
	appAlert := []Node{{Type: "Button", Label: "Allow"}, {Type: "Button", Label: "Cancel"}}
	if _, deny := promptButtons(appAlert); deny != nil {
		t.Fatal("an app's alert was taken for a permission prompt")
	}
}
