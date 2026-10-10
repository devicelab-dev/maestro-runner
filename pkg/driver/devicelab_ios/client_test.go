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
	"sync/atomic"
	"testing"
	"time"
)

// agentServer is an httptest agent: it answers every request through handle.
func agentServer(t *testing.T, handle func(cmd string, args Args) Response) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID   string `json:"id"`
			Cmd  string `json:"cmd"`
			Args Args   `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Header.Get("Idempotency-Key") != req.ID {
			http.Error(w, "missing idempotency key", http.StatusBadRequest)
			return
		}
		resp := handle(req.Cmd, req.Args)
		resp.ID = req.ID
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)
	return srv, port
}

func TestClientCall(t *testing.T) {
	_, port := agentServer(t, func(cmd string, a Args) Response {
		if cmd == "find" && strings.Join(a.Needles, ",") == "sign,in" {
			return Response{OK: true, SnapshotID: 7, ServerMs: 12, Phases: map[string]float64{"snapshotMs": 10},
				Data: &Payload{Nodes: []Node{{I: 1, Label: "Sign In"}}}}
		}
		return Response{OK: false, Error: &ErrorBody{Code: ErrNotFound, Message: "nothing"}}
	})
	c := NewClient(port)
	resp, err := c.Call(context.Background(), "find", &Args{Needles: []string{"sign", "in"}})
	if err != nil || resp.SnapshotID != 7 || resp.payload().Nodes[0].Label != "Sign In" {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
	_, err = c.Call(context.Background(), "act", &Args{Kind: "tap"})
	var ae *AgentError
	if !errors.As(err, &ae) || ae.Code != ErrNotFound || !strings.Contains(err.Error(), "nothing") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientBadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)
	if _, err := NewClient(port).Call(context.Background(), "status", nil); err == nil || isTransport(err) {
		t.Fatalf("err = %v", err)
	}
	// ok:false without an error body.
	_, port2 := agentServer(t, func(string, Args) Response { return Response{OK: false} })
	if _, err := NewClient(port2).Call(context.Background(), "status", nil); err == nil {
		t.Fatal("want agent error")
	}
}

func deadPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestClientRevivesAndResendsReadOnly(t *testing.T) {
	var served atomic.Int32
	_, live := agentServer(t, func(cmd string, _ Args) Response {
		served.Add(1)
		return Response{OK: true, Data: &Payload{Message: cmd}}
	})
	c := NewClient(deadPort(t))
	var revived atomic.Int32
	c.SetReviver(func(context.Context) (int, error) {
		revived.Add(1)
		return live, nil
	})
	resp, err := c.Call(context.Background(), "snapshot", nil)
	if err != nil || resp.payload().Message != "snapshot" || revived.Load() != 1 {
		t.Fatalf("resp=%+v err=%v revived=%d", resp, err, revived.Load())
	}

	// An action is not re-sent after a restart.
	c.SetPort(deadPort(t))
	_, err = c.Call(context.Background(), "act", &Args{Kind: "tap"})
	if err == nil || !strings.Contains(err.Error(), "not re-sent") || served.Load() != 1 {
		t.Fatalf("err=%v served=%d", err, served.Load())
	}

	// Setting the orientation is re-sent; opening a URL is not.
	c.SetPort(deadPort(t))
	if _, err := c.Call(context.Background(), "device", &Args{Action: "orientation", Value: "portrait"}); err != nil || served.Load() != 2 {
		t.Fatalf("orientation: err=%v served=%d", err, served.Load())
	}
	c.SetPort(deadPort(t))
	if _, err := c.Call(context.Background(), "device", &Args{Action: "openURL", Value: "x://y"}); err == nil || !strings.Contains(err.Error(), "not re-sent") {
		t.Fatalf("openURL: err=%v", err)
	}

	// A failed restart is reported.
	c.SetPort(deadPort(t))
	c.SetReviver(func(context.Context) (int, error) { return 0, errBoom })
	if _, err := c.Call(context.Background(), "status", nil); err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientTimeoutAndCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(block)
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)
	c := NewClient(port)
	c.callTimeout = 100 * time.Millisecond
	if _, err := c.Call(context.Background(), "status", nil); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Call(ctx, "status", nil); err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("err = %v", err)
	}
}

func TestPortFor(t *testing.T) {
	for _, udid := range []string{"B4D6E1C2-1111-2222-3333-0123456789AB", "00008101-001C0C660A13001E", "not-hex-zz"} {
		p := PortFor(udid)
		if p < 22100 || p > 22899 {
			t.Errorf("PortFor(%s) = %d", udid, p)
		}
	}
	if PortFor("A-0000000000FF") == PortFor("A-000000000100") {
		t.Error("neighbouring UDIDs should differ")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = l.Close() }()
	if portFree(l.Addr().(*net.TCPAddr).Port) {
		t.Error("a bound port is not free")
	}
}

func writeManifest(t *testing.T, dir, protocol string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"version": "abc123", "protocol": protocol})
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	if _, err := readManifest(dir); err == nil {
		t.Fatal("missing manifest")
	}
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{"), 0o644)
	if _, err := readManifest(dir); err == nil {
		t.Fatal("bad json")
	}
	writeManifest(t, dir, "2.0")
	if _, err := readManifest(dir); err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("err = %v", err)
	}
	writeManifest(t, dir, "1.3")
	if m, err := readManifest(dir); err != nil || m.Version != "abc123" {
		t.Fatalf("m = %+v err = %v", m, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	if _, ok := loadState("SIM-9"); ok {
		t.Fatal("no state yet")
	}
	saveState("SIM-9", agentState{Port: 22123, Version: "v1"})
	s, ok := loadState("SIM-9")
	if !ok || s.Port != 22123 || s.Version != "v1" {
		t.Fatalf("state = %+v", s)
	}
}

func TestStartAgentReattaches(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, dir, "1.0")
	_, port := agentServer(t, func(cmd string, _ Args) Response {
		return Response{OK: true, Data: &Payload{Message: "ready"}}
	})
	saveState("SIM-R", agentState{Port: port, Version: "abc123"})
	a, c, err := StartAgent(context.Background(), AgentOptions{UDID: "SIM-R", Dir: dir})
	if err != nil || a.Mode() != "reattached" || a.Port() != port || c == nil {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	// Release leaves a daemon agent running and its state in place.
	a.Release(context.Background(), c)
	if _, ok := loadState("SIM-R"); !ok {
		t.Fatal("release should keep the state")
	}
	// Stop asks it to exit and forgets it.
	a.Stop(context.Background(), c)
	if _, ok := loadState("SIM-R"); ok {
		t.Fatal("stop should drop the state")
	}
}

func TestStartAgentBadDir(t *testing.T) {
	t.Setenv("DEVICELAB_IOS_AGENT_DIR", t.TempDir())
	if _, _, err := StartAgent(context.Background(), AgentOptions{UDID: "SIM-X"}); err == nil {
		t.Fatal("no manifest should fail")
	}
}

func TestXctestrunForSetsPort(t *testing.T) {
	if _, err := os.Stat("/usr/bin/plutil"); err != nil {
		t.Skip("plutil not available")
	}
	dir := t.TempDir()
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>DevicelabIOSAgentUITests</key><dict>
<key>EnvironmentVariables</key><dict><key>DL_AGENT_PORT</key><string>22087</string></dict>
<key>TestingEnvironmentVariables</key><dict><key>DL_AGENT_PORT</key><string>22087</string></dict>
</dict></dict></plist>`
	if err := os.WriteFile(filepath.Join(dir, "agent.xctestrun"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Agent{opts: AgentOptions{Dir: dir}}
	path, err := a.xctestrunFor(22345)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Count(string(raw), "22345") != 2 || filepath.Base(path) != "agent-22345.xctestrun" {
		t.Fatalf("xctestrun = %s", raw)
	}
	if _, err := (&Agent{opts: AgentOptions{Dir: t.TempDir()}}).xctestrunFor(1); err == nil {
		t.Fatal("missing xctestrun should fail")
	}
}

func TestLaunchUnknownMode(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	a := &Agent{opts: AgentOptions{UDID: "SIM-M", Mode: "bogus"}}
	if _, err := a.launch(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown agent launch mode") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogCall(t *testing.T) {
	logCall("find", time.Millisecond, &Response{ServerMs: 3, Phases: map[string]float64{"snapshotMs": 2, "nodes": 10}}, 10, 20, nil)
	logCall("act", time.Millisecond, nil, 10, 0, errBoom)
}
