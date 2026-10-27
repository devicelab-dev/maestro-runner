package devicelab_ios

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// tokenAgent is an httptest agent that, like the real one started with a
// token, answers a request without it 401 UNAUTHORIZED.
func tokenAgent(t *testing.T, token string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.Header.Get(TokenHeader) != token {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(Response{OK: false, Error: &ErrorBody{Code: "UNAUTHORIZED", Message: "missing or wrong X-DeviceLab-Token"}})
			return
		}
		_ = json.NewEncoder(w).Encode(Response{OK: true, ID: req.ID, Data: &Payload{Message: "ready"}})
	}))
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)
	return port
}

func TestClientSendsToken(t *testing.T) {
	port := tokenAgent(t, "s3cret")
	c := NewClient(port)
	c.SetToken("s3cret")
	if resp, err := c.Call(context.Background(), "status", nil); err != nil || !resp.OK {
		t.Fatalf("with the token: resp = %+v err = %v", resp, err)
	}
	for _, token := range []string{"", "wrong"} {
		c := NewClient(port)
		c.SetToken(token)
		_, err := c.Call(context.Background(), "status", nil)
		var ae *AgentError
		if !errors.As(err, &ae) || ae.Code != "UNAUTHORIZED" {
			t.Fatalf("token %q: err = %v, want UNAUTHORIZED", token, err)
		}
	}
}

func TestNewTokenIsRandom(t *testing.T) {
	a, b := newToken(), newToken()
	if len(a) != 32 || a == b {
		t.Fatalf("tokens %q %q", a, b)
	}
}

func TestStateKeepsTokenPrivate(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	saveState("SIM-T", agentState{Port: 22123, Version: "v1", Token: "tok"})
	s, ok := loadState("SIM-T")
	if !ok || s.Token != "tok" {
		t.Fatalf("state = %+v", s)
	}
	info, err := os.Stat(filepath.Join(stateDir("SIM-T"), "agent.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("agent.json mode = %v err = %v", info.Mode().Perm(), err)
	}
}

func TestStartAgentReattachesWithSavedToken(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, dir, "1.0")
	port := tokenAgent(t, "kept-token")
	saveState("SIM-TR", agentState{Port: port, Version: "abc123", Token: "kept-token"})
	a, c, err := StartAgent(context.Background(), AgentOptions{UDID: "SIM-TR", Dir: dir})
	if err != nil || a.Mode() != "reattached" || a.token != "kept-token" {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	if _, err := c.Call(context.Background(), "snapshot", &Args{MaxNodes: 1}); err != nil {
		t.Fatalf("re-attached client should send the saved token: %v", err)
	}
}

func TestXctestrunForSetsToken(t *testing.T) {
	if _, err := os.Stat("/usr/bin/plutil"); err != nil {
		t.Skip("plutil not available")
	}
	dir := t.TempDir()
	simXctestrun := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>DevicelabIOSAgentUITests</key><dict>
<key>EnvironmentVariables</key><dict><key>DL_AGENT_PORT</key><string>22087</string></dict>
<key>TestingEnvironmentVariables</key><dict><key>DL_AGENT_PORT</key><string>22087</string></dict>
</dict></dict></plist>`
	if err := os.WriteFile(filepath.Join(dir, "agent.xctestrun"), []byte(simXctestrun), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Agent{opts: AgentOptions{Dir: dir}, token: "abc"}
	path, err := a.xctestrunFor(22346)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	target := decodeXctestrun(t, raw)
	for _, key := range []string{"EnvironmentVariables", "TestingEnvironmentVariables"} {
		if v := target[key].(map[string]interface{})[agentTokenVar]; v != "abc" {
			t.Fatalf("%s token = %v", key, v)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("xctestrun mode = %v", info.Mode().Perm())
	}
}

func TestSetXctestrunEnvForDevice(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.xctestrun"), []byte(sampleXctestrun), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := writeDeviceXctestrun(dir, 22334, agentBundleIDs(""))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(deviceAgentBindEnv, "")
	if env := deviceAgentEnv("tok"); len(env) != 1 || env[agentTokenVar] != "tok" {
		t.Fatalf("env = %v", env)
	}
	t.Setenv(deviceAgentBindEnv, "any")
	if err := setXctestrunEnv(path, deviceAgentEnv("tok")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	target := decodeXctestrun(t, raw)
	for _, key := range []string{"EnvironmentVariables", "TestingEnvironmentVariables"} {
		env := target[key].(map[string]interface{})
		if env[agentTokenVar] != "tok" || env["DL_AGENT_BIND"] != "any" || env["DL_AGENT_PORT"] != "22334" {
			t.Fatalf("%s = %v", key, env)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("xctestrun mode = %v", info.Mode().Perm())
	}
	if err := setXctestrunEnv(filepath.Join(dir, "missing.xctestrun"), nil); err == nil {
		t.Fatal("a missing file should fail")
	}
	bad := filepath.Join(dir, "bad.xctestrun")
	_ = os.WriteFile(bad, []byte("junk"), 0o644)
	if err := setXctestrunEnv(bad, nil); err == nil {
		t.Fatal("a bad plist should fail")
	}
}
