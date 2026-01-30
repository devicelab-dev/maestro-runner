# Test Reporting Research Summary

Deep research on test reporting UX patterns from industry-leading tools.

---

## 1. Cypress - Gold Standard for Live Interactive Testing

### Key UX Patterns
- **Split-pane UI**: Command log (left) + live app view (right)
- **Time-travel debugging**: Hover over any step to see DOM state at that moment
- **Real-time feedback**: Commands execute with visual feedback
- **Automatic waiting**: Built into UX, no loading spinners

### Test Replay (Cloud Feature)
- Captures: test commands, network requests, console logs, DOM mutations, CSS styles
- **UI Layout**:
  - Command Log (left): Step-through debugging
  - Developer Tools (right): Network, console, DOM inspection
  - Run Header: Test metadata (run ID, branch, spec, platform, browser)
  - Replay Controls: Timeline scrubber, playback speed, attempt toggles
- Time-travel through failures at variable speeds
- Pause at specific moments to examine application state

### Reporting
- Built on Mocha reporters: `spec`, `junit`, `teamcity`
- **Mochawesome** for HTML reports (most popular)
- Multi-reporter support for CI (`cypress-multi-reporters`)
- Screenshots and videos via Cypress Cloud

**Source**: [Cypress Reporters Docs](https://docs.cypress.io/guides/tooling/reporters)

---

## 2. Playwright - Best for Trace-Based Post-Mortem Debugging

### HTML Reporter
- Self-contained folder served as web page
- Opens automatically on test failures
- Expandable test tree (suites → tests → steps)
- Click any test → see steps, screenshots, video, trace
- Filter by status (passed/failed/flaky/skipped)

### Trace Viewer - The Killer Feature
- **Actions Tab**: Each action with timing, locator info
  - Hover reveals DOM snapshot changes
  - Three snapshot views: before action, during input, after completion
- **Timeline & Screenshots**: Filmstrip view, hover for magnified views
- **DOM Snapshots**: Complete DOM states, highlighted nodes and click positions
- **Source Code Integration**: Click actions to highlight code lines
- **Call Details**: Duration, locator specificity, key inputs
- **Log Tab**: Behind-the-scenes operations (visibility checks, action sequences)
- **Errors Tab**: Failure messages with timeline markers, source line references
- **Console Tab**: Browser + test logs, filterable by action
- **Network Tab**: All HTTP requests, sortable, expandable headers/body
- **Metadata Tab**: Browser type, viewport, test duration
- **Attachments Tab**: Visual regression comparison with image diff overlays

### UI Mode (`--ui`)
- **Layout**: Left sidebar (test files), Main panel (timeline + analysis tabs)
- **Watch Mode**: Auto-rerun on file changes (eye icon toggle)
- **Locator Picking**: Interactive element selection on DOM snapshot
- **Pop-out DOM**: Open snapshots in separate windows for DevTools access

### Built-in Reporters
| Reporter | Use Case |
|----------|----------|
| `list` | Default locally, one line per test |
| `dot` | Default CI, one character per test |
| `html` | Self-contained web report |
| `json` | Programmatic access |
| `junit` | CI/CD integration |
| `blob` | Merging sharded test results |
| `github` | GitHub Actions annotations |

**Source**: [Playwright Test Reporters](https://playwright.dev/docs/test-reporters), [Trace Viewer](https://playwright.dev/docs/trace-viewer)

---

## 3. Allure Report - Best for Rich Visualization

### Architecture
1. **Data Collection**: Framework adapters capture execution data
2. **Metadata Enrichment**: Environment, CI/CD, history, categories
3. **Report Generation**: Static HTML (`allure generate`) or live server (`allure serve`)

### Core Features

#### Test Steps
- Divide complex tests into well-defined steps
- Nested steps form tree-like structures
- Each step has: duration, status (Passed/Failed/Broken), parameters
- Can attach files to individual steps
- Fixtures displayed in special blocks above/below test steps

#### Attachments
| Type | Formats |
|------|---------|
| Images | PNG, JPEG, GIF, SVG, BMP, TIFF |
| Videos | MP4, OGG, WebM |
| Text | Plain, HTML, CSV, TSV, XML, JSON, YAML |
| Visual Diff | Custom format with overlay comparison |

#### Visual Analytics
- Pie charts for pass/fail breakdown
- Trend graphs across runs
- Timeline view for parallelization issues

#### Defect Categories
- Classify failures: product bug, automation issue, system problem
- Custom category rules

#### Test Stability Analysis
- Flaky test detection
- History tracking across runs
- Retry tracking

#### Export Options
- CSV for spreadsheets
- Metrics to InfluxDB/Prometheus

### Report Navigation
- **Overview**: Summary with charts
- **Categories**: Failures grouped by type
- **Suites**: Test hierarchy
- **Timeline**: Execution sequence
- **Behaviors**: BDD-style grouping
- **Packages**: Code structure view

**Source**: [Allure Docs](https://allurereport.org/docs/), [Features](https://allurereport.org/docs/features-overview/)

---

## 4. ReportPortal - Best for Analytics at Scale

### Real-Time Capabilities
- Stream results as tests run
- Monitor execution status directly
- Centralized logs, screenshots, binary files

### AI-Powered Features
- **Automatic root cause analysis** for failures
- **AI-based defect triage**: Flags results, alerts engineers
- Historical pattern matching for analysis

### Dashboard Widgets (28+ types)

| Category | Widgets |
|----------|---------|
| **Trends** | Launch statistics, duration comparisons, cumulative trends |
| **Status** | Overall statistics, pass/fail/skip, defect breakdown |
| **Analysis** | Investigated %, flaky tests, TOP-50 failures |
| **Comparison** | Side-by-side launches, patterns, time-consuming tests |
| **Bugs** | Unique bugs table, component health |

### Visualization Options
- Bar charts, area charts, pie charts, tables
- Customizable, resizable, rearrangeable widgets
- Drill-down capabilities

### Collaborative Features
- Team analysis through shared dashboards
- "To Investigate" workflow for triage
- Associate failures with: product bugs, automation issues, system problems

**Source**: [ReportPortal Docs](https://reportportal.io/docs/)

---

## 5. Vitest - Best for Module-Graph Intelligence

### UI Mode
- Browser-based dashboard at `localhost:51204/__vitest__/`
- Requires watch mode

### Module Graph Visualization
- Shows dependency structure of test files
- **Complexity Management**: First 2 levels when >50 modules
- **Navigation**: Right-click/Shift+click for dependencies
- **Filtering**: Toggle node_modules visibility

### Performance Metrics
- **Self Time**: Excluding static imports
- **Total Time**: Including static imports
- **Transform Duration**: Processing time
- Color coding: Orange >100ms, Red >500ms

### Import Breakdown
- Top 10 slowest-loading modules
- Yellow = external modules
- Expandable list

### Reporters
- Default, Verbose, Tree, Dot
- JUnit, JSON, TAP
- GitHub Actions annotations
- HTML (via `@vitest/ui`)

**Source**: [Vitest UI Guide](https://vitest.dev/guide/ui)

---

## 6. Mochawesome - Beautiful Standalone HTML

### Design Features
- Modern, clean, responsive design
- ChartJS charts for visualization
- Mobile-friendly
- Offline viewing capability

### Report Contents
- Nested test suites and hooks
- Inline test code review
- Stack traces for failures
- Pass/fail statistics with charts
- Execution times

### Context Attachments
```javascript
addContext(test, 'Simple string')
addContext(test, 'https://example.com/image.png') // Embedded inline
addContext(test, { title: 'Environment', value: { browser: 'Chrome' } })
```

### Customization
- Dynamic filename tokens: `[name]`, `[status]`, `[datetime]`
- Embedded screenshots option
- Inline assets for single-file reports

**Source**: [Mochawesome GitHub](https://github.com/adamgruber/mochawesome)

---

## 7. Detox - Mobile-Specific Artifacts

### Artifact Types
| Artifact | Format | Notes |
|----------|--------|-------|
| Screenshots | PNG | Auto on failure, manual via API |
| Videos | MP4 | Configurable bitrate/codec |
| Logs | .log | Device output |
| Performance | .dtxrec | iOS only, requires Detox Instruments |
| UI Hierarchy | XML | iOS with Xcode 12+ |

### Recording Modes
- `all`: Every test
- `failing`: Only failed tests
- `manual`: Explicit capture only
- `none`: Disabled

### Directory Structure
```
artifacts/
└── config-timestamp/
    └── test-name/
        ├── screenshot.png
        ├── video.mp4
        └── device.log
```

**Source**: [Detox Artifacts Docs](https://wix.github.io/Detox/docs/config/artifacts)

---

## 8. Jest - Module Coverage Focus

### Built-in Reporters
- `default`: Standard output with summary
- `summary`: Condensed summary only
- `github-actions`: Annotations for CI

### Coverage Reports
- Formats: Clover, JSON, LCOV, Text
- Per-file and per-directory thresholds
- Providers: Babel (istanbul) or V8 (c8)

### Custom Reporter API
```javascript
class CustomReporter {
  onRunComplete(testContexts, results) {
    // Access: success, startTime, numTotalTestSuites,
    // numPassedTests, testResults[], coverage
  }
}
```

**Source**: [Jest Configuration Docs](https://jestjs.io/docs/configuration#reporters-arraymodulename--modulename-options)

---

## 9. Mocha - Reporter Variety

### Built-in Reporters
| Reporter | Output |
|----------|--------|
| `spec` | Hierarchical view (default) |
| `dot` | Minimal dots |
| `nyan` | Nyan cat animation |
| `list` | Simple list |
| `progress` | Progress bar |
| `json` | Single JSON object |
| `json-stream` | Newline-delimited events |
| `min` | Summary only |
| `doc` | HTML documentation |
| `markdown` | GitHub wiki format |
| `xunit` | CI-compatible XML |

**Source**: [Mocha Reporters](https://mochajs.org/#reporters)

---

## 10. Storybook - Component States Catalog

### Test Runner Features
- Transforms stories into Jest/Playwright tests
- Verifies render without errors
- Checks assertions in play functions

### Output Formats
- CLI output
- JSON (`--json`)
- JUnit XML (`--junit`)
- Coverage via addon

### CI Integration
- Sharded execution (`--shard [index/count]`)
- Error truncation configurable via `DEBUG_PRINT_LIMIT`

**Source**: [Storybook Test Runner](https://storybook.js.org/docs/writing-tests/test-runner)

---

## Key Insights for Maestro Runner

### What to Steal

| Feature | Source | Adaptation for Mobile |
|---------|--------|----------------------|
| Left panel: step list with current highlight | Cypress | Parse Maestro flow, show clickable list |
| Right panel: live device stream | Cypress | WebRTC stream (already solved) |
| Timeline scrubber | Playwright | Record stream, allow scrubbing |
| Before/after snapshots on failure | Playwright | Capture only on demand (avoid overhead) |
| Watch mode with smart re-run | Vitest | Re-run only affected flows on .yaml change |
| Trace file export | Playwright | Shareable .zip with video + logs |
| Element inspector overlay | Appium | Click-to-inspect on streamed view |

### Critical Insight
> Cypress/Playwright can afford their UX because DOM snapshots are cheap. For mobile, lean heavily on video stream for real-time experience and only capture discrete snapshots for forensics (failures, checkpoints, explicit requests).

---

## Recommended Report Architecture

### Layer 1: Live Runner UI
- Steps (left) + stream (right)
- Current step highlighted
- Live pass/fail indicators
- Real-time console/log output

### Layer 2: Single Run HTML Report
- Expandable test tree (flow → commands)
- Video per flow (streamable, not giant file)
- Screenshots on failure (before/after)
- Each command with duration and status
- Error message + stack trace
- Filters: failed only, sort by duration

### Layer 3: Aggregated Dashboard
- Pass rate trends over time
- Slowest tests ranking
- Flaky test detection
- Device/OS breakdown
- Historical comparison

---

## Report Format Recommendations

### MVP: Static HTML (like Playwright)
- Single file, shareable, no server needed
- Upload as CI artifact
- Embed video as base64 or link to files
- JavaScript for interactivity (expand/collapse, filters)

### Later: Allure Integration
- Well-known standard for teams
- Rich ecosystem of integrations
- Categories, trends, attachments built-in

### Enterprise: ReportPortal
- AI-powered analysis
- Real-time streaming
- Team collaboration features

---

## HTML Report Structure (MVP)

```
┌─────────────────────────────────────────────────────────────┐
│ Summary Bar: Total | Passed ✓ | Failed ✗ | Skipped ○       │
├──────────────────────┬──────────────────────────────────────┤
│ Test Tree            │ Detail Panel                         │
│                      │                                      │
│ ▼ login.yaml         │ ┌────────────────────────────────┐  │
│   ✓ launchApp (2s)   │ │ Video Player                   │  │
│   ✓ tapOn (0.5s)     │ │ [▶ advancement bar ──────── ]  │  │
│   ✗ assertVisible    │ └────────────────────────────────┘  │
│                      │                                      │
│ ▶ checkout.yaml      │ Error: Element not found             │
│                      │ Expected: "Welcome"                  │
│                      │ Timeout: 5000ms                      │
│                      │                                      │
│                      │ ┌──────────┐ ┌──────────┐           │
│                      │ │ Before   │ │ After    │           │
│                      │ │ [image]  │ │ [image]  │           │
│                      │ └──────────┘ └──────────┘           │
├──────────────────────┴──────────────────────────────────────┤
│ Console | Network | Device Info                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Sources

- [Cypress Reporters](https://docs.cypress.io/guides/tooling/reporters)
- [Cypress Test Replay](https://docs.cypress.io/cloud/features/test-replay)
- [Playwright Test Reporters](https://playwright.dev/docs/test-reporters)
- [Playwright Trace Viewer](https://playwright.dev/docs/trace-viewer)
- [Playwright UI Mode](https://playwright.dev/docs/test-ui-mode)
- [Allure Report Docs](https://allurereport.org/docs/)
- [Allure Steps](https://allurereport.org/docs/steps/)
- [Allure Attachments](https://allurereport.org/docs/attachments/)
- [ReportPortal Docs](https://reportportal.io/docs/)
- [Vitest Reporters](https://vitest.dev/guide/reporters)
- [Vitest UI](https://vitest.dev/guide/ui)
- [Mochawesome](https://github.com/adamgruber/mochawesome)
- [Jest Configuration](https://jestjs.io/docs/configuration)
- [Mocha Reporters](https://mochajs.org/#reporters)
- [Detox Artifacts](https://wix.github.io/Detox/docs/config/artifacts)
- [Storybook Test Runner](https://storybook.js.org/docs/writing-tests/test-runner)
- [WebdriverIO Allure](https://webdriver.io/docs/allure-reporter/)
