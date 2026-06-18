// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package flutter

import "testing"

func TestParseIOSVMServiceLog(t *testing.T) {
	output := `2026-03-02 10:00:00.000 Df Runner[1234] The Dart VM service is listening on http://127.0.0.1:54321/xYz789Token/
`
	port, token := parseIOSVMServiceLog(output)
	if port != 54321 {
		t.Errorf("port = %d, want 54321", port)
	}
	if token != "xYz789Token" {
		t.Errorf("token = %q, want %q", token, "xYz789Token")
	}
}

func TestParseIOSVMServiceLog_MultipleRestarts(t *testing.T) {
	output := `2026-03-02 10:00:00.000 Df Runner[1000] The Dart VM service is listening on http://127.0.0.1:11111/oldToken/
2026-03-02 10:00:05.000 Df Runner[2000] The Dart VM service is listening on http://127.0.0.1:22222/newToken/
`
	port, token := parseIOSVMServiceLog(output)
	if port != 22222 {
		t.Errorf("port = %d, want 22222 (most recent)", port)
	}
	if token != "newToken" {
		t.Errorf("token = %q, want %q (most recent)", token, "newToken")
	}
}

func TestParseIOSVMServiceLog_NotFlutterApp(t *testing.T) {
	output := `2026-03-02 10:00:00.000 Df SpringBoard[123] some other log message
2026-03-02 10:00:01.000 Df UIKitApp[456] no flutter here
`
	port, token := parseIOSVMServiceLog(output)
	if port != 0 {
		t.Errorf("port = %d, want 0 for non-Flutter app", port)
	}
	if token != "" {
		t.Errorf("token = %q, want empty for non-Flutter app", token)
	}
}

func TestParseIOSVMServiceLog_Empty(t *testing.T) {
	port, token := parseIOSVMServiceLog("")
	if port != 0 {
		t.Errorf("port = %d, want 0 for empty output", port)
	}
	if token != "" {
		t.Errorf("token = %q, want empty for empty output", token)
	}
}
