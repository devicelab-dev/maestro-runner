package core

import "strings"

// RedactedBody is what a typing request's body is logged as.
const RedactedBody = `{"text":"<redacted>"}`

// RedactTypedText returns the loggable form of a device-request body: the
// body itself, or RedactedBody when the path is a text-entry endpoint. The
// per-request client logs land in the run's output directory next to the
// report, and a password typed through `${PASSWORD}` was readable there in
// clear. Element values, keys and WDA's type endpoints are the ones that
// carry what the flow typed.
func RedactTypedText(path, body string) string {
	if body == "" {
		return body
	}
	p := strings.ToLower(path)
	switch {
	case strings.HasSuffix(p, "/value"), // WebDriver element send-keys
		strings.HasSuffix(p, "/keys"),     // uiautomator2 + WDA keyboard input
		strings.HasSuffix(p, "/wda/type"), // WDA typeText
		strings.HasSuffix(p, "/actions"):  // W3C actions can carry key sequences
		return RedactedBody
	}
	return body
}
