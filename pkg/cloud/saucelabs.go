package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

func init() {
	Register(newSauceLabs)
}

func newSauceLabs(appiumURL string) Provider {
	if !strings.Contains(strings.ToLower(appiumURL), "saucelabs") {
		return nil
	}
	return &sauceLabs{}
}

type sauceLabs struct{}

func (s *sauceLabs) Name() string { return "Sauce Labs" }

func (s *sauceLabs) ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string) {
	sid := strings.TrimSpace(sessionID)
	meta[MetaSessionID] = sid
	if capsDeviceNameIndicatesEmuSim(caps) {
		meta["type"] = "vms"
		meta["jobID"] = sid
	} else {
		meta["type"] = "rdc"
		meta["jobID"] = jobUUIDFromSessionCaps(caps)
		if meta["jobID"] == "" {
			logger.Info("*** Sauce Labs ExtractMeta: no jobUuid in session caps; using session id as VMS job id")
			meta["type"] = "vms"
			meta["jobID"] = sid
		}
	}
	logger.Info("*** Sauce Labs ExtractMeta: type=%q jobID=%q", meta["type"], meta["jobID"])
}

// OnRunStart is a no-op — Sauce Labs jobs are created server-side by the
// Appium session, so there's nothing to signal at run start.
func (s *sauceLabs) OnRunStart(meta map[string]string, totalFlows int) error {
	logger.Info("*** Sauce Labs OnRunStart called: totalFlows=%d", totalFlows)
	logSauceMeta("OnRunStart", meta)
	return nil
}

// OnFlowStart calls Sauce executeScript via WebDriver execute/sync using
// appiumURL + sessionID from meta.
func (s *sauceLabs) OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error {
	logger.Info("*** Sauce Labs OnFlowStart called: flowIdx=%d totalFlows=%d name=%q file=%q", flowIdx, totalFlows, name, file)
	logSauceMeta("OnFlowStart", meta)
	appiumURL := strings.TrimSpace(meta[MetaAppiumURL])
	sessionID := strings.TrimSpace(meta[MetaSessionID])
	if appiumURL == "" || sessionID == "" {
		logger.Warn("Sauce Labs OnFlowStart: missing appiumURL/sessionID, skip Sauce executeScript call")
		return nil
	}

	yamlForContext := strings.TrimSpace(file)
	if yamlForContext != "" {
		yamlForContext = filepath.Base(yamlForContext)
	}
	if err := callSauceExecuteScriptContext(appiumURL, sessionID, flowIdx, totalFlows, yamlForContext); err != nil {
		return fmt.Errorf("call sauce executeScript: %w", err)
	}
	logger.Info("Sauce Labs OnFlowStart: executeScript sauce:context (yaml file) for [%d/%d]: %q", flowIdx+1, totalFlows, yamlForContext)
	return nil
}

// OnFlowEnd is a no-op — Sauce's job result is updated once at run end via
// ReportResult. There's no per-test-case live update API.
func (s *sauceLabs) OnFlowEnd(meta map[string]string, result *FlowResult) error {
	return nil
}

func (s *sauceLabs) ReportResult(appiumURL string, meta map[string]string, result *TestResult) error {
	jobID := strings.TrimSpace(meta["jobID"])
	logger.Info("*** Sauce Labs ReportResult: starting with jobID=%q result=%+v", jobID, result)
	if jobID == "" {
		return fmt.Errorf("no job ID available")
	}

	apiBase, err := apiBaseFromAppiumURL(appiumURL)
	if err != nil {
		return err
	}

	username, accessKey, err := credentialsFromAppiumURL(appiumURL)
	if err != nil {
		return err
	}

	endpoint, err := sauceJobRESTURL(apiBase, meta["type"], jobID, username)
	if err != nil {
		return err
	}

	// GET same job URL used for RDC ("/v1/rdc/...") or VMS ("/rest/v1/.../jobs/...") and read "name" from the JSON.
	nameFromJob := ""
	if n, err := getSauceJobNameFromEndpoint(endpoint, username, accessKey); err != nil {
		logger.Warn("Sauce Labs ReportResult: job GET (for name): %v", err)
	} else {
		nameFromJob = n
		logger.Info("Sauce Labs ReportResult: name from job GET response: %q", nameFromJob)
	}

	putName := ""
	if shouldSetJobNameFromFlowYAML(nameFromJob) {
		if stem := firstFlowFileStemWithoutYAMLExt(result); stem != "" {
			putName = stem
			logger.Info("Sauce Labs ReportResult: will set name on job update to %q (from flow file, replacing empty/default)", putName)
		}
	}

	return updateJob(endpoint, username, accessKey, result.Passed, putName)
}

// apiBaseFromAppiumURL returns the Sauce Labs REST API base URL.
// Region is inferred from the Appium hub URL.
func apiBaseFromAppiumURL(appiumURL string) (string, error) {
	raw := strings.TrimSpace(appiumURL)
	if raw == "" {
		return "", fmt.Errorf("empty appium url")
	}
	if _, err := url.Parse(raw); err != nil {
		return "", fmt.Errorf("parse appium url: %w", err)
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "eu-central-1"):
		return "https://api.eu-central-1.saucelabs.com", nil
	case strings.Contains(lower, "us-east-4"):
		return "https://api.us-east-4.saucelabs.com", nil
	default:
		return "https://api.us-west-1.saucelabs.com", nil
	}
}

// sauceJobRESTURL is the job resource URL used for RDC (GET/PUT) or VMS (GET/PUT).
func sauceJobRESTURL(apiBase, jobType, jobID, username string) (string, error) {
	base := strings.TrimSuffix(strings.TrimSpace(apiBase), "/")
	escapedID := url.PathEscape(jobID)
	escapedUser := url.PathEscape(username)
	switch jobType {
	case "rdc":
		return base + "/v1/rdc/jobs/" + escapedID, nil
	case "vms":
		return base + "/rest/v1/" + escapedUser + "/jobs/" + escapedID, nil
	default:
		return "", fmt.Errorf("unknown Sauce Labs job type: %q", jobType)
	}
}

// credentialsFromAppiumURL extracts Sauce Labs credentials from the URL userinfo
// or falls back to SAUCE_USERNAME and SAUCE_ACCESS_KEY environment variables.
func credentialsFromAppiumURL(appiumURL string) (username, accessKey string, err error) {
	u, err := url.Parse(strings.TrimSpace(appiumURL))
	if err != nil {
		return "", "", fmt.Errorf("parse appium url: %w", err)
	}
	if u.User != nil {
		username = strings.TrimSpace(u.User.Username())
		if pw, ok := u.User.Password(); ok {
			accessKey = strings.TrimSpace(pw)
		}
	}
	if username != "" && accessKey != "" {
		return username, accessKey, nil
	}
	username = strings.TrimSpace(os.Getenv("SAUCE_USERNAME"))
	accessKey = strings.TrimSpace(os.Getenv("SAUCE_ACCESS_KEY"))
	if username == "" || accessKey == "" {
		return "", "", fmt.Errorf("sauce credentials missing: use https://USERNAME:ACCESS_KEY@... in --appium-url or set SAUCE_USERNAME and SAUCE_ACCESS_KEY")
	}
	return username, accessKey, nil
}

// capsDeviceNameIndicatesEmuSim returns true when any capability key containing
// "deviceName" has a value containing "Emulator" or "Simulator".
func capsDeviceNameIndicatesEmuSim(caps map[string]interface{}) bool {
	if caps == nil {
		return false
	}
	var walk func(map[string]interface{}, int) bool
	walk = func(m map[string]interface{}, depth int) bool {
		if m == nil || depth > 4 {
			return false
		}
		for k, v := range m {
			if strings.Contains(strings.ToLower(k), "devicename") {
				if s, ok := v.(string); ok {
					lower := strings.ToLower(s)
					if strings.Contains(lower, "emulator") || strings.Contains(lower, "simulator") {
						return true
					}
				}
			}
			if sub, ok := v.(map[string]interface{}); ok {
				if walk(sub, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return walk(caps, 0)
}

// jobUUIDFromSessionCaps reads the Sauce Labs RDC job UUID from session capabilities.
func jobUUIDFromSessionCaps(caps map[string]interface{}) string {
	if caps == nil {
		return ""
	}
	for _, key := range []string{"appium:jobUuid", "jobUuid"} {
		if s, ok := caps[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// getSauceJobNameFromEndpoint GETs a job (RDC or VMS path) and returns the "name" field from the JSON.
func getSauceJobNameFromEndpoint(endpoint, username, accessKey string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build get request: %w", err)
	}
	req.SetBasicAuth(username, accessKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("sauce labs: close get job body: %v", err)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	name := nameFromSauceGetJobResponse(parsed)
	return name, nil
}

// nameFromSauceGetJobResponse returns the "name" field from Sauce job GET JSON (top-level or common wrappers).
func nameFromSauceGetJobResponse(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	if s := stringField(m, "name"); s != "" {
		return s
	}
	for _, k := range []string{"value", "job", "data"} {
		if sub, ok := m[k].(map[string]interface{}); ok {
			if s := stringField(sub, "name"); s != "" {
				return s
			}
		}
	}
	return ""
}

func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

const defaultSauceAppiumTestName = "Default Appium Test"

// shouldSetJobNameFromFlowYAML is true when the job has no real name in Sauce
// and we should send "name" from the flow file on the status PUT.
func shouldSetJobNameFromFlowYAML(nameFromJob string) bool {
	t := strings.TrimSpace(nameFromJob)
	return t == "" || t == defaultSauceAppiumTestName
}

// firstFlowFileStemWithoutYAMLExt returns the first flow's file basename without .yaml or .yml.
func firstFlowFileStemWithoutYAMLExt(result *TestResult) string {
	if result == nil {
		return ""
	}
	for _, f := range result.Flows {
		p := strings.TrimSpace(f.File)
		if p == "" {
			continue
		}
		base := filepath.Base(p)
		if ext := filepath.Ext(base); ext != "" {
			if strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml") {
				return base[:len(base)-len(ext)]
			}
			return strings.TrimSuffix(base, ext)
		}
		return base
	}
	return ""
}

// updateJob sends a PUT to the job endpoint. If putName is non-empty, the JSON
// body includes "name" alongside "passed" (RDC and VMS job update).
func updateJob(endpoint, username, accessKey string, passed bool, putName string) error {
	reqBody := map[string]interface{}{"passed": passed}
	if strings.TrimSpace(putName) != "" {
		reqBody["name"] = strings.TrimSpace(putName)
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(username, accessKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http put: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("sauce labs: close response body: %v", err)
		}
	}()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sauce labs api %s: status %d, body: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func callSauceExecuteScriptContext(appiumURL, sessionID string, flowIdx, totalFlows int, contextText string) error {
	endpoint := strings.TrimSuffix(strings.TrimSpace(appiumURL), "/") + "/session/" + url.PathEscape(sessionID) + "/execute/sync"
	contextMsg := fmt.Sprintf("- Starting Maestro test flow: %s", contextText)
	payload := map[string]interface{}{
		"script": "sauce:context= " + contextMsg,
		"args":   []interface{}{},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal execute payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("build execute request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http post execute: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("sauce labs: close execute response body: %v", err)
		}
	}()

	execReadBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sauce execute %s: status %d, body: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(execReadBody)))
	}
	return nil
}


func logSauceMeta(hook string, meta map[string]string) {
	if len(meta) == 0 {
		logger.Info("Sauce Labs %s meta: <empty>", hook)
		return
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		logger.Info("Sauce Labs %s meta[%s]=%q", hook, k, meta[k])
	}
}
