package flutter

import (
	"fmt"
	"testing"
)

type mockDevice struct {
	shellOutput         string
	shellErr            error
	forwardSocketCalled bool
	forwardSocketPath   string
	forwardSocketRemote int
	forwardSocketErr    error
}

func (m *mockDevice) Shell(cmd string) (string, error) {
	return m.shellOutput, m.shellErr
}

func (m *mockDevice) ForwardSocket(socketPath string, remotePort int) error {
	m.forwardSocketCalled = true
	m.forwardSocketPath = socketPath
	m.forwardSocketRemote = remotePort
	return m.forwardSocketErr
}

func TestDiscoverVMService(t *testing.T) {
	dev := &mockDevice{
		shellOutput: `--------- beginning of main
I/flutter ( 1234): Observatory listening on http://127.0.0.1:12345/abc123/
D/flutter ( 1234): Some debug log
I/flutter ( 1234): The Dart VM service is listening on http://127.0.0.1:54321/xYz789Token/
`,
	}

	token, err := DiscoverVMService(dev, "com.example.app", "/tmp/test-flutter.sock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "xYz789Token" {
		t.Errorf("token = %q, want %q", token, "xYz789Token")
	}
	if !dev.forwardSocketCalled {
		t.Error("ForwardSocket was not called")
	}
	if dev.forwardSocketPath != "/tmp/test-flutter.sock" {
		t.Errorf("socketPath = %q, want %q", dev.forwardSocketPath, "/tmp/test-flutter.sock")
	}
	if dev.forwardSocketRemote != 54321 {
		t.Errorf("remotePort = %d, want 54321", dev.forwardSocketRemote)
	}
}

func TestDiscoverVMService_MultipleRestarts(t *testing.T) {
	dev := &mockDevice{
		shellOutput: `I/flutter ( 1000): The Dart VM service is listening on http://127.0.0.1:11111/oldToken/
I/flutter ( 2000): The Dart VM service is listening on http://127.0.0.1:22222/newToken/
`,
	}

	token, err := DiscoverVMService(dev, "com.example.app", "/tmp/test-flutter.sock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use the most recent (last) token
	if token != "newToken" {
		t.Errorf("token = %q, want most recent token %q", token, "newToken")
	}
	if dev.forwardSocketRemote != 22222 {
		t.Errorf("remotePort = %d, want 22222", dev.forwardSocketRemote)
	}
}

func TestDiscoverVMService_NotFlutterApp(t *testing.T) {
	dev := &mockDevice{
		shellOutput: `--------- beginning of main
D/SomeTag ( 1234): No flutter here
`,
	}

	token, err := DiscoverVMService(dev, "com.example.nativeapp", "/tmp/test-flutter.sock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty for non-Flutter app", token)
	}
	if dev.forwardSocketCalled {
		t.Error("ForwardSocket should not be called for non-Flutter app")
	}
}

func TestDiscoverVMService_EmptyLogcat(t *testing.T) {
	dev := &mockDevice{shellOutput: ""}

	token, err := DiscoverVMService(dev, "com.example.app", "/tmp/test-flutter.sock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
}

func TestDiscoverVMService_ShellError(t *testing.T) {
	dev := &mockDevice{shellErr: fmt.Errorf("adb error")}

	_, err := DiscoverVMService(dev, "com.example.app", "/tmp/test-flutter.sock")
	if err == nil {
		t.Error("expected error when shell fails")
	}
}

func TestDiscoverVMService_ForwardError(t *testing.T) {
	dev := &mockDevice{
		shellOutput: `I/flutter ( 1234): The Dart VM service is listening on http://127.0.0.1:54321/abc123/
`,
		forwardSocketErr: fmt.Errorf("forward failed"),
	}

	_, err := DiscoverVMService(dev, "com.example.app", "/tmp/test-flutter.sock")
	if err == nil {
		t.Error("expected error when forward fails")
	}
}
