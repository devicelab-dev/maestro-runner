package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// A test run holds its device, so `maestro-runner hierarchy` used to refuse
// it ("device ... is already in use"). Maestro allows `maestro hierarchy`
// next to a running test, and tools rely on that: Expo's image-comparison
// server asks for the hierarchy mid-flow to crop a view. So a test run also
// answers hierarchy and screenshot requests for its device on a local socket,
// through the driver it already has: no second connection to the device, and
// nothing about the run changes.

// hierarchyShareDir is where the sockets live, next to the device lock
// sockets (a variable for tests).
var hierarchyShareDir = "/tmp"

func hierarchySharePath(serial string) string {
	return filepath.Join(hierarchyShareDir, "maestro-runner-hierarchy-"+serial+".sock")
}

type hierarchyShareRequest struct {
	Kind string `json:"kind"` // "hierarchy" or "screenshot"
}

type hierarchyShareResponse struct {
	Data  []byte `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// shareHierarchy serves hierarchy and screenshot requests for serial from
// driver until the returned stop func is called. A failure to listen is only
// logged: the run itself does not depend on it.
func shareHierarchy(serial string, driver core.Driver) (stop func()) {
	if runtime.GOOS == "windows" || serial == "" || driver == nil {
		return func() {}
	}
	path := hierarchySharePath(serial)
	if device.IsOwnerAlive(path) {
		// Another live run already answers for this device.
		return func() {}
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		logger.Debug("hierarchy share for %s: %v", serial, err)
		return func() {}
	}
	pidPath := sharePidPath(path)
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644)

	// One request at a time: a hierarchy read is short, and the driver's
	// own commands keep running alongside it.
	var mu sync.Mutex
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(60 * time.Second))
				var req hierarchyShareRequest
				if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
					return
				}
				mu.Lock()
				resp := answerShareRequest(driver, req)
				mu.Unlock()
				_ = json.NewEncoder(c).Encode(resp)
			}(conn)
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = ln.Close()
			_ = os.Remove(path)
			_ = os.Remove(pidPath)
		})
	}
}

func answerShareRequest(driver core.Driver, req hierarchyShareRequest) hierarchyShareResponse {
	var data []byte
	var err error
	switch req.Kind {
	case "hierarchy":
		data, err = driver.Hierarchy()
	case "screenshot":
		data, err = driver.Screenshot()
	default:
		err = fmt.Errorf("unknown request %q", req.Kind)
	}
	if err != nil {
		return hierarchyShareResponse{Error: err.Error()}
	}
	return hierarchyShareResponse{Data: data}
}

// sharePidPath is the owner file next to a share socket, named the way the
// device lock's is (see device.IsOwnerAlive).
func sharePidPath(socketPath string) string {
	return strings.TrimSuffix(socketPath, filepath.Ext(socketPath)) + ".pid"
}

// liveHierarchyShare finds the share socket of a running test: the one for
// serial, or, with no serial, the only live one. "" when there is none.
func liveHierarchyShare(serial string) string {
	if serial != "" {
		if path := hierarchySharePath(serial); device.IsOwnerAlive(path) {
			return path
		}
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(hierarchyShareDir, "maestro-runner-hierarchy-*.sock"))
	var live []string
	for _, m := range matches {
		if device.IsOwnerAlive(m) {
			live = append(live, m)
		}
	}
	if len(live) == 1 {
		return live[0]
	}
	return ""
}

// askHierarchyShare sends one request to a running test's share socket.
func askHierarchyShare(path, kind string) ([]byte, error) {
	conn, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(90 * time.Second))
	if err := json.NewEncoder(conn).Encode(hierarchyShareRequest{Kind: kind}); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(conn)
	if err != nil {
		return nil, err
	}
	var resp hierarchyShareResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("read %s from the running test: %w", kind, err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s from the running test: %s", kind, resp.Error)
	}
	return resp.Data, nil
}
