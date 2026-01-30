# MCP Server Implementation Plan for maestro-runner

## Overview

Add Model Context Protocol (MCP) support to maestro-runner, enabling AI agents (Claude, etc.) to drive mobile tests directly via JSON-RPC 2.0 over stdio.

**Goal:** Allow AI to interact with Android/iOS devices using natural language, leveraging maestro-runner's smart selectors, actionability waits, and YAML flow execution.

---

## Research Summary

### What is MCP?
- Open standard by Anthropic (donated to Linux Foundation)
- JSON-RPC 2.0 protocol over stdio/HTTP
- Adopted by: Anthropic, OpenAI, Google, Microsoft, AWS
- Spec version: `2025-11-25`

### Existing Mobile MCP Implementations Analyzed
| Project | Approach | Learnings |
|---------|----------|-----------|
| mobile-mcp | Native (ADB + UIAutomator) | Prerequisite validation, error classification, retry patterns |
| appium-mcp | Appium-based (TypeScript) | Inspector tools, test generation, session wrapper, FastMCP framework |
| clicker | Browser automation | Clean MCP server structure, tool schemas |

### Deep Dive: appium-mcp (AppVitals)

**Source:** `/Users/omnarayan/work/temp/AppVitals/appium-mcp` (~9,125 lines TypeScript)

#### Architecture Patterns Worth Adopting

| Pattern | What They Do | Our Adaptation |
|---------|--------------|----------------|
| **FastMCP Framework** | Uses `fastmcp` library instead of raw JSON-RPC | Use Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`) |
| **Tool Middleware** | Wraps all tools with logging, timing, sensitive data redaction | Create `toolWrapper()` function in Go |
| **Session Lock** | `isDeletingSession` flag prevents concurrent deletion | Add `sync.Mutex` + deletion flag |
| **Dependency Injection** | Tools accept deps interface for testability | Use interfaces for driver/device |
| **Graceful Error Returns** | Return errors in response content, not throw | Return `ToolResult` with `isError: true` |
| **Singleton Managers** | Lazy initialization with promise/mutex caching | Use `sync.Once` for device manager |
| **Tool Annotations** | `readOnlyHint`, `destructiveHint`, `openWorldHint` | Add hints to tool schemas |

#### Tool Middleware Pattern (Key Learning)

```typescript
// appium-mcp wraps every tool with this middleware
function wrapTool(tool) {
  return async (args, context) => {
    const start = Date.now();
    log.info(`[${tool.name}] Starting...`);

    // Redact sensitive fields (passwords, tokens, secrets)
    const safeArgs = redactSensitive(args);
    log.debug(`[${tool.name}] Args:`, safeArgs);

    try {
      const result = await tool.execute(args, context);
      log.info(`[${tool.name}] Completed in ${Date.now() - start}ms`);
      return result;
    } catch (err) {
      log.error(`[${tool.name}] Failed:`, err.stack);
      // Return error in content, don't throw
      return {
        content: [{ type: 'text', text: `Failed: ${err.message}` }],
        isError: true,
      };
    }
  };
}
```

**Go Implementation:**
```go
// pkg/mcp/middleware.go

func WrapTool(name string, handler ToolHandler) ToolHandler {
    return func(args map[string]any) (*ToolResult, error) {
        start := time.Now()
        log.Info().Str("tool", name).Msg("Starting")

        // Redact sensitive fields
        safeArgs := redactSensitive(args)
        log.Debug().Str("tool", name).Interface("args", safeArgs).Msg("Args")

        result, err := handler(args)
        duration := time.Since(start)

        if err != nil {
            log.Error().Str("tool", name).Err(err).Dur("duration", duration).Msg("Failed")
            return &ToolResult{
                Content: []Content{{Type: "text", Text: fmt.Sprintf("Failed: %s", err)}},
                IsError: true,
            }, nil // Return error in content, not as error
        }

        log.Info().Str("tool", name).Dur("duration", duration).Msg("Completed")
        return result, nil
    }
}
```

#### Session Management Pattern

```typescript
// appium-mcp: Singleton with deletion lock
let driver: any = null;
let sessionId: string | null = null;
let isDeletingSession = false; // Prevents concurrent deletion

export async function safeDeleteSession(): Promise<boolean> {
  if (isDeletingSession) return false; // Already in progress
  isDeletingSession = true;
  try {
    await driver.deleteSession();
    driver = null;
    sessionId = null;
    return true;
  } finally {
    isDeletingSession = false;
  }
}
```

**Go Implementation:**
```go
// pkg/mcp/session.go

type Session struct {
    Device      *device.AndroidDevice
    Client      *uiautomator2.Client
    Driver      *uiautomator2.Driver
    AppID       string
    StartedAt   time.Time
    mu          sync.Mutex
    isDeleting  bool  // Deletion lock
}

func (s *Session) SafeDelete() (bool, error) {
    s.mu.Lock()
    if s.isDeleting {
        s.mu.Unlock()
        return false, nil // Already deleting
    }
    s.isDeleting = true
    s.mu.Unlock()

    defer func() {
        s.mu.Lock()
        s.isDeleting = false
        s.mu.Unlock()
    }()

    // Cleanup logic
    if s.Driver != nil {
        s.Driver.DeleteSession()
    }
    s.Client = nil
    s.Driver = nil
    return true, nil
}
```

#### Device Manager Singleton

```typescript
// appium-mcp: Lazy init with promise caching
class ADBManager {
  private static instance: ADBManager | null = null;
  private initPromise: Promise<void> | null = null;

  static async getInstance(): Promise<ADBManager> {
    if (!this.instance) {
      this.instance = new ADBManager();
    }
    if (this.instance.initPromise) {
      await this.instance.initPromise; // Wait for ongoing init
    }
    return this.instance;
  }
}
```

**Go Implementation:**
```go
// pkg/mcp/device_manager.go

var (
    deviceManager *DeviceManager
    managerOnce   sync.Once
)

func GetDeviceManager() *DeviceManager {
    managerOnce.Do(func() {
        deviceManager = &DeviceManager{}
        deviceManager.init()
    })
    return deviceManager
}
```

#### Tool Annotations (New MCP Feature)

appium-mcp uses annotations to hint tool behavior to AI:

```typescript
server.addTool({
  name: 'appium_screenshot',
  annotations: {
    readOnlyHint: true,      // Doesn't modify state
    openWorldHint: false,    // No external knowledge needed
  },
  // ...
});

server.addTool({
  name: 'appium_delete_session',
  annotations: {
    destructiveHint: true,   // Destructive operation
    readOnlyHint: false,
  },
  // ...
});
```

**Apply to our tools:**
| Tool | readOnlyHint | destructiveHint | openWorldHint |
|------|--------------|-----------------|---------------|
| screenshot | true | false | false |
| hierarchy | true | false | false |
| tap | false | false | false |
| app_clear | false | true | false |
| server_stop | false | true | false |
| run_flow | false | false | true |

#### Smart Locator Priority (Platform-Specific)

appium-mcp generates locators with platform-specific priority:

```typescript
// Android (UIAutomator2) priority:
const androidPriority = ['id', 'accessibility-id', 'xpath', 'uiautomator', 'class-name'];

// iOS (XCUITest) priority:
const iosPriority = ['id', 'accessibility-id', 'predicate-string', 'class-chain', 'xpath'];
```

**Our selector priority:**
```go
// pkg/mcp/locators.go

var SelectorPriority = []string{
    "text",           // Most readable
    "id",             // Most stable
    "contentDesc",    // Accessibility
    "hint",           // Hint text
    "class",          // Last resort
}
```

#### File Organization

```
appium-mcp/src/
├── index.ts              # CLI entry
├── server.ts             # FastMCP setup
├── session-store.ts      # Global state
├── logger.ts             # Logging wrapper
├── tools/                # Tool implementations
│   ├── index.ts          # Central registry
│   ├── session/          # Session tools
│   ├── interactions/     # tap, type, swipe
│   ├── navigation/       # scroll, back
│   ├── app-management/   # launch, stop
│   └── screen/           # screenshot, hierarchy
├── resources/            # MCP resources
├── devicemanager/        # ADB/iOS singletons
├── locators/             # Element finding
└── ui/                   # MCP-UI integration
```

#### MCP-UI Integration (Future Enhancement)

appium-mcp uses `ui://` scheme for interactive HTML components:
- Device picker with click handlers
- Screenshot viewer
- Page source inspector
- Locator generator UI

**Consider for v2:** Add interactive UI for device selection and element inspection.

### Deep Dive: Playwright MCP (Microsoft)

**Source:** [github.com/microsoft/playwright-mcp](https://github.com/microsoft/playwright-mcp)

Playwright's MCP server is the gold standard for browser automation via AI. Key architectural decisions we should adopt:

#### Core Design Philosophy

> "Uses Playwright's accessibility tree, not pixel-based input"

This is the most important insight: **structured semantic data beats screenshots** for LLM interaction. The AI works with element roles, names, and relationships instead of pixel coordinates.

**Why this matters for mobile:**
- UI hierarchy (XML) is our equivalent of accessibility tree
- Faster than screenshot + vision model
- Deterministic element identification
- No ambiguity in element selection

#### Tool Categories (40+ tools)

| Category | Tools | Our Equivalent |
|----------|-------|----------------|
| **Core Automation** | click, type, navigate, fill_form, drag, hover | tap, type, swipe, scroll |
| **Snapshots** | browser_snapshot (accessibility tree) | `hierarchy` (UI tree) |
| **Assertions** | verify_element_visible, verify_text_visible, verify_value | assert_visible, assert_not_visible |
| **Tab/Window** | tabs (list, create, close, select) | N/A (single app focus) |
| **Testing** | generate_locator, verify_list_visible | get_locators, generate_flow |
| **Advanced** | evaluate (JS), console_messages, network_requests | evalScript, run_flow |

#### Snapshot Tool (Key Pattern)

```json
{
  "name": "browser_snapshot",
  "description": "Capture accessibility snapshot of the current page, this is better than screenshot",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```

**Returns structured data:**
```json
{
  "elements": [
    { "role": "button", "name": "Login", "ref": "e1" },
    { "role": "textbox", "name": "Email", "ref": "e2" }
  ]
}
```

**Our equivalent - `hierarchy` tool enhancement:**
```json
{
  "name": "hierarchy",
  "description": "Get UI hierarchy (element tree) - better than screenshot for element finding",
  "inputSchema": {
    "properties": {
      "format": { "enum": ["json", "xml"], "default": "json" },
      "filtered": { "type": "boolean", "default": true, "description": "Only interactive elements" },
      "with_refs": { "type": "boolean", "default": true, "description": "Include element refs for actions" }
    }
  }
}
```

**Returns:**
```json
{
  "elements": [
    { "ref": "e1", "text": "Login", "id": "com.app:id/login", "class": "Button", "bounds": {...} },
    { "ref": "e2", "hint": "Email", "id": "com.app:id/email", "class": "EditText", "bounds": {...} }
  ]
}
```

#### Element Reference Pattern

Playwright uses `ref` parameter to reference elements from snapshots:

```json
// Step 1: Get snapshot
{ "tool": "browser_snapshot" }
// Returns: { "elements": [{ "ref": "e1", "name": "Login" }] }

// Step 2: Click using ref
{ "tool": "browser_click", "params": { "element": "e1" } }
```

**Adopt for maestro-runner:**
```json
// Step 1: Get hierarchy with refs
{ "tool": "hierarchy", "params": { "with_refs": true } }
// Returns: { "elements": [{ "ref": "e1", "text": "Login" }] }

// Step 2: Tap using ref (faster than selector search)
{ "tool": "tap", "params": { "ref": "e1" } }
// OR traditional selector
{ "tool": "tap", "params": { "selector": "Login" } }
```

#### Opt-in Capabilities

Playwright uses capability flags for advanced features:

```bash
# Enable vision (coordinate-based) tools
--capabilities vision

# Enable PDF generation
--capabilities pdf
```

**Apply to maestro-runner:**
```bash
# Enable advanced tools
maestro-runner mcp --capabilities recording,debug

# Capabilities:
# - recording: Enable action recording for flow generation
# - debug: Enable verbose element info
# - adb: Enable raw ADB command execution
```

#### Incremental Snapshots

Playwright's default mode captures deltas between page states:
- Reduces data transfer to LLM
- Faster responses
- Only changed elements sent

**Consider for v2:** Track UI changes between hierarchy calls.

#### Playwright Test Agents (Agentic Pattern)

Playwright provides three autonomous agents:

| Agent | Purpose | Our Equivalent |
|-------|---------|----------------|
| **🎭 Planner** | Explores app, generates markdown test plans | MCP Prompt: `create_test` |
| **🎭 Generator** | Converts plans to executable tests | Tool: `generate_flow` |
| **🎭 Healer** | Runs tests, auto-repairs failures | Self-healing selector system |

**Healer workflow (adopt this pattern):**
1. Run test → Step fails
2. Capture screenshot + hierarchy
3. Inspect UI to find equivalent element
4. Suggest patch (locator update, wait adjustment)
5. Re-run test
6. Loop until pass or guardrail stops

```go
// pkg/mcp/tools/healer.go (Future)

type HealerResult struct {
    OriginalSelector string
    NewSelector      string
    Confidence       float64
    Reasoning        string
}

func HealFailedStep(intent string, screenshot []byte, hierarchy string) (*HealerResult, error) {
    // 1. Analyze hierarchy for matching elements
    // 2. Score candidates by intent similarity
    // 3. Return best match with confidence
}
```

### Deep Dive: mobile-next/mobile-mcp

**Source:** [github.com/mobile-next/mobile-mcp](https://github.com/mobile-next/mobile-mcp) (3,070+ stars)

The most popular mobile MCP server with cross-platform iOS/Android support.

#### Hybrid Perception Model (Key Innovation)

> "Uses native accessibility trees for most interactions, or screenshot-based coordinates where a11y labels are not available."

**Fallback chain:**
1. Structured accessibility snapshots (primary) - fast, deterministic
2. Screenshot-based coordinate analysis (fallback) - when a11y unavailable
3. Visual analysis optional - no CV model required in snapshot mode

**Why this matters:** Games and custom views often lack accessibility labels. Having a fallback to screenshot + coordinates ensures 100% coverage.

#### Tool Categories

| Category | Tools |
|----------|-------|
| **Device** | `list_available_devices`, `get_screen_size`, `get_orientation`, `set_orientation` |
| **App** | `list_apps`, `launch_app`, `terminate_app`, `install_app`, `uninstall_app` |
| **Screen** | `take_screenshot`, `save_screenshot`, `list_elements_on_screen` |
| **Input** | `click_on_screen_at_coordinates`, `double_tap`, `long_press`, `swipe`, `type_keys` |
| **Navigation** | `press_button` (HOME, BACK, VOLUME), `open_url` |

#### Unique Patterns to Adopt

**1. Unified API across platforms:**
```typescript
// Same tool works for iOS and Android
mobile_launch_app({
  bundleId: "com.example.app",  // iOS
  packageName: "com.example.app" // Android - same param works
})
```

**2. Element list with coordinates:**
```json
{
  "tool": "mobile_list_elements_on_screen",
  "returns": [
    { "label": "Login", "type": "button", "x": 180, "y": 450, "width": 100, "height": 44 },
    { "label": "Email", "type": "textfield", "x": 20, "y": 200, "width": 340, "height": 44 }
  ]
}
```

**3. Hardware button support:**
```json
{ "tool": "mobile_press_button", "params": { "button": "VOLUME_UP" } }
// Buttons: HOME, BACK, VOLUME_UP, VOLUME_DOWN, ENTER
```

### Deep Dive: Other Android MCP Servers

#### nim444/mcp-android-server-python

**Source:** [github.com/nim444/mcp-android-server-python](https://github.com/nim444/mcp-android-server-python)

Uses **uiautomator2** (same as maestro-runner) with modular architecture.

**Key Pattern - Modular Tool Organization:**
```
server.py (61 lines - just registers tools)
tools/
├── device.py      # connect_device, get_device_status, check_adb
├── app.py         # launch, terminate, list_apps
├── screen.py      # screenshot, screen_on/off, smart_unlock
├── input.py       # click, type, swipe, key_press
├── ui.py          # dump_ui (hierarchy), wait_for_element, scroll_to
└── advanced.py    # toast detection, activity monitoring
```

**Refactored from 1,321 lines to 61-line server** - tools in separate modules.

**Advanced Features Worth Adopting:**
| Feature | Description |
|---------|-------------|
| `smart_unlock` | screen_on + swipe gesture combo |
| `wait_for_element` | Poll until element appears |
| `scroll_to_element` | Auto-scroll to find obscured elements |
| `toast_detection` | Capture Android toast messages |
| `activity_monitoring` | Wait for specific Activity transitions |

**Two Transport Modes:**
1. **stdio** - Standard for Claude Desktop
2. **HTTP (port 8080)** - For web apps, API testing, CI/CD

#### CursorTouch/Android-MCP

**Source:** [github.com/CursorTouch/Android-MCP](https://github.com/CursorTouch/Android-MCP)

Lightweight Python MCP with 11 tools.

**Unique Tool - Combined State Snapshot:**
```json
{
  "tool": "State-Tool",
  "description": "Combined snapshot of active apps and interactive UI elements",
  "returns": {
    "foreground_app": "com.example.app",
    "elements": [
      { "text": "Login", "bounds": [100, 200, 200, 250], "clickable": true }
    ]
  }
}
```

**Performance Note:** 2-4s latency between actions (device-dependent).

#### erichung9060/Android-Mobile-MCP

**Source:** [github.com/erichung9060/Android-Mobile-MCP](https://github.com/erichung9060/Android-Mobile-MCP)

**Key Pattern - UI Dump with Center Coordinates:**
```json
{
  "tool": "mobile_dump_ui",
  "returns": [
    {
      "text": "Login",
      "bounds": [100, 200, 200, 250],
      "center": [150, 225],  // Pre-calculated for easy clicking
      "clickable": true
    }
  ]
}
```

Pre-calculating center coordinates saves AI from doing math.

### Deep Dive: iOS-Specific MCP

#### iHackSubhodip iOS Automation MCP

**Source:** [skywork.ai article](https://skywork.ai/skypage/en/agentic-shift-mobile-testing-ios-automation/1981253016629465088)

Uses **FastMCP 2.0 + Appium + XCUITest**.

**Architecture Pattern - Platform Segregation:**
```
src/
├── shared/           # Cross-platform utilities
├── platforms/
│   ├── ios/          # iOS-specific implementation
│   └── android/      # Android (future)
└── server.py         # MCP server entry
```

**Tools:**
- `launch_app` - Bundle-based app launching
- `take_screenshot` - Visual context capture
- `find_and_tap` - Accessibility-based element interaction
- `appium_tap_and_type` - Text input automation
- `list_simulators` / `get_server_status` - Infrastructure management

### Deep Dive: AI Agent Frameworks

#### minitap-ai/mobile-use

**Source:** [github.com/minitap-ai/mobile-use](https://github.com/minitap-ai/mobile-use)

Full AI agent (not just MCP server) that controls Android/iOS via natural language.

**Key Insight - Limitations:**
> "Limited effectiveness with games since games typically don't expose accessibility tree data."

**Agentic Graph Architecture:**
- Uses LangGraph for workflow orchestration
- Multiple specialized agent nodes
- Structured data extraction (JSON output)
- Multi-model support (OpenAI, Vertex AI, etc.)

### Summary: Best Patterns Across All Projects

| Pattern | Source | Apply to maestro-runner |
|---------|--------|-------------------------|
| **Hybrid perception** (a11y + screenshot fallback) | mobile-mcp | Add coordinate-based tap for games |
| **Pre-calculated center coords** | Android-Mobile-MCP | Include in hierarchy response |
| **Combined state snapshot** | Android-MCP | Single tool for app + UI state |
| **Modular tool organization** | mcp-android-server-python | Already planned |
| **Two transport modes** (stdio + HTTP) | mcp-android-server-python | Add REST API |
| **Smart unlock** | mcp-android-server-python | Add to device tools |
| **Toast detection** | mcp-android-server-python | Add to screen tools |
| **Activity monitoring** | mcp-android-server-python | Add wait_for_activity |
| **Platform segregation** | iOS MCP | Future iOS support |

### maestro-runner Advantages
- No Appium dependency (native UIAutomator2) - faster than all Appium-based MCPs
- Rich selector syntax (text, ID, relative positioning)
- Built-in actionability/smart waits
- YAML flow execution (`run_flow` tool) - unique capability
- HTML/JSON report generation

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      AI Agent (Claude)                       │
└─────────────────────────────┬───────────────────────────────┘
                              │ JSON-RPC 2.0 (stdio)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    MCP Server (pkg/mcp/)                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   server.go │  │  session.go │  │      tools/         │  │
│  │  (JSON-RPC) │  │   (state)   │  │  device.go, app.go  │  │
│  │             │  │             │  │  input.go, screen.go│  │
│  │             │  │             │  │  inspector.go       │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────┬───────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│               Existing maestro-runner Core                   │
│  ┌───────────────┐  ┌────────────────┐  ┌────────────────┐  │
│  │ uiautomator2  │  │     driver     │  │    executor    │  │
│  │    client     │  │  (commands)    │  │  (run flows)   │  │
│  └───────────────┘  └────────────────┘  └────────────────┘  │
└─────────────────────────────┬───────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Device (via ADB)                          │
│                  UIAutomator2 Server                         │
└─────────────────────────────────────────────────────────────┘
```

---

## File Structure

```
pkg/mcp/
├── server.go           # MCP server using Go SDK, lifecycle handling
├── session.go          # Session state management (singleton + deletion lock)
├── errors.go           # Custom error types (ActionableError)
├── middleware.go       # Tool wrapper: logging, timing, sensitive redaction
├── device_manager.go   # Device singleton with sync.Once
├── resources.go        # MCP Resources (documentation for AI)
├── prompts.go          # MCP Prompts (templates for AI)
├── locators.go         # Selector priority and generation
├── tools/
│   ├── registry.go     # Central tool registration (all tools registered here)
│   ├── device.go       # server_start, server_stop
│   ├── app.go          # app_launch, app_stop, app_clear
│   ├── input.go        # tap, type, swipe, scroll, back
│   ├── screen.go       # screenshot, hierarchy, orientation
│   ├── assert.go       # assert_visible, assert_not_visible
│   ├── inspector.go    # get_locators, generate_flow, get_test_template
│   └── flow.go         # run_flow, validate_flow
└── schema.go           # Tool schemas with annotations (JSON Schema)

pkg/rest/
├── server.go           # REST API server
├── handlers.go         # HTTP handlers (reuse MCP tool logic)
└── middleware.go       # Auth, logging, CORS
```

---

## Tool Specifications

### Category 1: Device/Server Management

#### `server_start`
Start UIAutomator2 server and create session.

```json
{
  "name": "server_start",
  "description": "Start UIAutomator2 server on device and create automation session",
  "inputSchema": {
    "type": "object",
    "properties": {
      "device_id": {
        "type": "string",
        "description": "Device serial (optional, uses first available)"
      },
      "app_id": {
        "type": "string",
        "description": "App package to launch after start (optional)"
      }
    }
  }
}
```

**Returns:** `{ "status": "started", "device": "emulator-5554", "session_id": "xxx" }`

#### `server_stop`
Stop server and cleanup session.

```json
{
  "name": "server_stop",
  "description": "Stop UIAutomator2 server and cleanup session",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```

---

### Category 2: App Lifecycle

#### `app_launch`
```json
{
  "name": "app_launch",
  "description": "Launch an app on the device",
  "inputSchema": {
    "type": "object",
    "properties": {
      "app_id": {
        "type": "string",
        "description": "App package name (e.g., com.example.app)"
      },
      "clear_state": {
        "type": "boolean",
        "description": "Clear app data before launch",
        "default": false
      },
      "wait_for_idle": {
        "type": "boolean",
        "description": "Wait for app to become idle",
        "default": true
      }
    },
    "required": ["app_id"]
  }
}
```

#### `app_stop`
```json
{
  "name": "app_stop",
  "description": "Stop/kill an app",
  "inputSchema": {
    "type": "object",
    "properties": {
      "app_id": {
        "type": "string",
        "description": "App package (optional, uses current app)"
      }
    }
  }
}
```

#### `app_clear`
```json
{
  "name": "app_clear",
  "description": "Clear app data and cache",
  "inputSchema": {
    "type": "object",
    "properties": {
      "app_id": {
        "type": "string",
        "description": "App package (optional, uses current app)"
      }
    }
  }
}
```

---

### Category 3: Input/Interaction

#### `tap`
```json
{
  "name": "tap",
  "description": "Tap on an element. Waits for element to be visible and enabled.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Element selector (text, id:resource_id, or desc:content_desc)"
      },
      "timeout": {
        "type": "integer",
        "description": "Timeout in seconds",
        "default": 17
      },
      "optional": {
        "type": "boolean",
        "description": "Don't fail if element not found",
        "default": false
      }
    },
    "required": ["selector"]
  }
}
```

**Returns:** `{ "status": "tapped", "element": { "text": "Login", "bounds": {...} } }`

#### `double_tap`
```json
{
  "name": "double_tap",
  "description": "Double tap on an element",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": { "type": "string" }
    },
    "required": ["selector"]
  }
}
```

#### `long_press`
```json
{
  "name": "long_press",
  "description": "Long press on an element",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": { "type": "string" },
      "duration": {
        "type": "integer",
        "description": "Duration in milliseconds",
        "default": 1000
      }
    },
    "required": ["selector"]
  }
}
```

#### `type`
```json
{
  "name": "type",
  "description": "Type text into an element or focused field",
  "inputSchema": {
    "type": "object",
    "properties": {
      "text": {
        "type": "string",
        "description": "Text to type"
      },
      "selector": {
        "type": "string",
        "description": "Element selector (optional, types into focused field if not provided)"
      },
      "clear_first": {
        "type": "boolean",
        "description": "Clear existing text before typing",
        "default": false
      }
    },
    "required": ["text"]
  }
}
```

#### `swipe`
```json
{
  "name": "swipe",
  "description": "Swipe on screen",
  "inputSchema": {
    "type": "object",
    "properties": {
      "direction": {
        "type": "string",
        "enum": ["up", "down", "left", "right"]
      },
      "percent": {
        "type": "number",
        "description": "Swipe distance as percentage of screen",
        "default": 50
      },
      "speed": {
        "type": "integer",
        "description": "Swipe speed in ms",
        "default": 300
      }
    },
    "required": ["direction"]
  }
}
```

#### `scroll_to`
```json
{
  "name": "scroll_to",
  "description": "Scroll until element is visible",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": { "type": "string" },
      "direction": {
        "type": "string",
        "enum": ["up", "down"],
        "default": "down"
      },
      "max_scrolls": {
        "type": "integer",
        "default": 10
      }
    },
    "required": ["selector"]
  }
}
```

#### `back`
```json
{
  "name": "back",
  "description": "Press the back button",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```

---

### Category 4: Screen/Visual

#### `screenshot`
```json
{
  "name": "screenshot",
  "description": "Capture screenshot of current screen",
  "inputSchema": {
    "type": "object",
    "properties": {
      "filename": {
        "type": "string",
        "description": "Save to file (optional, returns base64 if not provided)"
      },
      "quality": {
        "type": "integer",
        "description": "JPEG quality 1-100",
        "default": 80
      }
    }
  }
}
```

**Returns:** `{ "type": "image", "data": "base64...", "mimeType": "image/png" }`

#### `hierarchy`
```json
{
  "name": "hierarchy",
  "description": "Get UI hierarchy (element tree)",
  "inputSchema": {
    "type": "object",
    "properties": {
      "format": {
        "type": "string",
        "enum": ["json", "xml"],
        "default": "json"
      },
      "filtered": {
        "type": "boolean",
        "description": "Only include interactive elements",
        "default": true
      }
    }
  }
}
```

#### `get_orientation`
```json
{
  "name": "get_orientation",
  "description": "Get current screen orientation",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```

**Returns:** `{ "orientation": "portrait" }`

---

### Category 5: Assertions

#### `assert_visible`
```json
{
  "name": "assert_visible",
  "description": "Assert element is visible on screen",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": { "type": "string" },
      "timeout": {
        "type": "integer",
        "default": 17
      }
    },
    "required": ["selector"]
  }
}
```

#### `assert_not_visible`
```json
{
  "name": "assert_not_visible",
  "description": "Assert element is NOT visible on screen",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": { "type": "string" },
      "timeout": {
        "type": "integer",
        "default": 5
      }
    },
    "required": ["selector"]
  }
}
```

---

### Category 6: Inspector (Unique Value-Add)

#### `get_locators`
```json
{
  "name": "get_locators",
  "description": "Get all possible selectors for an element (for test authoring)",
  "inputSchema": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Initial selector to find element"
      }
    },
    "required": ["selector"]
  }
}
```

**Returns:**
```json
{
  "locators": [
    { "type": "text", "value": "Login", "priority": 1 },
    { "type": "id", "value": "com.app:id/login_btn", "priority": 2 },
    { "type": "contentDesc", "value": "Sign in", "priority": 3 }
  ]
}
```

#### `generate_flow`
```json
{
  "name": "generate_flow",
  "description": "Generate YAML flow from recorded actions",
  "inputSchema": {
    "type": "object",
    "properties": {
      "actions": {
        "type": "array",
        "description": "List of actions performed",
        "items": {
          "type": "object",
          "properties": {
            "type": { "type": "string" },
            "selector": { "type": "string" },
            "text": { "type": "string" }
          }
        }
      },
      "flow_name": {
        "type": "string",
        "description": "Name for the flow"
      }
    },
    "required": ["actions"]
  }
}
```

**Returns:**
```yaml
appId: com.example.app
name: login_flow
---
- tapOn: "Login"
- inputText:
    text: "user@test.com"
- tapOn: "Submit"
- assertVisible: "Welcome"
```

#### `get_test_template`
```json
{
  "name": "get_test_template",
  "description": "Get suggested test format with self-healing selectors and intent. Returns a template showing the recommended structure for maintainable, self-healing tests.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "format": {
        "type": "string",
        "enum": ["typescript", "yaml", "json"],
        "description": "Template format",
        "default": "typescript"
      },
      "example": {
        "type": "string",
        "enum": ["login", "checkout", "search", "minimal"],
        "description": "Example type to generate",
        "default": "minimal"
      }
    }
  }
}
```

**Returns (TypeScript):**
```typescript
import { device, test } from 'maestro-runner'

test('user can login', async () => {
  await device.launch('com.example.app')

  await device.tap({
    selectors: ['Login', 'Sign In', 'id:login_btn'],
    intent: 'tap login button on welcome screen'
  })

  await device.type({
    selectors: ['Email', 'id:email_input'],
    value: 'test@example.com',
    intent: 'enter email in login form'
  })

  await device.assertVisible({
    selectors: ['Welcome', 'id:home_screen'],
    intent: 'verify login succeeded'
  })
})
```

---

### Category 7: Flow Execution (Unique to maestro-runner)

#### `run_flow`
```json
{
  "name": "run_flow",
  "description": "Execute a YAML test flow",
  "inputSchema": {
    "type": "object",
    "properties": {
      "file": {
        "type": "string",
        "description": "Path to YAML flow file"
      },
      "env": {
        "type": "object",
        "description": "Environment variables for the flow"
      },
      "stop_on_fail": {
        "type": "boolean",
        "default": true
      }
    },
    "required": ["file"]
  }
}
```

**Returns:**
```json
{
  "status": "passed",
  "steps": 5,
  "passed": 5,
  "failed": 0,
  "duration_ms": 12340,
  "report_path": "/tmp/report/report.json"
}
```

#### `validate_flow`
```json
{
  "name": "validate_flow",
  "description": "Validate YAML flow syntax without running",
  "inputSchema": {
    "type": "object",
    "properties": {
      "file": { "type": "string" }
    },
    "required": ["file"]
  }
}
```

---

## MCP Resources (Documentation for AI)

Resources allow AI to read documentation about suggested test formats and best practices.
Users are NOT forced to use these formats - they are suggestions AI can reference.

### Resource: Test Format Guide

```go
// pkg/mcp/resources.go

type Resource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}

func GetResources() []Resource {
    return []Resource{
        {
            URI:         "maestro://docs/test-format",
            Name:        "Suggested Test Format",
            Description: "Recommended test structure with self-healing selectors and intent for maintainable tests",
            MimeType:    "text/markdown",
        },
        {
            URI:         "maestro://docs/selectors",
            Name:        "Selector Types Reference",
            Description: "Available selector types: text, id, desc, hint, class, xpath",
            MimeType:    "text/markdown",
        },
        {
            URI:         "maestro://docs/self-healing",
            Name:        "Self-Healing Strategy",
            Description: "How to write tests that self-heal using multiple selectors",
            MimeType:    "text/markdown",
        },
    }
}
```

### Resource Content: `maestro://docs/test-format`

```markdown
# Suggested Test Format

## Design Goals
- AI can write tests
- AI can heal broken tests
- Tests run fast without AI in the loop
- Self-healing via fallback selectors

## Structure

```typescript
await device.tap({
  selectors: ['Primary', 'Fallback1', 'id:fallback2'],
  intent: 'what this step is trying to do'
})
```

### Why Multiple Selectors?
- First selector that matches is used
- If UI changes, fallbacks work automatically
- Only call AI when ALL selectors fail

### Why Intent?
- Helps AI understand purpose when healing
- AI can find correct new selector with context
- Documents test steps

## Self-Healing Flow

1. Try selector 1 → Not found
2. Try selector 2 → Found ✓ (test continues)

When all fail:
1. Capture screenshot + hierarchy
2. Send to AI with intent
3. AI suggests new selector
4. Update test file
5. Retry step
```

---

## MCP Prompts (Templates for AI)

Prompts are pre-built templates AI can use for common test generation tasks.

```go
// pkg/mcp/prompts.go

type Prompt struct {
    Name        string     `json:"name"`
    Description string     `json:"description,omitempty"`
    Arguments   []Argument `json:"arguments,omitempty"`
}

type Argument struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    Required    bool   `json:"required,omitempty"`
}

func GetPrompts() []Prompt {
    return []Prompt{
        {
            Name:        "create_test",
            Description: "Create a test using the suggested format with self-healing selectors",
            Arguments: []Argument{
                {Name: "app_id", Description: "App package name", Required: true},
                {Name: "test_name", Description: "Name for the test", Required: true},
                {Name: "scenario", Description: "What the test should do", Required: true},
            },
        },
        {
            Name:        "heal_test",
            Description: "Fix a broken test by finding new selectors",
            Arguments: []Argument{
                {Name: "test_file", Description: "Path to broken test", Required: true},
                {Name: "error_message", Description: "Error from test run", Required: true},
            },
        },
        {
            Name:        "add_selectors",
            Description: "Add fallback selectors to existing test steps",
            Arguments: []Argument{
                {Name: "test_file", Description: "Path to test file", Required: true},
            },
        },
    }
}
```

### Prompt: `create_test`

When AI calls this prompt, it receives:

```
Create a test for {app_id} named "{test_name}" that does: {scenario}

Use this format for each step:

await device.tap({
  selectors: ['Primary Text', 'Alternate Text', 'id:resource_id'],
  intent: 'description of what this step does'
})

Guidelines:
1. Use 2-4 selectors per step (text first, then ID as fallback)
2. Intent should explain the purpose, not the action
3. Use get_locators tool to find all available selectors for elements
4. Prefer text selectors for readability, ID for reliability
```

### Prompt: `heal_test`

When AI calls this prompt:

```
A test has failed with error: {error_message}

Test file: {test_file}

Steps to heal:
1. Use screenshot tool to see current screen
2. Use hierarchy tool to get element tree
3. Use get_locators tool on visible elements
4. Find the step that failed (check intent)
5. Update selectors array with new valid selectors
6. Keep original selectors as fallbacks if they might work later
```

---

## REST API (For Test Execution)

REST API for users who want to run tests programmatically without Appium/Maestro CLI.

**Note:** This is separate from MCP. MCP is for AI agents. REST is for CI/CD and programmatic access.

### Base URL

```
http://localhost:9000/api/v1
```

### Endpoints

#### Start Server
```
POST /server/start
{
  "device_id": "emulator-5554"  // optional
}

Response:
{
  "status": "started",
  "session_id": "abc123",
  "device": "emulator-5554"
}
```

#### Execute Command
```
POST /command
{
  "action": "tap",
  "selector": "Login",
  "timeout": 17
}

Response:
{
  "status": "success",
  "element": { "text": "Login", "bounds": {...} }
}
```

#### Screenshot
```
GET /screenshot?format=base64

Response:
{
  "type": "image",
  "data": "base64...",
  "mimeType": "image/png"
}
```

#### UI Hierarchy
```
GET /hierarchy?format=json&filtered=true

Response:
{
  "elements": [...]
}
```

#### Run Flow
```
POST /flow/run
{
  "file": "/path/to/flow.yaml",
  "env": { "USERNAME": "test" }
}

Response:
{
  "status": "passed",
  "steps": 5,
  "passed": 5,
  "duration_ms": 12340
}
```

#### Stop Server
```
POST /server/stop

Response:
{
  "status": "stopped"
}
```

### CLI Command

```bash
# Start REST server
maestro-runner serve --port 9000

# With verbose logging
maestro-runner serve --port 9000 --verbose
```

---

## Error Handling Strategy

### Error Types

```go
// pkg/mcp/errors.go

type ErrorCategory string

const (
    ErrTransient  ErrorCategory = "transient"   // Retry-able
    ErrTerminal   ErrorCategory = "terminal"    // Fail fast
    ErrValidation ErrorCategory = "validation"  // Bad input
)

type ActionableError struct {
    Category   ErrorCategory
    Code       string   // e.g., "device_offline", "element_not_found"
    Message    string
    Suggestion string   // What user can do
    DocsURL    string   // Link to docs
}

// Transient errors (retry)
var transientErrors = map[string]bool{
    "null root node returned": true,
    "connection refused":      true,
    "device offline":          true,
    "socket timeout":          true,
}

// Terminal errors (fail fast)
var terminalErrors = map[string]ActionableError{
    "device_not_found": {
        Category:   ErrTerminal,
        Message:    "No device connected",
        Suggestion: "Connect device via USB or start emulator",
        DocsURL:    "https://docs.devicelab.dev/setup",
    },
    "app_not_installed": {
        Category:   ErrTerminal,
        Message:    "App not installed on device",
        Suggestion: "Install app: adb install <apk>",
    },
}
```

### Prerequisite Validation

```go
func (h *Handlers) ensureReady() error {
    // 1. Device connected?
    if !h.session.device.IsConnected() {
        return terminalErrors["device_not_found"]
    }

    // 2. UIAutomator2 running?
    if ready, _ := h.session.client.Status(); !ready {
        return &ActionableError{
            Category:   ErrTerminal,
            Message:    "UIAutomator2 server not running",
            Suggestion: "Call server_start first",
        }
    }

    return nil
}
```

---

## Session Management

```go
// pkg/mcp/session.go

type Session struct {
    Device    *device.AndroidDevice
    Client    *uiautomator2.Client
    Driver    *uiautomator2.Driver
    AppID     string
    StartedAt time.Time
    mu        sync.Mutex
}

func (s *Session) Start(deviceID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 1. Find/connect device
    // 2. Start UIAutomator2 server
    // 3. Create client and driver
    // 4. Create session
    return nil
}

func (s *Session) Stop() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 1. Delete session
    // 2. Stop UIAutomator2 server (optional)
    // 3. Cleanup resources
    return nil
}

func (s *Session) IsActive() bool {
    return s.Client != nil && s.Client.HasSession()
}
```

---

## Implementation Phases

### Phase 1: Core Infrastructure
- [ ] Create `pkg/mcp/` directory structure
- [ ] Implement JSON-RPC 2.0 server (`server.go`)
- [ ] Implement session management (`session.go`)
- [ ] Implement error types (`errors.go`)
- [ ] Add `mcp` command to CLI
- [ ] Basic tests

**Deliverable:** `maestro-runner mcp` starts and responds to `initialize`

### Phase 2: Device & App Tools
- [ ] `server_start` - Start UIAutomator2, create session
- [ ] `server_stop` - Cleanup
- [ ] `app_launch` - Launch app with options
- [ ] `app_stop` - Stop app
- [ ] `app_clear` - Clear app data
- [ ] Integration tests

**Deliverable:** AI can start server and launch apps

### Phase 3: Input Tools
- [ ] `tap` - With actionability waits
- [ ] `double_tap`
- [ ] `long_press`
- [ ] `type` - With clear option
- [ ] `swipe`
- [ ] `scroll_to`
- [ ] `back`
- [ ] Integration tests

**Deliverable:** AI can interact with app UI

### Phase 4: Screen Tools
- [ ] `screenshot` - Return base64 or save to file
- [ ] `hierarchy` - JSON/XML with filtering
- [ ] `get_orientation`
- [ ] `assert_visible`
- [ ] `assert_not_visible`
- [ ] Integration tests

**Deliverable:** AI can capture screen state and verify elements

### Phase 5: Inspector & Flow Tools
- [ ] `get_locators` - Generate selector alternatives
- [ ] `generate_flow` - Convert actions to YAML
- [ ] `get_test_template` - Return suggested test format
- [ ] `run_flow` - Execute YAML flows
- [ ] `validate_flow` - Syntax check
- [ ] Integration tests

**Deliverable:** AI can create and run test flows

### Phase 6: Resources & Prompts
- [ ] Implement MCP Resources handler
- [ ] Create `maestro://docs/test-format` resource
- [ ] Create `maestro://docs/selectors` resource
- [ ] Create `maestro://docs/self-healing` resource
- [ ] Implement MCP Prompts handler
- [ ] Create `create_test` prompt
- [ ] Create `heal_test` prompt
- [ ] Create `add_selectors` prompt

**Deliverable:** AI can read documentation and use templates

### Phase 7: REST API
- [ ] Create `pkg/rest/` package
- [ ] Implement HTTP server with routing
- [ ] Port MCP tool handlers to REST endpoints
- [ ] Add `maestro-runner serve` CLI command
- [ ] Integration tests

**Deliverable:** REST API for CI/CD and programmatic access

### Phase 8: Polish & Documentation
- [ ] Error message improvements
- [ ] Performance optimization
- [ ] Documentation
- [ ] Example prompts for AI
- [ ] Claude Desktop configuration guide

**Deliverable:** Production-ready MCP server + REST API

---

## CLI Integration

```go
// pkg/cli/mcp.go

var mcpCommand = &cli.Command{
    Name:  "mcp",
    Usage: "Start MCP server for AI agent integration",
    Description: `Start Model Context Protocol server over stdio.

This allows AI agents (Claude, etc.) to control mobile devices
through maestro-runner.

Configuration for Claude Desktop (~/.claude/settings.json):
  {
    "mcpServers": {
      "maestro-runner": {
        "command": "maestro-runner",
        "args": ["mcp"]
      }
    }
  }

Examples:
  # Start MCP server
  maestro-runner mcp

  # With verbose logging (to stderr)
  maestro-runner mcp --verbose
`,
    Flags: []cli.Flag{
        &cli.BoolFlag{
            Name:  "verbose",
            Usage: "Enable verbose logging to stderr",
        },
    },
    Action: runMCP,
}

func runMCP(c *cli.Context) error {
    server := mcp.NewServer(mcp.ServerOptions{
        Name:    "maestro-runner",
        Version: version,
        Verbose: c.Bool("verbose"),
    })

    return server.Run()
}
```

---

## Testing Strategy

### Unit Tests
- JSON-RPC parsing
- Error classification
- Session state transitions
- Tool schema validation

### Integration Tests
- Full tool execution against emulator
- Error scenarios (device offline, element not found)
- Flow execution

### Manual Testing
- Claude Desktop integration
- Various AI prompts
- Edge cases

---

## Configuration

### Claude Desktop
```json
// ~/.claude/settings.json
{
  "mcpServers": {
    "maestro-runner": {
      "command": "maestro-runner",
      "args": ["mcp"],
      "env": {
        "ANDROID_HOME": "/path/to/android/sdk"
      }
    }
  }
}
```

### Environment Variables
```
MAESTRO_MCP_VERBOSE=1      # Enable verbose logging
MAESTRO_MCP_TIMEOUT=30     # Default timeout in seconds
MAESTRO_MCP_DEVICE=xxx     # Default device ID
```

---

## Success Criteria

1. **Functional:** All 19 tools working correctly (+ Resources, Prompts, REST API)
2. **Reliable:** Proper error handling with actionable messages
3. **Fast:** <100ms overhead per tool call
4. **Documented:** Clear usage guide and examples
5. **Tested:** >80% code coverage

---

## Open Questions

1. **Multi-device support?** - Start with single device, add later
2. **iOS support?** - Android first, iOS in future phase
3. **Official SDK vs DIY?** - ✅ DECIDED: Use official Go SDK (`github.com/modelcontextprotocol/go-sdk`) - learned from appium-mcp that using a framework (FastMCP) significantly reduces boilerplate
4. **Streaming?** - Not in v1, consider for long operations later
5. **REST API auth?** - ✅ DECIDED: None for local use (same as appium-mcp), add optional token for remote
6. **Resource updates?** - How to notify AI when docs change?
7. **TypeScript SDK?** - Should we provide a client SDK for the self-healing test format?
8. **MCP-UI support?** - NEW: Consider `ui://` scheme for interactive device picker (v2)

---

## References

### MCP Protocol & SDKs
- [MCP Specification](https://modelcontextprotocol.io/specification/2025-11-25)
- [Official Go SDK](https://github.com/modelcontextprotocol/go-sdk)

### Mobile MCP Implementations
- [mobile-next/mobile-mcp](https://github.com/mobile-next/mobile-mcp) - ⭐ 3,070+ stars, cross-platform iOS/Android, hybrid perception
- [appium-mcp (GitHub)](https://github.com/AppVitals/appium-mcp) - TypeScript, FastMCP framework
- [appium-mcp (Local)](file:///Users/omnarayan/work/temp/AppVitals/appium-mcp) - Deep dive analysis done
- [nim444/mcp-android-server-python](https://github.com/nim444/mcp-android-server-python) - Python + uiautomator2, modular design
- [CursorTouch/Android-MCP](https://github.com/CursorTouch/Android-MCP) - Lightweight, combined state snapshot
- [erichung9060/Android-Mobile-MCP](https://github.com/erichung9060/Android-Mobile-MCP) - Pre-calculated center coords
- [minitap-ai/mobile-use](https://github.com/minitap-ai/mobile-use) - Full AI agent with LangGraph
- [clicker MCP](https://github.com/AceInfo1/vibium/tree/main/clicker/internal/mcp) - Browser automation reference

### Playwright MCP (Gold Standard)
- [Playwright MCP Server (Microsoft)](https://github.com/microsoft/playwright-mcp) - 40+ tools, accessibility tree approach
- [Playwright Test Agents](https://playwright.dev/docs/test-agents) - Planner, Generator, Healer agents
- [Using Playwright MCP with Claude Code](https://til.simonwillison.net/claude-code/playwright-mcp-claude-code) - Practical guide
- [ExecuteAutomation Playwright MCP](https://github.com/executeautomation/mcp-playwright) - Alternative implementation

### Key Learnings from Research

**From appium-mcp:**
- FastMCP TypeScript framework: `npm install fastmcp`
- Tool middleware pattern: `src/tools/index.ts`
- Session management: `src/session-store.ts`
- Device manager singleton: `src/devicemanager/`
- MCP-UI integration: `src/ui/`

**From Playwright MCP:**
- Accessibility tree > screenshots for LLM interaction
- Element refs for fast, unambiguous actions
- Tool annotations (readOnlyHint, destructiveHint)
- Opt-in capabilities (--capabilities vision,pdf)
- Incremental snapshots for efficiency
- Three-agent pattern: Planner → Generator → Healer

**From mobile-next/mobile-mcp:**
- Hybrid perception: a11y tree + screenshot fallback for 100% coverage
- Unified API across iOS/Android platforms
- Hardware button support (HOME, BACK, VOLUME)
- Element list with coordinates for games/custom views

**From mcp-android-server-python:**
- Modular tool organization (61-line server, tools in separate files)
- Two transport modes: stdio + HTTP
- Advanced features: smart_unlock, toast_detection, activity_monitoring
- wait_for_element and scroll_to_element patterns

**From Android-Mobile-MCP:**
- Pre-calculated center coordinates in UI dump
- Saves AI from doing bounds → center math

**From Android-MCP:**
- Combined state snapshot (foreground app + UI elements in one call)
- 2-4s latency expectation for actions

**From mobile-use:**
- Limitation: games don't expose accessibility tree
- Need coordinate-based fallback for full coverage
