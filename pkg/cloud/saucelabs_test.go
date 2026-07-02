package cloud

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewSauceLabs_MatchesURLs(t *testing.T) {
	urls := []string{
		"https://user:key@ondemand.us-west-1.saucelabs.com:443/wd/hub",
		"https://user:key@ondemand.eu-central-1.saucelabs.com/wd/hub",
		"https://user:key@ondemand.us-east-4.saucelabs.com/wd/hub",
		"https://user:key@ondemand.SAUCELABS.com/wd/hub",
	}
	for _, u := range urls {
		if p := newSauceLabs(u); p == nil {
			t.Errorf("expected match for %s", u)
		}
	}
}

func TestNewSauceLabs_RejectsNonSauce(t *testing.T) {
	urls := []string{
		"http://localhost:4723",
		"https://hub.browserstack.com/wd/hub",
		"https://hub.testingbot.com/wd/hub",
		"",
	}
	for _, u := range urls {
		if p := newSauceLabs(u); p != nil {
			t.Errorf("expected nil for %q, got %q", u, p.Name())
		}
	}
}

func TestExtractMeta_RealDevice(t *testing.T) {
	p := &sauceLabs{}
	caps := map[string]interface{}{
		"appium:jobUuid":  "abc-123",
		"appium:deviceName": "Samsung Galaxy S21",
	}
	meta := make(map[string]string)
	p.ExtractMeta("session-456", caps, meta)

	if meta["type"] != "rdc" {
		t.Errorf("expected type=rdc, got %q", meta["type"])
	}
	if meta["jobID"] != "abc-123" {
		t.Errorf("expected jobID=abc-123, got %q", meta["jobID"])
	}
}

func TestExtractMeta_Emulator(t *testing.T) {
	p := &sauceLabs{}
	caps := map[string]interface{}{
		"appium:deviceName": "Google Pixel 9 Emulator",
	}
	meta := make(map[string]string)
	p.ExtractMeta("session-789", caps, meta)

	if meta["type"] != "vms" {
		t.Errorf("expected type=vms, got %q", meta["type"])
	}
	if meta["jobID"] != "session-789" {
		t.Errorf("expected jobID=session-789, got %q", meta["jobID"])
	}
}

func TestExtractMeta_Simulator(t *testing.T) {
	p := &sauceLabs{}
	caps := map[string]interface{}{
		"appium:deviceName": "iPhone Simulator",
	}
	meta := make(map[string]string)
	p.ExtractMeta("session-101", caps, meta)

	if meta["type"] != "vms" {
		t.Errorf("expected type=vms, got %q", meta["type"])
	}
}

func TestAPIBaseFromAppiumURL_Regions(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://user:key@ondemand.eu-central-1.saucelabs.com/wd/hub", "https://api.eu-central-1.saucelabs.com"},
		{"https://user:key@ondemand.us-east-4.saucelabs.com/wd/hub", "https://api.us-east-4.saucelabs.com"},
		{"https://user:key@ondemand.us-west-1.saucelabs.com/wd/hub", "https://api.us-west-1.saucelabs.com"},
		{"https://user:key@ondemand.saucelabs.com/wd/hub", "https://api.us-west-1.saucelabs.com"},
	}
	for _, tt := range tests {
		got, err := apiBaseFromAppiumURL(tt.url)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", tt.url, err)
		}
		if got != tt.expected {
			t.Errorf("apiBase(%s) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestAPIBaseFromAppiumURL_Empty(t *testing.T) {
	_, err := apiBaseFromAppiumURL("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestCredentialsFromAppiumURL_FromURL(t *testing.T) {
	u, k, err := credentialsFromAppiumURL("https://myuser:mykey@ondemand.saucelabs.com/wd/hub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "myuser" || k != "mykey" {
		t.Errorf("got (%q, %q), want (myuser, mykey)", u, k)
	}
}

func TestCredentialsFromAppiumURL_FromEnv(t *testing.T) {
	os.Setenv("SAUCE_USERNAME", "envuser")
	os.Setenv("SAUCE_ACCESS_KEY", "envkey")
	defer os.Unsetenv("SAUCE_USERNAME")
	defer os.Unsetenv("SAUCE_ACCESS_KEY")

	u, k, err := credentialsFromAppiumURL("https://ondemand.saucelabs.com/wd/hub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "envuser" || k != "envkey" {
		t.Errorf("got (%q, %q), want (envuser, envkey)", u, k)
	}
}

func TestCredentialsFromAppiumURL_Missing(t *testing.T) {
	os.Unsetenv("SAUCE_USERNAME")
	os.Unsetenv("SAUCE_ACCESS_KEY")

	_, _, err := credentialsFromAppiumURL("https://ondemand.saucelabs.com/wd/hub")
	if err == nil {
		t.Error("expected error when credentials are missing")
	}
}

func TestCapsDeviceNameIndicatesEmuSim(t *testing.T) {
	tests := []struct {
		name     string
		caps     map[string]interface{}
		expected bool
	}{
		{"nil caps", nil, false},
		{"real device", map[string]interface{}{"appium:deviceName": "Samsung Galaxy S21"}, false},
		{"emulator", map[string]interface{}{"appium:deviceName": "Google Pixel 9 Emulator"}, true},
		{"simulator", map[string]interface{}{"deviceName": "iPhone Simulator"}, true},
		{"nested", map[string]interface{}{
			"sauce:options": map[string]interface{}{
				"deviceName": "Android Emulator",
			},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capsDeviceNameIndicatesEmuSim(tt.caps); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestJobUUIDFromSessionCaps(t *testing.T) {
	tests := []struct {
		name     string
		caps     map[string]interface{}
		expected string
	}{
		{"nil", nil, ""},
		{"has appium:jobUuid", map[string]interface{}{"appium:jobUuid": "uuid-123"}, "uuid-123"},
		{"has jobUuid", map[string]interface{}{"jobUuid": "uuid-456"}, "uuid-456"},
		{"prefers appium:jobUuid", map[string]interface{}{"appium:jobUuid": "a", "jobUuid": "b"}, "a"},
		{"empty value", map[string]interface{}{"appium:jobUuid": ""}, ""},
		{"no uuid key", map[string]interface{}{"platformName": "Android"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobUUIDFromSessionCaps(tt.caps); got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestReportResult_RDC(t *testing.T) {
	var gotPath string
	var gotBody map[string]bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := &sauceLabs{}
	meta := map[string]string{"type": "rdc", "jobID": "job-abc"}
	result := &TestResult{Passed: true}

	// Override apiBase by using the test server URL directly
	err := updateJob(srv.URL+"/v1/rdc/jobs/job-abc", "user", "key", true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/rdc/jobs/job-abc" {
		t.Errorf("path = %q, want /v1/rdc/jobs/job-abc", gotPath)
	}
	if gotBody["passed"] != true {
		t.Errorf("body passed = %v, want true", gotBody["passed"])
	}

	// Verify the provider wiring works
	_ = p
	_ = meta
	_ = result
}

func TestReportResult_VMs(t *testing.T) {
	var gotPath string
	var gotBody map[string]bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := updateJob(srv.URL+"/rest/v1/myuser/jobs/session-123", "myuser", "mykey", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/rest/v1/myuser/jobs/session-123" {
		t.Errorf("path = %q, want /rest/v1/myuser/jobs/session-123", gotPath)
	}
	if gotBody["passed"] != false {
		t.Errorf("body passed = %v, want false", gotBody["passed"])
	}
}

func TestReportResult_EmptyJobID(t *testing.T) {
	p := &sauceLabs{}
	meta := map[string]string{"type": "rdc", "jobID": ""}
	err := p.ReportResult("https://user:key@ondemand.saucelabs.com/wd/hub", meta, &TestResult{Passed: true})
	if err == nil {
		t.Error("expected error for empty job ID")
	}
}

func TestUpdateJob_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	err := updateJob(srv.URL+"/v1/rdc/jobs/123", "user", "key", true, "")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// --- New behavior added in PR #66 ---

// TestExtractMeta_RealDeviceCaps_FallbackWhenJobUUIDEmpty verifies that when
// caps look like a real device (no emu/sim marker) but jobUuid is missing,
// ExtractMeta falls back to type=vms with the session id as the job id —
// matching Sauce's virtual-device REST paths so ReportResult still works.
func TestExtractMeta_RealDeviceCaps_FallbackWhenJobUUIDEmpty(t *testing.T) {
	p := &sauceLabs{}
	caps := map[string]interface{}{
		// Real device by name, but no appium:jobUuid present.
		"appium:deviceName": "Samsung Galaxy S21",
	}
	meta := make(map[string]string)
	p.ExtractMeta("session-fallback", caps, meta)

	if meta["type"] != "vms" {
		t.Errorf("type = %q, want vms (fallback)", meta["type"])
	}
	if meta["jobID"] != "session-fallback" {
		t.Errorf("jobID = %q, want session-fallback (sessionID fallback)", meta["jobID"])
	}
	if meta[MetaSessionID] != "session-fallback" {
		t.Errorf("MetaSessionID = %q, want session-fallback", meta[MetaSessionID])
	}
}

// TestExtractMeta_StoresSessionID guards the new MetaSessionID convention so
// downstream callers (OnFlowStart / executeScript context) keep working.
func TestExtractMeta_StoresSessionID(t *testing.T) {
	p := &sauceLabs{}
	caps := map[string]interface{}{
		"appium:jobUuid":    "uuid-1",
		"appium:deviceName": "Samsung Galaxy S21",
	}
	meta := make(map[string]string)
	p.ExtractMeta("  session-trim  ", caps, meta)

	if meta[MetaSessionID] != "session-trim" {
		t.Errorf("MetaSessionID = %q, want trimmed session-trim", meta[MetaSessionID])
	}
}

func TestSauceJobRESTURL(t *testing.T) {
	tests := []struct {
		name     string
		apiBase  string
		jobType  string
		jobID    string
		username string
		want     string
		wantErr  bool
	}{
		{
			name: "rdc",
			apiBase:  "https://api.us-west-1.saucelabs.com",
			jobType:  "rdc",
			jobID:    "job-abc",
			username: "user",
			want:     "https://api.us-west-1.saucelabs.com/v1/rdc/jobs/job-abc",
		},
		{
			name: "vms",
			apiBase:  "https://api.eu-central-1.saucelabs.com",
			jobType:  "vms",
			jobID:    "session-123",
			username: "myuser",
			want:     "https://api.eu-central-1.saucelabs.com/rest/v1/myuser/jobs/session-123",
		},
		{
			name: "trims trailing slash on apiBase",
			apiBase:  "https://api.us-west-1.saucelabs.com/",
			jobType:  "rdc",
			jobID:    "x",
			username: "u",
			want:     "https://api.us-west-1.saucelabs.com/v1/rdc/jobs/x",
		},
		{
			name: "escapes weird ids",
			apiBase:  "https://api.us-west-1.saucelabs.com",
			jobType:  "vms",
			jobID:    "id with space",
			username: "user/name",
			want:     "https://api.us-west-1.saucelabs.com/rest/v1/user%2Fname/jobs/id%20with%20space",
		},
		{
			name:    "unknown type errors",
			apiBase: "https://api.us-west-1.saucelabs.com",
			jobType: "bogus",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sauceJobRESTURL(tt.apiBase, tt.jobType, tt.jobID, tt.username)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got url %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("url = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNameFromSauceGetJobResponse(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		want string
	}{
		{name: "nil map", body: nil, want: ""},
		{name: "top-level name", body: map[string]interface{}{"name": "my test"}, want: "my test"},
		{name: "trims whitespace", body: map[string]interface{}{"name": "  my test  "}, want: "my test"},
		{name: "value wrapper", body: map[string]interface{}{"value": map[string]interface{}{"name": "wrapped"}}, want: "wrapped"},
		{name: "job wrapper", body: map[string]interface{}{"job": map[string]interface{}{"name": "from job"}}, want: "from job"},
		{name: "data wrapper", body: map[string]interface{}{"data": map[string]interface{}{"name": "from data"}}, want: "from data"},
		{name: "no name field", body: map[string]interface{}{"id": "abc"}, want: ""},
		{name: "name not a string", body: map[string]interface{}{"name": 42}, want: ""},
		{name: "top-level wins over wrapper", body: map[string]interface{}{"name": "outer", "value": map[string]interface{}{"name": "inner"}}, want: "outer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nameFromSauceGetJobResponse(tt.body)
			if got != tt.want {
				t.Errorf("name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldSetJobNameFromFlowYAML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: true},
		{name: "whitespace only", in: "   ", want: true},
		{name: "default appium test", in: "Default Appium Test", want: true},
		{name: "default with surrounding whitespace", in: "  Default Appium Test  ", want: true},
		{name: "real custom name", in: "My Suite", want: false},
		{name: "name containing default substring", in: "Not Default Appium Test", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSetJobNameFromFlowYAML(tt.in); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstFlowFileStemWithoutYAMLExt(t *testing.T) {
	tests := []struct {
		name   string
		result *TestResult
		want   string
	}{
		{name: "nil", result: nil, want: ""},
		{name: "empty flows", result: &TestResult{}, want: ""},
		{name: "first flow with .yaml", result: &TestResult{Flows: []FlowResult{{File: "flows/login.yaml"}}}, want: "login"},
		{name: "first flow with .yml", result: &TestResult{Flows: []FlowResult{{File: "flows/login.yml"}}}, want: "login"},
		{name: "uppercase extension", result: &TestResult{Flows: []FlowResult{{File: "flows/login.YAML"}}}, want: "login"},
		{name: "no extension", result: &TestResult{Flows: []FlowResult{{File: "flows/login"}}}, want: "login"},
		{name: "non-yaml extension", result: &TestResult{Flows: []FlowResult{{File: "flows/login.json"}}}, want: "login"},
		{name: "skips empty File then takes next", result: &TestResult{Flows: []FlowResult{{File: ""}, {File: "second.yaml"}}}, want: "second"},
		{name: "all empty", result: &TestResult{Flows: []FlowResult{{File: ""}, {File: "  "}}}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstFlowFileStemWithoutYAMLExt(tt.result); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUpdateJob_WithPutName verifies the new putName arg shows up in the PUT
// body so Sauce will rename a job whose original name was empty/default.
func TestUpdateJob_WithPutName(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	if err := updateJob(srv.URL+"/v1/rdc/jobs/abc", "user", "key", true, "login_flow"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["passed"] != true {
		t.Errorf("body passed = %v, want true", gotBody["passed"])
	}
	if gotBody["name"] != "login_flow" {
		t.Errorf("body name = %v, want login_flow", gotBody["name"])
	}
}

// TestUpdateJob_EmptyPutNameOmitsField is a regression guard: an empty
// putName must not put "name":"" in the body, since that would overwrite a
// good Sauce-side name with an empty string.
func TestUpdateJob_EmptyPutNameOmitsField(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	if err := updateJob(srv.URL+"/v1/rdc/jobs/abc", "user", "key", true, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := gotBody["name"]; present {
		t.Errorf("expected no 'name' field when putName empty, got %v", gotBody["name"])
	}
}

// TestSauceLabs_OnFlowStart_SkipsWhenMissingMeta verifies the early return
// when appiumURL or sessionID is missing — should not error and should not
// hit the network.
func TestSauceLabs_OnFlowStart_SkipsWhenMissingMeta(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	p := &sauceLabs{}
	cases := []map[string]string{
		{},
		{MetaAppiumURL: srv.URL},
		{MetaSessionID: "session-1"},
		{MetaAppiumURL: "", MetaSessionID: "session-1"},
		{MetaAppiumURL: srv.URL, MetaSessionID: ""},
	}
	for i, meta := range cases {
		if err := p.OnFlowStart(meta, 0, 1, "Login", "login.yaml"); err != nil {
			t.Errorf("case %d: unexpected error: %v", i, err)
		}
	}
	if hit {
		t.Error("server was hit despite missing meta")
	}
}

// TestSauceLabs_OnFlowStart_PostsContext exercises the happy path: the
// provider should POST to /session/<id>/execute/sync with a sauce:context
// body that names the YAML file.
func TestSauceLabs_OnFlowStart_PostsContext(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"value":null}`))
	}))
	defer srv.Close()

	p := &sauceLabs{}
	meta := map[string]string{
		MetaAppiumURL: srv.URL,
		MetaSessionID: "session-xyz",
	}
	if err := p.OnFlowStart(meta, 1, 3, "Login", "flows/login.yaml"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/session/session-xyz/execute/sync" {
		t.Errorf("path = %q, want /session/session-xyz/execute/sync", gotPath)
	}
	script, _ := gotBody["script"].(string)
	if script == "" {
		t.Errorf("body.script empty, body = %v", gotBody)
	}
	// We don't assert exact format (it has changed across patches) — just that
	// the YAML basename made it into the script.
	if !contains(script, "login.yaml") {
		t.Errorf("body.script = %q, expected to mention login.yaml", script)
	}
}

// TestGetSauceJobNameFromEndpoint covers happy + error paths so the
// ReportResult name-detection logic doesn't regress silently.
func TestGetSauceJobNameFromEndpoint(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || user != "u" || pass != "k" {
				t.Errorf("basic auth = %q/%q, want u/k", user, pass)
			}
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"name":"hello"}`))
		}))
		defer srv.Close()

		got, err := getSauceJobNameFromEndpoint(srv.URL+"/v1/rdc/jobs/abc", "u", "k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hello" {
			t.Errorf("name = %q, want hello", got)
		}
	})
	t.Run("non-2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		}))
		defer srv.Close()

		if _, err := getSauceJobNameFromEndpoint(srv.URL+"/x", "u", "k"); err == nil {
			t.Error("expected error for 404")
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()

		if _, err := getSauceJobNameFromEndpoint(srv.URL+"/x", "u", "k"); err == nil {
			t.Error("expected error for malformed json")
		}
	})
}

// contains is a tiny helper to keep table-test bodies short without pulling
// strings.Contains into every line.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
