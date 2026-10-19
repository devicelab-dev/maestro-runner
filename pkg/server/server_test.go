package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/driver/mock"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// recordingDriver wraps a driver and captures the steps handed to Execute so
// tests can assert the server decoded the JSON body into the same concrete
// flow.Step implementation YAML would have produced.
type recordingDriver struct {
	core.Driver
	mu    sync.Mutex
	steps []flow.Step
}

func (d *recordingDriver) Execute(step flow.Step) *core.CommandResult {
	d.mu.Lock()
	d.steps = append(d.steps, step)
	d.mu.Unlock()
	return core.SuccessResult("executed "+string(step.Type()), nil)
}

func (d *recordingDriver) recorded() []flow.Step {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]flow.Step(nil), d.steps...)
}

// hierarchyDriver overrides Hierarchy so a test can control whether the
// server sees XML or JSON and therefore which Content-Type it must emit.
type hierarchyDriver struct {
	core.Driver
	hierarchy []byte
	err       error
}

func (d hierarchyDriver) Hierarchy() ([]byte, error) { return d.hierarchy, d.err }

func newTestServer() (*Server, *mock.Driver) {
	drv := mock.New(mock.Config{Platform: "android", DeviceID: "test-device"})
	srv := New(func(req SessionRequest) (core.Driver, func(), error) {
		return drv, func() {}, nil
	})
	return srv, drv
}

// cleanupCounter counts how many session cleanups ran.
type cleanupCounter struct {
	mu sync.Mutex
	n  int
}

func (c *cleanupCounter) inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *cleanupCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// newServerWithDriver builds a server whose factory hands out drv and records
// how many times the returned cleanup ran.
func newServerWithDriver(drv core.Driver, cleaned *cleanupCounter) *Server {
	return New(func(req SessionRequest) (core.Driver, func(), error) {
		return drv, cleaned.inc, nil
	})
}

func createSession(t *testing.T, handler http.Handler) string {
	t.Helper()
	body := `{"platformName":"android","deviceId":"test-device"}`
	req := httptest.NewRequest("POST", "/session", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("session id is empty")
	}
	return resp.SessionID
}

// sessionRoutes are the {id}-scoped routes the server exposes. Every one of
// them must answer 404 for an unknown session so a client never mistakes a
// dead session for a device failure.
var sessionRoutes = []struct {
	name   string
	method string
	suffix string
}{
	{"execute", http.MethodPost, "/execute"},
	{"screenshot", http.MethodGet, "/screenshot"},
	{"source", http.MethodGet, "/source"},
	{"device-info", http.MethodGet, "/device-info"},
}

func TestSessionRoutes_SessionNotFound(t *testing.T) {
	for _, route := range sessionRoutes {
		t.Run(route.name, func(t *testing.T) {
			srv, _ := newTestServer()
			handler := srv.Handler()

			var body io.Reader
			if route.method == http.MethodPost {
				body = bytes.NewBufferString(`{"type":"back"}`)
			}
			req := httptest.NewRequest(route.method, "/session/nonexistent"+route.suffix, body)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", w.Code)
			}
			var errResp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if !strings.Contains(errResp.Error, "nonexistent") {
				t.Errorf("error %q should name the missing session", errResp.Error)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

func TestCreateSession(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["sessions"].(float64) != 1 {
		t.Errorf("expected 1 session, got %v", body["sessions"])
	}
	_ = sid
}

func TestCreateSession_MissingPlatform(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	req := httptest.NewRequest("POST", "/session", bytes.NewBufferString(`{"deviceId":"x"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestExecuteStep(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	step := `{"type":"tapOn","selector":"Login"}`
	req := httptest.NewRequest("POST", "/session/"+sid+"/execute", bytes.NewBufferString(step))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result core.CommandResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
}

func TestExecuteStep_InvalidJSON(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	req := httptest.NewRequest("POST", "/session/"+sid+"/execute", bytes.NewBufferString(`not json`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestExecuteStep_SessionNotFound(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	req := httptest.NewRequest("POST", "/session/nonexistent/execute", bytes.NewBufferString(`{"type":"back"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestScreenshot(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	req := httptest.NewRequest("GET", "/session/"+sid+"/screenshot", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png, got %s", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty body")
	}
}

func TestSource(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	req := httptest.NewRequest("GET", "/session/"+sid+"/source", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	if len(body) == 0 {
		t.Error("expected non-empty hierarchy")
	}
}

func TestDeviceInfo(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	req := httptest.NewRequest("GET", "/session/"+sid+"/device-info", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var info core.PlatformInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if info.Platform != "android" {
		t.Errorf("expected platform android, got %q", info.Platform)
	}
	if info.DeviceID != "test-device" {
		t.Errorf("expected deviceId test-device, got %q", info.DeviceID)
	}
}

// screenshotDriver overrides Screenshot so a test can exercise the failure
// branch, which the happy-path mock driver never reaches.
type screenshotDriver struct {
	core.Driver
	err error
}

func (d screenshotDriver) Screenshot() ([]byte, error) { return nil, d.err }

// TestScreenshot_DriverError asserts a capture failure surfaces as 500 with a
// JSON error, not as a 200 with an empty body the client would try to parse as
// a PNG.
func TestScreenshot_DriverError(t *testing.T) {
	base := mock.New(mock.Config{Platform: "android"})
	srv := newServerWithDriver(
		screenshotDriver{Driver: base, err: errors.New("screencap timed out")},
		&cleanupCounter{},
	)
	handler := srv.Handler()
	sid := createSession(t, handler)

	req := httptest.NewRequest("GET", "/session/"+sid+"/screenshot", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errResp.Error, "screencap timed out") {
		t.Errorf("error %q should mention the driver failure", errResp.Error)
	}
}

// TestSource_ContentType asserts the route serves the hierarchy bytes
// untouched and labels them by what they actually are: JSON hierarchies come
// back as application/json, XML hierarchies as application/xml. A client that
// trusted the label rather than the body would mis-parse the other case.
func TestSource_ContentType(t *testing.T) {
	xmlHierarchy := []byte(`<?xml version="1.0" encoding="UTF-8"?><hierarchy><node/></hierarchy>`)
	jsonHierarchy := []byte(`{"type":"View","children":[]}`)

	tests := []struct {
		name        string
		hierarchy   []byte
		wantType    string
		wantBodyStr string
	}{
		{"xml hierarchy", xmlHierarchy, "application/xml", string(xmlHierarchy)},
		{"json hierarchy", jsonHierarchy, "application/json", string(jsonHierarchy)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := mock.New(mock.Config{Platform: "android", DeviceID: "test-device"})
			srv := newServerWithDriver(hierarchyDriver{Driver: base, hierarchy: tt.hierarchy}, &cleanupCounter{})
			handler := srv.Handler()
			sid := createSession(t, handler)

			req := httptest.NewRequest("GET", "/session/"+sid+"/source", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
			if got := w.Body.String(); got != tt.wantBodyStr {
				t.Errorf("body = %q, want %q", got, tt.wantBodyStr)
			}
		})
	}
}

// TestSource_DriverError asserts a hierarchy failure surfaces as 500 with a
// JSON error rather than an empty 200 body the client would try to parse.
func TestSource_DriverError(t *testing.T) {
	base := mock.New(mock.Config{Platform: "android"})
	srv := newServerWithDriver(
		hierarchyDriver{Driver: base, err: errors.New("adb shell failed")},
		&cleanupCounter{},
	)
	handler := srv.Handler()
	sid := createSession(t, handler)

	req := httptest.NewRequest("GET", "/session/"+sid+"/source", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errResp.Error, "adb shell failed") {
		t.Errorf("error %q should mention the driver failure", errResp.Error)
	}
}

// TestExecuteStep_ReachesDriverUnchanged covers the protocol's central promise:
// the JSON body decodes into the same concrete flow.Step that YAML produces and
// arrives at the driver untouched, selectors included.
func TestExecuteStep_ReachesDriverUnchanged(t *testing.T) {
	base := mock.New(mock.Config{Platform: "android", DeviceID: "test-device"})
	rec := &recordingDriver{Driver: base}
	srv := newServerWithDriver(rec, &cleanupCounter{})
	handler := srv.Handler()
	sid := createSession(t, handler)

	body := `{"type":"tapOn","selector":{"text":"Login","id":"btn_login"},"timeout":5000,"optional":true}`
	req := httptest.NewRequest("POST", "/session/"+sid+"/execute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := rec.recorded()
	if len(got) != 1 {
		t.Fatalf("driver executed %d steps, want 1", len(got))
	}
	tap, ok := got[0].(*flow.TapOnStep)
	if !ok {
		t.Fatalf("driver received %T, want *flow.TapOnStep", got[0])
	}
	if tap.Type() != flow.StepTapOn {
		t.Errorf("step type = %q, want %q", tap.Type(), flow.StepTapOn)
	}
	if tap.Selector.Text != "Login" || tap.Selector.ID != "btn_login" {
		t.Errorf("selector = %+v, want text=Login id=btn_login", tap.Selector)
	}
	if tap.TimeoutMs != 5000 {
		t.Errorf("timeout = %d, want 5000", tap.TimeoutMs)
	}
	if !tap.IsOptional() {
		t.Error("expected optional=true to survive decoding")
	}
}

// TestExecuteStep_FailedStepStillReturnsOKResult asserts a step the driver
// rejects comes back as HTTP 200 with success=false, not as a transport error.
// Clients branch on the result body, not the status code.
func TestExecuteStep_FailedStepStillReturnsOKResult(t *testing.T) {
	drv := mock.New(mock.Config{Platform: "android", FailOnStep: 1})
	srv := newServerWithDriver(drv, &cleanupCounter{})
	handler := srv.Handler()
	sid := createSession(t, handler)

	req := httptest.NewRequest("POST", "/session/"+sid+"/execute",
		bytes.NewBufferString(`{"type":"tapOn","selector":"Nope"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result core.CommandResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Success {
		t.Error("expected success=false for a step the mock driver fails")
	}
}

// TestExecuteStep_UnsupportedStepType covers the "clear 400 rather than panic"
// requirement for a well-formed step whose type the protocol does not support.
func TestExecuteStep_UnsupportedStepType(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)

	req := httptest.NewRequest("POST", "/session/"+sid+"/execute",
		bytes.NewBufferString(`{"type":"teleport","selector":"Login"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errResp.Error, "teleport") {
		t.Errorf("error %q should name the unsupported step type", errResp.Error)
	}
}

func TestCreateSession_InvalidJSON(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/session", bytes.NewBufferString(`{"platformName":`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

// TestCreateSession_DriverFailure asserts that when the factory cannot bring up
// a driver the client gets 500 and no session is registered, so /status does
// not advertise a session that never existed.
func TestCreateSession_DriverFailure(t *testing.T) {
	srv := New(func(req SessionRequest) (core.Driver, func(), error) {
		return nil, nil, errors.New("no devices found")
	})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/session",
		bytes.NewBufferString(`{"platformName":"android"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errResp.Error, "no devices found") {
		t.Errorf("error %q should mention the driver failure", errResp.Error)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body["sessions"].(float64) != 0 {
		t.Errorf("expected 0 sessions after a failed create, got %v", body["sessions"])
	}
}

// TestCreateSession_PlatformIsPassedThrough asserts the server forwards the
// client's deviceId/appId/driver choice to the factory rather than silently
// substituting a global default.
func TestCreateSession_PlatformIsPassedThrough(t *testing.T) {
	var got SessionRequest
	srv := New(func(req SessionRequest) (core.Driver, func(), error) {
		got = req
		return mock.New(mock.Config{Platform: "ios"}), func() {}, nil
	})
	handler := srv.Handler()

	body := `{"platformName":"ios","deviceId":"00008030-DEVICE","appId":"com.example.app","driver":"appium"}`
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/session", bytes.NewBufferString(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if got.PlatformName != "ios" {
		t.Errorf("platformName = %q, want ios", got.PlatformName)
	}
	if got.DeviceID != "00008030-DEVICE" {
		t.Errorf("deviceId = %q, want 00008030-DEVICE", got.DeviceID)
	}
	if got.AppID != "com.example.app" {
		t.Errorf("appId = %q, want com.example.app", got.AppID)
	}
	if got.Driver != "appium" {
		t.Errorf("driver = %q, want appium", got.Driver)
	}
}

func TestDeleteSession(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	sid := createSession(t, handler)
	req := httptest.NewRequest("DELETE", "/session/"+sid, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	req = httptest.NewRequest("GET", "/session/"+sid+"/device-info", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

// TestDeleteSession_RunsCleanup asserts deleting a session tears the driver
// down. Without this, every leaked session would hold a device connection open.
func TestDeleteSession_RunsCleanup(t *testing.T) {
	cleaned := &cleanupCounter{}
	base := mock.New(mock.Config{Platform: "android"})
	srv := newServerWithDriver(base, cleaned)
	handler := srv.Handler()

	sid := createSession(t, handler)
	if got := cleaned.count(); got != 0 {
		t.Fatalf("cleanup ran %d times before delete, want 0", got)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("DELETE", "/session/"+sid, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if got := cleaned.count(); got != 1 {
		t.Errorf("cleanup ran %d times, want 1", got)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected an empty 204 body, got %q", w.Body.String())
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()
	req := httptest.NewRequest("DELETE", "/session/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestShutdownAll covers the SIGINT/SIGTERM contract: every live session gets
// its cleanup run, not just the ones the client happened to delete.
func TestShutdownAll(t *testing.T) {
	cleaned := &cleanupCounter{}
	base := mock.New(mock.Config{Platform: "android"})
	srv := newServerWithDriver(base, cleaned)
	handler := srv.Handler()

	const sessions = 3
	for i := 0; i < sessions; i++ {
		createSession(t, handler)
	}

	srv.ShutdownAll()

	if got := cleaned.count(); got != sessions {
		t.Errorf("cleanup ran %d times, want %d", got, sessions)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body["sessions"].(float64) != 0 {
		t.Errorf("expected 0 sessions after shutdown, got %v", body["sessions"])
	}
}

// TestShutdownAll_EmptyIsSafe asserts shutting down with nothing running does
// not run any cleanup and does not panic. The signal handler can fire at any
// moment, so ShutdownAll must tolerate an empty map; if it did not, a stray
// Ctrl-C would crash a server that had simply not been used yet.
func TestShutdownAll_EmptyIsSafe(t *testing.T) {
	cleaned := &cleanupCounter{}
	srv := newServerWithDriver(mock.New(mock.Config{Platform: "android"}), cleaned)

	srv.ShutdownAll()
	srv.ShutdownAll()

	if got := cleaned.count(); got != 0 {
		t.Errorf("cleanup ran %d times with no sessions, want 0", got)
	}

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body["sessions"].(float64) != 0 {
		t.Errorf("sessions = %v, want 0", body["sessions"])
	}
}

// TestShutdownAll_AfterDeleteIsIdempotent asserts a session already torn down
// by DELETE is not cleaned up a second time when shutdown follows — double
// cleanup would tear down an already-freed driver.
func TestShutdownAll_AfterDeleteIsIdempotent(t *testing.T) {
	cleaned := &cleanupCounter{}
	srv := newServerWithDriver(mock.New(mock.Config{Platform: "android"}), cleaned)
	handler := srv.Handler()

	sid := createSession(t, handler)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("DELETE", "/session/"+sid, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	srv.ShutdownAll()

	if got := cleaned.count(); got != 1 {
		t.Errorf("cleanup ran %d times, want exactly 1 (delete only)", got)
	}
}

// TestConcurrentSessionAccess hammers create/status/execute/delete from many
// goroutines at once. Run with -race, this is what proves the session map is
// not corrupted under the parallel clients the API is built for.
func TestConcurrentSessionAccess(t *testing.T) {
	base := mock.New(mock.Config{Platform: "android", DeviceID: "test-device"})
	srv := newServerWithDriver(&recordingDriver{Driver: base}, &cleanupCounter{})
	handler := srv.Handler()

	const goroutines = 12
	const iterations = 15

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("POST", "/session",
					bytes.NewBufferString(`{"platformName":"android"}`)))
				if w.Code != http.StatusOK {
					t.Errorf("create: expected 200, got %d", w.Code)
					return
				}
				var resp SessionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Errorf("decode session: %v", err)
					return
				}

				w = httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
				if w.Code != http.StatusOK {
					t.Errorf("status: expected 200, got %d", w.Code)
					return
				}
				var body map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
					t.Errorf("decode status body: %v", err)
					return
				}
				if body["status"] != "ok" {
					t.Errorf("status = %v, want ok", body["status"])
					return
				}

				w = httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("POST",
					"/session/"+resp.SessionID+"/execute",
					bytes.NewBufferString(`{"type":"tapOn","selector":"Login"}`)))
				if w.Code != http.StatusOK {
					t.Errorf("execute: expected 200, got %d", w.Code)
					return
				}

				w = httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("DELETE", "/session/"+resp.SessionID, nil))
				if w.Code != http.StatusNoContent {
					t.Errorf("delete: expected 204, got %d", w.Code)
					return
				}
			}
		}()
	}
	wg.Wait()

	srv.ShutdownAll()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body["sessions"].(float64) != 0 {
		t.Errorf("expected 0 sessions after every delete, got %v", body["sessions"])
	}
}
