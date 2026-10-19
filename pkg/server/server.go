// Package server provides a REST API server that bridges HTTP calls to core.Driver.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// SessionState holds a driver session.
type SessionState struct {
	Driver  core.Driver
	Cleanup func()
}

// Server is the REST API server.
type Server struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState

	// CreateDriver is called when POST /session is invoked.
	// It receives the session request and must return a driver + cleanup func.
	CreateDriver func(req SessionRequest) (core.Driver, func(), error)
}

// SessionRequest is the JSON body for POST /session.
type SessionRequest struct {
	PlatformName string `json:"platformName"`
	DeviceID     string `json:"deviceId,omitempty"`
	AppID        string `json:"appId,omitempty"`
	Driver       string `json:"driver,omitempty"`
}

// SessionResponse is the JSON response for POST /session.
type SessionResponse struct {
	SessionID string `json:"sessionId"`
}

// ErrorResponse is a JSON error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// New creates a new Server.
func New(createDriver func(req SessionRequest) (core.Driver, func(), error)) *Server {
	return &Server{
		sessions:     make(map[string]*SessionState),
		CreateDriver: createDriver,
	}
}

// Handler returns an http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("POST /session", s.handleCreateSession)
	mux.HandleFunc("POST /session/{id}/execute", s.handleExecute)
	mux.HandleFunc("GET /session/{id}/screenshot", s.handleScreenshot)
	mux.HandleFunc("GET /session/{id}/source", s.handleSource)
	mux.HandleFunc("GET /session/{id}/device-info", s.handleDeviceInfo)
	mux.HandleFunc("DELETE /session/{id}", s.handleDeleteSession)

	return mux
}

// ShutdownAll cleans up all active sessions.
func (s *Server) ShutdownAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		logger.Info("Cleaning up session %s", id)
		if sess.Cleanup != nil {
			sess.Cleanup()
		}
		delete(s.sessions, id)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.mu.RLock()
	count := len(s.sessions)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"sessions": count,
	})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req SessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
		serverTracef(
			"server request worker=%s method=%s path=%s status=%d duration_ms=%d error=%q",
			serverWorkerID(), r.Method, r.URL.Path, http.StatusBadRequest, time.Since(started).Milliseconds(),
			err.Error(),
		)
		return
	}

	if req.PlatformName == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "platformName is required"})
		serverTracef(
			"server request worker=%s method=%s path=%s status=%d duration_ms=%d error=%q",
			serverWorkerID(), r.Method, r.URL.Path, http.StatusBadRequest, time.Since(started).Milliseconds(),
			"platformName is required",
		)
		return
	}

	driver, cleanup, err := s.CreateDriver(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create driver: " + err.Error()})
		serverTracef(
			"server request worker=%s method=%s path=%s status=%d duration_ms=%d error=%q platform=%s deviceId=%s",
			serverWorkerID(), r.Method, r.URL.Path, http.StatusInternalServerError,
			time.Since(started).Milliseconds(), err.Error(), req.PlatformName, req.DeviceID,
		)
		return
	}

	sessionID := generateSessionID()
	s.mu.Lock()
	s.sessions[sessionID] = &SessionState{
		Driver:  driver,
		Cleanup: cleanup,
	}
	s.mu.Unlock()

	logger.Info("Created session %s for platform=%s", sessionID, req.PlatformName)
	serverTracef(
		"server request worker=%s method=%s path=%s status=%d duration_ms=%d session=%s platform=%s deviceId=%s",
		serverWorkerID(), r.Method, r.URL.Path, http.StatusOK, time.Since(started).Milliseconds(),
		sessionID, req.PlatformName, req.DeviceID,
	)
	writeJSON(w, http.StatusOK, SessionResponse{SessionID: sessionID})
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	sessionID := r.PathValue("id")
	sess, ok := s.getSession(w, r)
	if !ok {
		serverTracef(
			"server execute worker=%s session=%s status=%d duration_ms=%d error=%q",
			serverWorkerID(), sessionID, http.StatusNotFound, time.Since(started).Milliseconds(),
			"session not found",
		)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "failed to read body: " + err.Error()})
		serverTracef(
			"server execute worker=%s session=%s status=%d duration_ms=%d error=%q",
			serverWorkerID(), sessionID, http.StatusBadRequest, time.Since(started).Milliseconds(),
			err.Error(),
		)
		return
	}

	step, err := flow.UnmarshalStep(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid step: " + err.Error()})
		serverTracef(
			"server execute worker=%s session=%s status=%d duration_ms=%d error=%q raw_step=%s",
			serverWorkerID(), sessionID, http.StatusBadRequest, time.Since(started).Milliseconds(),
			err.Error(), trimForLog(string(body), 280),
		)
		return
	}

	serverTracef(
		"server execute request worker=%s session=%s step=%q payload=%s",
		serverWorkerID(), sessionID, stepName(step), trimForLog(string(body), 320),
	)
	result := sess.Driver.Execute(step)
	status := "passed"
	if !result.Success {
		status = "failed"
	}
	serverTracef(
		"server execute response worker=%s session=%s step=%q status=%s duration_ms=%d message=%s",
		serverWorkerID(), sessionID, stepName(step), status, time.Since(started).Milliseconds(),
		trimForLog(result.Message, 220),
	)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.getSession(w, r)
	if !ok {
		return
	}

	png, err := sess.Driver.Screenshot()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "screenshot failed: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(png); err != nil {
		log.Printf("failed to write screenshot: %v", err)
	}
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.getSession(w, r)
	if !ok {
		return
	}

	hierarchy, err := sess.Driver.Hierarchy()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "hierarchy failed: " + err.Error()})
		return
	}

	// Detect content type from the hierarchy bytes
	contentType := "application/xml"
	if len(hierarchy) > 0 && hierarchy[0] == '{' {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(hierarchy); err != nil {
		log.Printf("failed to write hierarchy: %v", err)
	}
}

func (s *Server) handleDeviceInfo(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.getSession(w, r)
	if !ok {
		return
	}

	info := sess.Driver.GetPlatformInfo()
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	id := r.PathValue("id")

	s.mu.Lock()
	sess, exists := s.sessions[id]
	if exists {
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	if !exists {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("session %s not found", id)})
		serverTracef(
			"server request worker=%s method=%s path=%s status=%d duration_ms=%d session=%s error=%q",
			serverWorkerID(), r.Method, r.URL.Path, http.StatusNotFound, time.Since(started).Milliseconds(),
			id, "session not found",
		)
		return
	}

	if sess.Cleanup != nil {
		sess.Cleanup()
	}
	logger.Info("Deleted session %s", id)
	serverTracef(
		"server request worker=%s method=%s path=%s status=%d duration_ms=%d session=%s",
		serverWorkerID(), r.Method, r.URL.Path, http.StatusNoContent, time.Since(started).Milliseconds(), id,
	)
	w.WriteHeader(http.StatusNoContent)
}

// getSession retrieves a session by the {id} path parameter. Returns false if not found.
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) (*SessionState, bool) {
	id := r.PathValue("id")

	s.mu.RLock()
	sess, exists := s.sessions[id]
	s.mu.RUnlock()

	if !exists {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("session %s not found", id)})
		return nil, false
	}
	return sess, true
}

func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback — should never happen
		return fmt.Sprintf("session-%d", len(b))
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

// SanitizePlatform normalizes the platform name.
func SanitizePlatform(p string) string {
	return strings.ToLower(strings.TrimSpace(p))
}

func serverWorkerID() string {
	if worker := strings.TrimSpace(os.Getenv("PYTEST_XDIST_WORKER")); worker != "" {
		return worker
	}
	if worker := strings.TrimSpace(os.Getenv("MAESTRO_WORKER_ID")); worker != "" {
		return worker
	}
	return "master"
}

func trimForLog(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func stepName(step flow.Step) string {
	if step == nil {
		return "unknown"
	}
	return string(step.Type())
}

func serverTracef(format string, v ...interface{}) {
	fmt.Printf("%s [TRACE] %s\n", time.Now().Format("15:04:05.000000"), fmt.Sprintf(format, v...))
}
