# Report JSON Schema v2

Single source of truth with index-based design for real-time updates.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Test Execution                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                      │
│  │ Flow 1   │  │ Flow 2   │  │ Flow 3   │  (parallel)          │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                      │
│       │             │             │                             │
│       ▼             ▼             ▼                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                      │
│  │flow-0.json│ │flow-1.json│ │flow-2.json│  (no lock)          │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                      │
│       │             │             │                             │
│       └─────────────┼─────────────┘                             │
│                     ▼                                           │
│              ┌─────────────┐                                    │
│              │ report.json │  (mutex + atomic write)            │
│              │   (index)   │                                    │
│              └─────────────┘                                    │
└─────────────────────────────────────────────────────────────────┘
                      │
         ┌────────────┼────────────┬────────────┐
         ▼            ▼            ▼            ▼
    ┌────────┐  ┌──────────┐  ┌────────┐  ┌────────┐
    │  HTML  │  │  Allure  │  │ JUnit  │  │Console │
    │ Report │  │  Export  │  │  XML   │  │ Output │
    └────────┘  └──────────┘  └────────┘  └────────┘
```

---

## Directory Structure

```
artifacts/
├── report.json                    # Main index (small, frequently updated)
├── flows/
│   ├── flow-000.json              # Full flow details
│   ├── flow-000-attempt-2.json    # Retry attempt
│   ├── flow-001.json
│   └── flow-002.json
└── assets/
    ├── flow-000/
    │   ├── recording.mp4
    │   ├── cmd-001-after.png
    │   ├── cmd-005-before.png
    │   ├── cmd-005-after.png
    │   ├── cmd-005-hierarchy.xml
    │   └── device.log
    ├── flow-001/
    └── flow-002/
```

---

## report.json (Main Index)

Small file with minimal info. Single source of truth for:
- Run status
- Flow statuses
- Pointers to detail files
- Change tracking (updateSeq)

```json
{
  "version": "1.0.0",
  "updateSeq": 42,
  "status": "running",
  "startTime": "2025-01-15T10:30:00Z",
  "endTime": null,
  "lastUpdated": "2025-01-15T10:31:23.456Z",

  "device": {
    "id": "emulator-5554",
    "name": "Pixel 6 API 34",
    "platform": "android",
    "osVersion": "14",
    "isSimulator": true
  },

  "app": {
    "id": "com.example.app",
    "name": "MyApp",
    "version": "2.1.0"
  },

  "ci": {
    "provider": "github-actions",
    "buildId": "12345",
    "buildUrl": "https://github.com/org/repo/actions/runs/12345",
    "branch": "feature/login",
    "commit": "abc123def"
  },

  "maestroRunner": {
    "version": "0.1.0",
    "driver": "appium"
  },

  "summary": {
    "total": 3,
    "passed": 1,
    "failed": 0,
    "skipped": 0,
    "running": 1,
    "pending": 1
  },

  "flows": [
    {
      "index": 0,
      "id": "flow-000",
      "name": "Login Flow",
      "sourceFile": "flows/login.yaml",
      "dataFile": "flows/flow-000.json",
      "assetsDir": "assets/flow-000",
      "status": "passed",
      "updateSeq": 12,
      "startTime": "2025-01-15T10:30:00Z",
      "endTime": "2025-01-15T10:30:45Z",
      "duration": 45000,
      "lastUpdated": "2025-01-15T10:30:45.123Z",
      "commands": {
        "total": 5,
        "passed": 5,
        "failed": 0,
        "running": 0,
        "pending": 0
      },
      "attempts": 1,
      "error": null
    },
    {
      "index": 1,
      "id": "flow-001",
      "name": "Checkout Flow",
      "sourceFile": "flows/checkout.yaml",
      "dataFile": "flows/flow-001.json",
      "assetsDir": "assets/flow-001",
      "status": "running",
      "updateSeq": 8,
      "startTime": "2025-01-15T10:31:00Z",
      "endTime": null,
      "duration": null,
      "lastUpdated": "2025-01-15T10:31:23.456Z",
      "commands": {
        "total": 8,
        "passed": 3,
        "failed": 0,
        "running": 1,
        "pending": 4,
        "current": 4
      },
      "attempts": 1,
      "error": null
    },
    {
      "index": 2,
      "id": "flow-002",
      "name": "Profile Flow",
      "sourceFile": "flows/profile.yaml",
      "dataFile": "flows/flow-002.json",
      "assetsDir": "assets/flow-002",
      "status": "pending",
      "updateSeq": 0,
      "startTime": null,
      "endTime": null,
      "duration": null,
      "lastUpdated": null,
      "commands": {
        "total": 4,
        "passed": 0,
        "failed": 0,
        "running": 0,
        "pending": 4
      },
      "attempts": 0,
      "error": null
    }
  ]
}
```

---

## flows/flow-XXX.json (Flow Details)

Full command details. Artifacts are **paths only**, never inline.

```json
{
  "id": "flow-000",
  "name": "Login Flow",
  "sourceFile": "flows/login.yaml",
  "tags": ["smoke", "auth"],
  "startTime": "2025-01-15T10:30:00Z",
  "endTime": "2025-01-15T10:30:45Z",
  "duration": 45000,

  "commands": [
    {
      "id": "cmd-000",
      "index": 0,
      "type": "launchApp",
      "yaml": "- launchApp",
      "status": "passed",
      "startTime": "2025-01-15T10:30:00Z",
      "endTime": "2025-01-15T10:30:05Z",
      "duration": 5000,
      "params": {},
      "element": null,
      "error": null,
      "artifacts": {
        "screenshotAfter": "assets/flow-000/cmd-000-after.png"
      }
    },
    {
      "id": "cmd-001",
      "index": 1,
      "type": "tapOn",
      "yaml": "- tapOn:\n    id: \"email_input\"",
      "status": "passed",
      "startTime": "2025-01-15T10:30:05Z",
      "endTime": "2025-01-15T10:30:06Z",
      "duration": 1000,
      "params": {
        "selector": {
          "type": "id",
          "value": "email_input"
        }
      },
      "element": {
        "found": true,
        "id": "email_input",
        "class": "android.widget.EditText",
        "bounds": {"x": 50, "y": 200, "width": 980, "height": 60}
      },
      "error": null,
      "artifacts": {}
    },
    {
      "id": "cmd-002",
      "index": 2,
      "type": "inputText",
      "yaml": "- inputText: \"user@example.com\"",
      "status": "passed",
      "startTime": "2025-01-15T10:30:06Z",
      "endTime": "2025-01-15T10:30:08Z",
      "duration": 2000,
      "params": {
        "text": "user@example.com"
      },
      "element": null,
      "error": null,
      "artifacts": {}
    },
    {
      "id": "cmd-003",
      "index": 3,
      "type": "assertVisible",
      "yaml": "- assertVisible:\n    text: \"Welcome\"",
      "status": "failed",
      "startTime": "2025-01-15T10:30:08Z",
      "endTime": "2025-01-15T10:30:38Z",
      "duration": 30000,
      "params": {
        "selector": {
          "type": "text",
          "value": "Welcome"
        },
        "timeout": 30000
      },
      "element": {
        "found": false
      },
      "error": {
        "type": "element_not_found",
        "message": "Element with text 'Welcome' not found within 30000ms",
        "details": "Waited for element matching: text='Welcome'",
        "suggestion": "Check if the element text changed or if page loaded correctly"
      },
      "artifacts": {
        "screenshotBefore": "assets/flow-000/cmd-003-before.png",
        "screenshotAfter": "assets/flow-000/cmd-003-after.png",
        "viewHierarchy": "assets/flow-000/cmd-003-hierarchy.xml"
      }
    }
  ],

  "artifacts": {
    "video": "assets/flow-000/recording.mp4",
    "videoTimestamps": [
      {"commandIndex": 0, "videoTimeMs": 0},
      {"commandIndex": 1, "videoTimeMs": 5000},
      {"commandIndex": 2, "videoTimeMs": 6000},
      {"commandIndex": 3, "videoTimeMs": 8000}
    ],
    "deviceLog": "assets/flow-000/device.log",
    "appLog": "assets/flow-000/app.log"
  }
}
```

---

## Retry Handling

When a flow retries, create new attempt file:

```
flows/
├── flow-001.json           # Current/latest attempt
├── flow-001-attempt-1.json # First attempt (failed)
└── flow-001-attempt-2.json # Second attempt (passed) = flow-001.json
```

Index tracks attempts:
```json
{
  "id": "flow-001",
  "status": "passed",
  "attempts": 2,
  "attemptHistory": [
    {
      "attempt": 1,
      "dataFile": "flows/flow-001-attempt-1.json",
      "status": "failed",
      "duration": 45000,
      "error": "Element not found"
    },
    {
      "attempt": 2,
      "dataFile": "flows/flow-001-attempt-2.json",
      "status": "passed",
      "duration": 42000,
      "error": null
    }
  ]
}
```

---

## Go Structs

```go
package report

import "time"

// ============================================================================
// INDEX (report.json)
// ============================================================================

type ReportIndex struct {
    Version       string        `json:"version"`
    UpdateSeq     uint64        `json:"updateSeq"`
    Status        Status        `json:"status"`
    StartTime     time.Time     `json:"startTime"`
    EndTime       *time.Time    `json:"endTime,omitempty"`
    LastUpdated   time.Time     `json:"lastUpdated"`
    Device        Device        `json:"device"`
    App           App           `json:"app"`
    CI            *CI           `json:"ci,omitempty"`
    MaestroRunner RunnerInfo    `json:"maestroRunner"`
    Summary       Summary       `json:"summary"`
    Flows         []FlowEntry   `json:"flows"`
}

type Device struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Platform    string `json:"platform"`
    OSVersion   string `json:"osVersion"`
    Model       string `json:"model,omitempty"`
    IsSimulator bool   `json:"isSimulator"`
}

type App struct {
    ID      string `json:"id"`
    Name    string `json:"name,omitempty"`
    Version string `json:"version,omitempty"`
}

type CI struct {
    Provider string `json:"provider"`
    BuildID  string `json:"buildId,omitempty"`
    BuildURL string `json:"buildUrl,omitempty"`
    Branch   string `json:"branch,omitempty"`
    Commit   string `json:"commit,omitempty"`
}

type RunnerInfo struct {
    Version string `json:"version"`
    Driver  string `json:"driver"`
}

type Summary struct {
    Total   int `json:"total"`
    Passed  int `json:"passed"`
    Failed  int `json:"failed"`
    Skipped int `json:"skipped"`
    Running int `json:"running"`
    Pending int `json:"pending"`
}

type FlowEntry struct {
    Index          int              `json:"index"`
    ID             string           `json:"id"`
    Name           string           `json:"name"`
    SourceFile     string           `json:"sourceFile"`
    DataFile       string           `json:"dataFile"`
    AssetsDir      string           `json:"assetsDir"`
    Status         Status           `json:"status"`
    UpdateSeq      uint64           `json:"updateSeq"`
    StartTime      *time.Time       `json:"startTime,omitempty"`
    EndTime        *time.Time       `json:"endTime,omitempty"`
    Duration       *int64           `json:"duration,omitempty"`
    LastUpdated    *time.Time       `json:"lastUpdated,omitempty"`
    Commands       CommandSummary   `json:"commands"`
    Attempts       int              `json:"attempts"`
    AttemptHistory []AttemptEntry   `json:"attemptHistory,omitempty"`
    Error          *string          `json:"error,omitempty"`
}

type CommandSummary struct {
    Total   int  `json:"total"`
    Passed  int  `json:"passed"`
    Failed  int  `json:"failed"`
    Skipped int  `json:"skipped"`
    Running int  `json:"running"`
    Pending int  `json:"pending"`
    Current *int `json:"current,omitempty"`
}

type AttemptEntry struct {
    Attempt  int    `json:"attempt"`
    DataFile string `json:"dataFile"`
    Status   Status `json:"status"`
    Duration int64  `json:"duration"`
    Error    string `json:"error,omitempty"`
}

// ============================================================================
// FLOW DETAIL (flows/flow-XXX.json)
// ============================================================================

type FlowDetail struct {
    ID         string          `json:"id"`
    Name       string          `json:"name"`
    SourceFile string          `json:"sourceFile"`
    Tags       []string        `json:"tags,omitempty"`
    StartTime  time.Time       `json:"startTime"`
    EndTime    *time.Time      `json:"endTime,omitempty"`
    Duration   *int64          `json:"duration,omitempty"`
    Commands   []Command       `json:"commands"`
    Artifacts  FlowArtifacts   `json:"artifacts"`
}

type Command struct {
    ID        string            `json:"id"`
    Index     int               `json:"index"`
    Type      string            `json:"type"`
    YAML      string            `json:"yaml,omitempty"`
    Status    Status            `json:"status"`
    StartTime *time.Time        `json:"startTime,omitempty"`
    EndTime   *time.Time        `json:"endTime,omitempty"`
    Duration  *int64            `json:"duration,omitempty"`
    Params    *CommandParams    `json:"params,omitempty"`
    Element   *Element          `json:"element,omitempty"`
    Error     *Error            `json:"error,omitempty"`
    Artifacts CommandArtifacts  `json:"artifacts"`
}

type Status string

const (
    StatusPending Status = "pending"
    StatusRunning Status = "running"
    StatusPassed  Status = "passed"
    StatusFailed  Status = "failed"
    StatusSkipped Status = "skipped"
)

type CommandParams struct {
    Selector  *Selector `json:"selector,omitempty"`
    Text      string    `json:"text,omitempty"`
    Direction string    `json:"direction,omitempty"`
    Timeout   int       `json:"timeout,omitempty"`
}

type Selector struct {
    Type     string `json:"type"`
    Value    string `json:"value"`
    Optional bool   `json:"optional,omitempty"`
}

type Element struct {
    Found  bool    `json:"found"`
    ID     string  `json:"id,omitempty"`
    Text   string  `json:"text,omitempty"`
    Class  string  `json:"class,omitempty"`
    Bounds *Bounds `json:"bounds,omitempty"`
}

type Bounds struct {
    X      int `json:"x"`
    Y      int `json:"y"`
    Width  int `json:"width"`
    Height int `json:"height"`
}

type Error struct {
    Type       string `json:"type"`
    Message    string `json:"message"`
    Details    string `json:"details,omitempty"`
    Suggestion string `json:"suggestion,omitempty"`
}

// ============================================================================
// ARTIFACTS (paths only, never inline data)
// ============================================================================

type FlowArtifacts struct {
    Video           string           `json:"video,omitempty"`
    VideoTimestamps []VideoTimestamp `json:"videoTimestamps,omitempty"`
    DeviceLog       string           `json:"deviceLog,omitempty"`
    AppLog          string           `json:"appLog,omitempty"`
}

type VideoTimestamp struct {
    CommandIndex int   `json:"commandIndex"`
    VideoTimeMs  int64 `json:"videoTimeMs"`
}

type CommandArtifacts struct {
    ScreenshotBefore string `json:"screenshotBefore,omitempty"`
    ScreenshotAfter  string `json:"screenshotAfter,omitempty"`
    ViewHierarchy    string `json:"viewHierarchy,omitempty"`
}
```

---

## Write Operations

### Write Order (Critical)

```
1. Write asset file (screenshot/video)     → assets/flow-000/cmd-001-after.png
2. Write flow detail file (atomic)         → flows/flow-000.json.tmp → flows/flow-000.json
3. Update index (mutex + atomic)           → report.json.tmp → report.json
```

### Atomic Write Helper

```go
package report

import (
    "encoding/json"
    "os"
    "path/filepath"
    "runtime"
)

func atomicWriteJSON(path string, v interface{}) error {
    data, err := json.MarshalIndent(v, "", "  ")
    if err != nil {
        return err
    }

    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }

    tmpPath := path + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }

    if runtime.GOOS == "windows" {
        os.Remove(path) // Windows requires this
    }

    return os.Rename(tmpPath, path)
}
```

### Index Writer (Thread-Safe)

```go
package report

import (
    "sync"
    "time"
)

type IndexWriter struct {
    mu        sync.Mutex
    path      string
    index     *ReportIndex

    // Debouncing
    pending   map[string]*FlowUpdate
    timer     *time.Timer
    immediate chan struct{}
}

type FlowUpdate struct {
    Status      Status
    StartTime   *time.Time
    EndTime     *time.Time
    Duration    *int64
    Commands    CommandSummary
    Error       *string
}

func NewIndexWriter(path string, index *ReportIndex) *IndexWriter {
    w := &IndexWriter{
        path:      path,
        index:     index,
        pending:   make(map[string]*FlowUpdate),
        immediate: make(chan struct{}, 1),
    }
    go w.flushLoop()
    return w
}

func (w *IndexWriter) UpdateFlow(flowID string, update *FlowUpdate) {
    w.mu.Lock()
    defer w.mu.Unlock()

    w.pending[flowID] = update

    // Immediate flush for terminal states
    if update.Status == StatusFailed || update.Status == StatusPassed {
        select {
        case w.immediate <- struct{}{}:
        default:
        }
        return
    }

    // Debounced flush for progress
    if w.timer == nil {
        w.timer = time.AfterFunc(100*time.Millisecond, func() {
            w.flush()
        })
    }
}

func (w *IndexWriter) flushLoop() {
    for range w.immediate {
        w.flush()
    }
}

func (w *IndexWriter) flush() {
    w.mu.Lock()
    defer w.mu.Unlock()

    if len(w.pending) == 0 {
        return
    }

    // Apply pending updates
    for flowID, update := range w.pending {
        w.applyUpdate(flowID, update)
    }
    w.pending = make(map[string]*FlowUpdate)

    // Update metadata
    w.index.UpdateSeq++
    w.index.LastUpdated = time.Now()
    w.index.Summary = w.computeSummary()

    // Atomic write
    atomicWriteJSON(w.path, w.index)

    // Reset timer
    if w.timer != nil {
        w.timer.Stop()
        w.timer = nil
    }
}

func (w *IndexWriter) applyUpdate(flowID string, update *FlowUpdate) {
    for i := range w.index.Flows {
        if w.index.Flows[i].ID == flowID {
            f := &w.index.Flows[i]
            f.Status = update.Status
            f.StartTime = update.StartTime
            f.EndTime = update.EndTime
            f.Duration = update.Duration
            f.Commands = update.Commands
            f.Error = update.Error
            f.UpdateSeq++
            now := time.Now()
            f.LastUpdated = &now
            break
        }
    }
}

func (w *IndexWriter) computeSummary() Summary {
    var s Summary
    for _, f := range w.index.Flows {
        s.Total++
        switch f.Status {
        case StatusPassed:
            s.Passed++
        case StatusFailed:
            s.Failed++
        case StatusSkipped:
            s.Skipped++
        case StatusRunning:
            s.Running++
        case StatusPending:
            s.Pending++
        }
    }
    return s
}

func (w *IndexWriter) Close() {
    close(w.immediate)
    w.flush() // Final flush
}
```

### Flow Writer (Per-Goroutine, No Lock)

```go
package report

import (
    "os"
    "path/filepath"
    "time"
)

type FlowWriter struct {
    flow      *FlowDetail
    path      string
    assetsDir string
    index     *IndexWriter
}

func NewFlowWriter(flowID, name, sourceFile, outputDir string, index *IndexWriter) *FlowWriter {
    assetsDir := filepath.Join(outputDir, "assets", flowID)
    os.MkdirAll(assetsDir, 0755)

    return &FlowWriter{
        flow: &FlowDetail{
            ID:         flowID,
            Name:       name,
            SourceFile: sourceFile,
            Commands:   []Command{},
            Artifacts:  FlowArtifacts{},
        },
        path:      filepath.Join(outputDir, "flows", flowID+".json"),
        assetsDir: assetsDir,
        index:     index,
    }
}

func (w *FlowWriter) Start() {
    now := time.Now()
    w.flow.StartTime = now
    w.flush()
    w.index.UpdateFlow(w.flow.ID, &FlowUpdate{
        Status:    StatusRunning,
        StartTime: &now,
    })
}

func (w *FlowWriter) CommandStart(cmd *Command) {
    now := time.Now()
    cmd.Status = StatusRunning
    cmd.StartTime = &now
    w.flow.Commands = append(w.flow.Commands, *cmd)
    w.flush()
    w.updateIndex()
}

func (w *FlowWriter) CommandEnd(cmdIndex int, status Status, duration int64, err *Error, artifacts CommandArtifacts) {
    now := time.Now()
    cmd := &w.flow.Commands[cmdIndex]
    cmd.Status = status
    cmd.EndTime = &now
    cmd.Duration = &duration
    cmd.Error = err
    cmd.Artifacts = artifacts
    w.flush()
    w.updateIndex()
}

func (w *FlowWriter) End(status Status) {
    now := time.Now()
    w.flow.EndTime = &now
    duration := now.Sub(w.flow.StartTime).Milliseconds()
    w.flow.Duration = &duration
    w.flush()

    w.index.UpdateFlow(w.flow.ID, &FlowUpdate{
        Status:   status,
        EndTime:  &now,
        Duration: &duration,
        Commands: w.commandSummary(),
    })
}

func (w *FlowWriter) SaveScreenshot(cmdIndex int, timing string, data []byte) string {
    filename := fmt.Sprintf("cmd-%03d-%s.png", cmdIndex, timing)
    path := filepath.Join(w.assetsDir, filename)
    os.WriteFile(path, data, 0644)
    return filepath.Join("assets", w.flow.ID, filename)
}

func (w *FlowWriter) flush() {
    atomicWriteJSON(w.path, w.flow)
}

func (w *FlowWriter) updateIndex() {
    w.index.UpdateFlow(w.flow.ID, &FlowUpdate{
        Status:   StatusRunning,
        Commands: w.commandSummary(),
    })
}

func (w *FlowWriter) commandSummary() CommandSummary {
    var s CommandSummary
    s.Total = len(w.flow.Commands)
    for i, c := range w.flow.Commands {
        switch c.Status {
        case StatusPassed:
            s.Passed++
        case StatusFailed:
            s.Failed++
        case StatusSkipped:
            s.Skipped++
        case StatusRunning:
            s.Running++
            s.Current = &i
        case StatusPending:
            s.Pending++
        }
    }
    return s
}
```

---

## Consumer (HTML Generator)

```go
package report

type Consumer struct {
    reportDir     string
    lastGlobalSeq uint64
    lastFlowSeq   map[string]uint64
}

func NewConsumer(reportDir string) *Consumer {
    return &Consumer{
        reportDir:   reportDir,
        lastFlowSeq: make(map[string]uint64),
    }
}

// Poll checks for changes, returns list of changed flow IDs
func (c *Consumer) Poll() (changed []string, index *ReportIndex) {
    index, err := c.ReadIndex()
    if err != nil {
        return nil, nil
    }

    // No global changes
    if index.UpdateSeq <= c.lastGlobalSeq {
        return nil, index
    }
    c.lastGlobalSeq = index.UpdateSeq

    // Find changed flows
    for _, f := range index.Flows {
        if f.UpdateSeq > c.lastFlowSeq[f.ID] {
            changed = append(changed, f.ID)
            c.lastFlowSeq[f.ID] = f.UpdateSeq
        }
    }

    return changed, index
}

func (c *Consumer) ReadIndex() (*ReportIndex, error) {
    path := filepath.Join(c.reportDir, "report.json")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var index ReportIndex
    if err := json.Unmarshal(data, &index); err != nil {
        return nil, err
    }
    return &index, nil
}

func (c *Consumer) ReadFlow(flowID string) (*FlowDetail, error) {
    path := filepath.Join(c.reportDir, "flows", flowID+".json")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var flow FlowDetail
    if err := json.Unmarshal(data, &flow); err != nil {
        return nil, err
    }
    return &flow, nil
}
```

---

## Output Generators

From this structure, generate:

| Format | Input | Output |
|--------|-------|--------|
| HTML | report.json + flows/*.json | Single HTML file |
| Allure | flows/*.json | allure-results/*.json |
| JUnit | report.json | junit.xml |
| Console | report.json | STDOUT |

---

## Recovery

On startup, recover from incomplete state:

```go
func Recover(reportDir string) error {
    index, err := ReadIndex(reportDir)
    if err != nil {
        return err
    }

    changed := false
    for i := range index.Flows {
        f := &index.Flows[i]
        if f.Status == StatusRunning {
            // Check flow file for actual state
            flow, err := ReadFlow(reportDir, f.ID)
            if err != nil {
                f.Status = StatusFailed
                errMsg := "Flow interrupted"
                f.Error = &errMsg
                changed = true
                continue
            }

            // Infer status from commands
            status := inferStatus(flow.Commands)
            if status != StatusRunning {
                f.Status = status
                changed = true
            }
        }
    }

    if changed {
        atomicWriteJSON(filepath.Join(reportDir, "report.json"), index)
    }

    return nil
}

func inferStatus(commands []Command) Status {
    allPassed := true
    for _, c := range commands {
        if c.Status == StatusFailed {
            return StatusFailed
        }
        if c.Status != StatusPassed {
            allPassed = false
        }
    }
    if allPassed && len(commands) > 0 {
        return StatusPassed
    }
    return StatusFailed // Incomplete = failed
}
```
