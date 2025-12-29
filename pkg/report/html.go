package report

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTMLConfig contains configuration for HTML report generation.
type HTMLConfig struct {
	OutputPath    string // Path to write the HTML file
	EmbedAssets   bool   // Embed screenshots as base64 (makes file larger but portable)
	Title         string // Report title (default: "Test Report")
	ReportDir     string // Directory containing report.json (needed for asset paths)
}

// GenerateHTML generates an HTML report from the report directory.
func GenerateHTML(reportDir string, cfg HTMLConfig) error {
	// Read report data
	index, flows, err := ReadReport(reportDir)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}

	// Set defaults
	if cfg.Title == "" {
		cfg.Title = "Test Report"
	}
	if cfg.ReportDir == "" {
		cfg.ReportDir = reportDir
	}
	if cfg.OutputPath == "" {
		cfg.OutputPath = filepath.Join(reportDir, "report.html")
	}

	// Build template data
	data := buildHTMLData(index, flows, cfg)

	// Generate HTML
	html, err := renderHTML(data)
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	// Write file
	if err := os.WriteFile(cfg.OutputPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}

	return nil
}

// HTMLData contains all data needed for the HTML template.
type HTMLData struct {
	Title       string
	GeneratedAt string
	Index       *Index
	Flows       []FlowHTMLData
	StatusClass map[Status]string
	JSONData    template.JS // JSON data for JavaScript
}

// FlowHTMLData contains flow data formatted for HTML.
type FlowHTMLData struct {
	FlowDetail
	StatusClass string
	DurationStr string
	Commands    []CommandHTMLData
}

// CommandHTMLData contains command data formatted for HTML.
type CommandHTMLData struct {
	Command
	StatusClass       string
	DurationStr       string
	ScreenshotBefore  string // base64 or path
	ScreenshotAfter   string // base64 or path
	HasScreenshots    bool
}

func buildHTMLData(index *Index, flows []FlowDetail, cfg HTMLConfig) HTMLData {
	statusClass := map[Status]string{
		StatusPassed:  "passed",
		StatusFailed:  "failed",
		StatusSkipped: "skipped",
		StatusRunning: "running",
		StatusPending: "pending",
	}

	flowsData := make([]FlowHTMLData, len(flows))
	for i, f := range flows {
		cmds := make([]CommandHTMLData, len(f.Commands))
		for j, c := range f.Commands {
			cmd := CommandHTMLData{
				Command:     c,
				StatusClass: statusClass[c.Status],
				DurationStr: formatDuration(c.Duration),
			}

			// Handle screenshots
			if c.Artifacts.ScreenshotBefore != "" {
				if cfg.EmbedAssets {
					cmd.ScreenshotBefore = loadAsBase64(filepath.Join(cfg.ReportDir, c.Artifacts.ScreenshotBefore))
				} else {
					cmd.ScreenshotBefore = c.Artifacts.ScreenshotBefore
				}
				cmd.HasScreenshots = true
			}
			if c.Artifacts.ScreenshotAfter != "" {
				if cfg.EmbedAssets {
					cmd.ScreenshotAfter = loadAsBase64(filepath.Join(cfg.ReportDir, c.Artifacts.ScreenshotAfter))
				} else {
					cmd.ScreenshotAfter = c.Artifacts.ScreenshotAfter
				}
				cmd.HasScreenshots = true
			}

			cmds[j] = cmd
		}

		flowsData[i] = FlowHTMLData{
			FlowDetail:  f,
			StatusClass: statusClass[index.Flows[i].Status],
			DurationStr: formatDuration(f.Duration),
			Commands:    cmds,
		}
	}

	// Serialize index and flows to JSON for JavaScript
	jsonBytes, _ := json.Marshal(map[string]interface{}{
		"index": index,
		"flows": flows,
	})

	return HTMLData{
		Title:       cfg.Title,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Index:       index,
		Flows:       flowsData,
		StatusClass: statusClass,
		JSONData:    template.JS(jsonBytes),
	}
}

func formatDuration(ms *int64) string {
	if ms == nil {
		return "-"
	}
	d := time.Duration(*ms) * time.Millisecond
	if d < time.Second {
		return fmt.Sprintf("%dms", *ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}

func loadAsBase64(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		mimeType = "image/jpeg"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

func renderHTML(data HTMLData) (string, error) {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <style>
        :root {
            --bg-primary: #1a1a2e;
            --bg-secondary: #16213e;
            --bg-tertiary: #0f3460;
            --text-primary: #eee;
            --text-secondary: #aaa;
            --text-muted: #666;
            --border-color: #333;
            --passed: #22c55e;
            --failed: #ef4444;
            --skipped: #eab308;
            --running: #3b82f6;
            --pending: #6b7280;
            --accent: #8b5cf6;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.5;
        }

        /* Summary Bar */
        .summary-bar {
            background: var(--bg-secondary);
            padding: 16px 24px;
            display: flex;
            align-items: center;
            gap: 24px;
            border-bottom: 1px solid var(--border-color);
            position: sticky;
            top: 0;
            z-index: 100;
        }

        .summary-title {
            font-size: 18px;
            font-weight: 600;
            flex-shrink: 0;
        }

        .summary-stats {
            display: flex;
            gap: 16px;
            flex-wrap: wrap;
        }

        .stat {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 4px 12px;
            background: var(--bg-tertiary);
            border-radius: 4px;
            font-size: 14px;
        }

        .stat-icon {
            width: 8px;
            height: 8px;
            border-radius: 50%;
        }

        .stat-icon.passed { background: var(--passed); }
        .stat-icon.failed { background: var(--failed); }
        .stat-icon.skipped { background: var(--skipped); }
        .stat-icon.pending { background: var(--pending); }

        .summary-meta {
            margin-left: auto;
            font-size: 12px;
            color: var(--text-secondary);
        }

        /* Main Layout */
        .main-container {
            display: flex;
            height: calc(100vh - 60px);
        }

        /* Flow List (Left Panel) */
        .flow-list {
            width: 320px;
            min-width: 280px;
            background: var(--bg-secondary);
            border-right: 1px solid var(--border-color);
            overflow-y: auto;
            flex-shrink: 0;
        }

        .flow-item {
            border-bottom: 1px solid var(--border-color);
        }

        .flow-header {
            padding: 12px 16px;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 10px;
            transition: background 0.15s;
        }

        .flow-header:hover {
            background: var(--bg-tertiary);
        }

        .flow-header.selected {
            background: var(--bg-tertiary);
            border-left: 3px solid var(--accent);
        }

        .flow-status {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            flex-shrink: 0;
        }

        .flow-status.passed { background: var(--passed); }
        .flow-status.failed { background: var(--failed); }
        .flow-status.skipped { background: var(--skipped); }
        .flow-status.running { background: var(--running); }
        .flow-status.pending { background: var(--pending); }

        .flow-name {
            flex: 1;
            font-size: 14px;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .flow-duration {
            font-size: 12px;
            color: var(--text-secondary);
            flex-shrink: 0;
        }

        .flow-toggle {
            color: var(--text-muted);
            font-size: 12px;
            transition: transform 0.2s;
        }

        .flow-toggle.expanded {
            transform: rotate(90deg);
        }

        /* Commands (Nested under flow) */
        .command-list {
            display: none;
            background: var(--bg-primary);
        }

        .command-list.expanded {
            display: block;
        }

        .command-item {
            padding: 8px 16px 8px 40px;
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 13px;
            cursor: pointer;
            border-bottom: 1px solid var(--border-color);
            transition: background 0.15s;
        }

        .command-item:hover {
            background: var(--bg-secondary);
        }

        .command-item.selected {
            background: var(--bg-secondary);
        }

        .command-status {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            flex-shrink: 0;
        }

        .command-status.passed { background: var(--passed); }
        .command-status.failed { background: var(--failed); }
        .command-status.skipped { background: var(--skipped); }
        .command-status.running { background: var(--running); }
        .command-status.pending { background: var(--pending); }

        .command-type {
            color: var(--accent);
            font-family: monospace;
            flex-shrink: 0;
        }

        .command-desc {
            flex: 1;
            color: var(--text-secondary);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .command-duration {
            font-size: 11px;
            color: var(--text-muted);
            flex-shrink: 0;
        }

        /* Detail Panel (Right) */
        .detail-panel {
            flex: 1;
            overflow-y: auto;
            padding: 24px;
        }

        .detail-empty {
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100%;
            color: var(--text-muted);
            font-size: 14px;
        }

        .detail-header {
            margin-bottom: 24px;
        }

        .detail-title {
            font-size: 20px;
            font-weight: 600;
            margin-bottom: 8px;
        }

        .detail-meta {
            display: flex;
            gap: 16px;
            font-size: 13px;
            color: var(--text-secondary);
        }

        .detail-meta-item {
            display: flex;
            align-items: center;
            gap: 6px;
        }

        /* Error Box */
        .error-box {
            background: rgba(239, 68, 68, 0.1);
            border: 1px solid var(--failed);
            border-radius: 8px;
            padding: 16px;
            margin-bottom: 24px;
        }

        .error-type {
            color: var(--failed);
            font-weight: 600;
            font-size: 14px;
            margin-bottom: 8px;
        }

        .error-message {
            font-family: monospace;
            font-size: 13px;
            white-space: pre-wrap;
            word-break: break-word;
        }

        .error-suggestion {
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid rgba(239, 68, 68, 0.3);
            font-size: 13px;
            color: var(--text-secondary);
        }

        /* Screenshots */
        .screenshots {
            display: flex;
            gap: 16px;
            margin-bottom: 24px;
        }

        .screenshot {
            flex: 1;
            max-width: 400px;
        }

        .screenshot-label {
            font-size: 12px;
            color: var(--text-secondary);
            margin-bottom: 8px;
        }

        .screenshot img {
            width: 100%;
            border-radius: 8px;
            border: 1px solid var(--border-color);
        }

        /* Command Detail */
        .command-detail {
            background: var(--bg-secondary);
            border-radius: 8px;
            padding: 16px;
            margin-bottom: 16px;
        }

        .command-detail-header {
            display: flex;
            align-items: center;
            gap: 12px;
            margin-bottom: 12px;
        }

        .command-detail-type {
            font-family: monospace;
            font-size: 16px;
            color: var(--accent);
        }

        .command-detail-status {
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 500;
        }

        .command-detail-status.passed { background: rgba(34, 197, 94, 0.2); color: var(--passed); }
        .command-detail-status.failed { background: rgba(239, 68, 68, 0.2); color: var(--failed); }
        .command-detail-status.skipped { background: rgba(234, 179, 8, 0.2); color: var(--skipped); }

        .yaml-block {
            background: var(--bg-primary);
            border-radius: 4px;
            padding: 12px;
            font-family: monospace;
            font-size: 13px;
            white-space: pre-wrap;
            overflow-x: auto;
        }

        /* Device Info Footer */
        .device-info {
            background: var(--bg-secondary);
            border-top: 1px solid var(--border-color);
            padding: 12px 24px;
            display: flex;
            gap: 24px;
            font-size: 12px;
            color: var(--text-secondary);
        }

        .device-info-item {
            display: flex;
            gap: 6px;
        }

        .device-info-label {
            color: var(--text-muted);
        }

        /* Filters */
        .filters {
            padding: 12px 16px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            gap: 8px;
        }

        .filter-btn {
            padding: 4px 10px;
            border: 1px solid var(--border-color);
            border-radius: 4px;
            background: transparent;
            color: var(--text-secondary);
            font-size: 12px;
            cursor: pointer;
            transition: all 0.15s;
        }

        .filter-btn:hover {
            background: var(--bg-tertiary);
        }

        .filter-btn.active {
            background: var(--accent);
            border-color: var(--accent);
            color: white;
        }

        /* Responsive */
        @media (max-width: 768px) {
            .main-container {
                flex-direction: column;
                height: auto;
            }

            .flow-list {
                width: 100%;
                max-height: 50vh;
            }

            .screenshots {
                flex-direction: column;
            }

            .screenshot {
                max-width: 100%;
            }
        }
    </style>
</head>
<body>
    <!-- Summary Bar -->
    <div class="summary-bar">
        <div class="summary-title">{{.Title}}</div>
        <div class="summary-stats">
            <div class="stat">
                <span>Total: {{.Index.Summary.Total}}</span>
            </div>
            <div class="stat">
                <span class="stat-icon passed"></span>
                <span>{{.Index.Summary.Passed}} passed</span>
            </div>
            <div class="stat">
                <span class="stat-icon failed"></span>
                <span>{{.Index.Summary.Failed}} failed</span>
            </div>
            {{if gt .Index.Summary.Skipped 0}}
            <div class="stat">
                <span class="stat-icon skipped"></span>
                <span>{{.Index.Summary.Skipped}} skipped</span>
            </div>
            {{end}}
        </div>
        <div class="summary-meta">
            Generated: {{.GeneratedAt}}
        </div>
    </div>

    <!-- Main Container -->
    <div class="main-container">
        <!-- Flow List -->
        <div class="flow-list">
            <div class="filters">
                <button class="filter-btn active" data-filter="all">All</button>
                <button class="filter-btn" data-filter="failed">Failed</button>
                <button class="filter-btn" data-filter="passed">Passed</button>
            </div>

            {{range $fi, $flow := .Flows}}
            <div class="flow-item" data-flow-index="{{$fi}}" data-status="{{$flow.StatusClass}}">
                <div class="flow-header" onclick="toggleFlow({{$fi}})">
                    <span class="flow-toggle">▶</span>
                    <span class="flow-status {{$flow.StatusClass}}"></span>
                    <span class="flow-name">{{$flow.Name}}</span>
                    <span class="flow-duration">{{$flow.DurationStr}}</span>
                </div>
                <div class="command-list" id="commands-{{$fi}}">
                    {{range $ci, $cmd := $flow.Commands}}
                    <div class="command-item" onclick="selectCommand({{$fi}}, {{$ci}})" data-flow="{{$fi}}" data-cmd="{{$ci}}">
                        <span class="command-status {{$cmd.StatusClass}}"></span>
                        <span class="command-type">{{$cmd.Type}}</span>
                        <span class="command-desc">{{if $cmd.Params}}{{if $cmd.Params.Selector}}{{$cmd.Params.Selector.Value}}{{else if $cmd.Params.Text}}{{$cmd.Params.Text}}{{end}}{{end}}</span>
                        <span class="command-duration">{{$cmd.DurationStr}}</span>
                    </div>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>

        <!-- Detail Panel -->
        <div class="detail-panel" id="detail-panel">
            <div class="detail-empty">
                Select a flow or command to view details
            </div>
        </div>
    </div>

    <!-- Device Info -->
    <div class="device-info">
        <div class="device-info-item">
            <span class="device-info-label">Device:</span>
            <span>{{.Index.Device.Name}}</span>
        </div>
        <div class="device-info-item">
            <span class="device-info-label">Platform:</span>
            <span>{{.Index.Device.Platform}} {{.Index.Device.OSVersion}}</span>
        </div>
        <div class="device-info-item">
            <span class="device-info-label">App:</span>
            <span>{{.Index.App.ID}}</span>
        </div>
        <div class="device-info-item">
            <span class="device-info-label">Driver:</span>
            <span>{{.Index.MaestroRunner.Driver}}</span>
        </div>
    </div>

    <script>
        // Report data
        const reportData = {{.JSONData}};
        const index = reportData.index;
        const flows = reportData.flows;

        let selectedFlow = null;
        let selectedCommand = null;

        // Toggle flow expansion
        function toggleFlow(flowIndex) {
            const cmdList = document.getElementById('commands-' + flowIndex);
            const flowItem = document.querySelector('[data-flow-index="' + flowIndex + '"]');
            const toggle = flowItem.querySelector('.flow-toggle');

            cmdList.classList.toggle('expanded');
            toggle.classList.toggle('expanded');

            // Select flow when expanding
            if (cmdList.classList.contains('expanded')) {
                selectFlow(flowIndex);
            }
        }

        // Select a flow
        function selectFlow(flowIndex) {
            // Remove previous selection
            document.querySelectorAll('.flow-header.selected').forEach(el => el.classList.remove('selected'));
            document.querySelectorAll('.command-item.selected').forEach(el => el.classList.remove('selected'));

            // Add selection
            const flowItem = document.querySelector('[data-flow-index="' + flowIndex + '"]');
            flowItem.querySelector('.flow-header').classList.add('selected');

            selectedFlow = flowIndex;
            selectedCommand = null;

            showFlowDetail(flowIndex);
        }

        // Select a command
        function selectCommand(flowIndex, cmdIndex) {
            event.stopPropagation();

            // Remove previous selection
            document.querySelectorAll('.flow-header.selected').forEach(el => el.classList.remove('selected'));
            document.querySelectorAll('.command-item.selected').forEach(el => el.classList.remove('selected'));

            // Add selection
            const cmdItem = document.querySelector('[data-flow="' + flowIndex + '"][data-cmd="' + cmdIndex + '"]');
            cmdItem.classList.add('selected');

            selectedFlow = flowIndex;
            selectedCommand = cmdIndex;

            showCommandDetail(flowIndex, cmdIndex);
        }

        // Show flow detail
        function showFlowDetail(flowIndex) {
            const flow = flows[flowIndex];
            const entry = index.flows[flowIndex];
            const panel = document.getElementById('detail-panel');

            let html = '<div class="detail-header">';
            html += '<div class="detail-title">' + escapeHtml(flow.name) + '</div>';
            html += '<div class="detail-meta">';
            html += '<div class="detail-meta-item">Status: <span class="command-detail-status ' + entry.status + '">' + entry.status + '</span></div>';
            html += '<div class="detail-meta-item">Duration: ' + formatDuration(flow.duration) + '</div>';
            html += '<div class="detail-meta-item">Commands: ' + flow.commands.length + '</div>';
            html += '</div></div>';

            // Show error if failed
            if (entry.status === 'failed') {
                const failedCmd = flow.commands.find(c => c.status === 'failed');
                if (failedCmd && failedCmd.error) {
                    html += '<div class="error-box">';
                    html += '<div class="error-type">' + escapeHtml(failedCmd.error.type || 'Error') + '</div>';
                    html += '<div class="error-message">' + escapeHtml(failedCmd.error.message) + '</div>';
                    if (failedCmd.error.suggestion) {
                        html += '<div class="error-suggestion">💡 ' + escapeHtml(failedCmd.error.suggestion) + '</div>';
                    }
                    html += '</div>';
                }
            }

            // Show source file
            html += '<div class="command-detail">';
            html += '<div class="command-detail-header">';
            html += '<div class="command-detail-type">Source File</div>';
            html += '</div>';
            html += '<div class="yaml-block">' + escapeHtml(flow.sourceFile) + '</div>';
            html += '</div>';

            panel.innerHTML = html;
        }

        // Show command detail
        function showCommandDetail(flowIndex, cmdIndex) {
            const flow = flows[flowIndex];
            const cmd = flow.commands[cmdIndex];
            const panel = document.getElementById('detail-panel');

            let html = '<div class="detail-header">';
            html += '<div class="detail-title">' + escapeHtml(cmd.type) + '</div>';
            html += '<div class="detail-meta">';
            html += '<div class="detail-meta-item">Status: <span class="command-detail-status ' + cmd.status + '">' + cmd.status + '</span></div>';
            html += '<div class="detail-meta-item">Duration: ' + formatDuration(cmd.duration) + '</div>';
            html += '</div></div>';

            // Show error if failed
            if (cmd.error) {
                html += '<div class="error-box">';
                html += '<div class="error-type">' + escapeHtml(cmd.error.type || 'Error') + '</div>';
                html += '<div class="error-message">' + escapeHtml(cmd.error.message) + '</div>';
                if (cmd.error.details) {
                    html += '<div class="error-message" style="margin-top: 8px; color: var(--text-secondary);">' + escapeHtml(cmd.error.details) + '</div>';
                }
                if (cmd.error.suggestion) {
                    html += '<div class="error-suggestion">💡 ' + escapeHtml(cmd.error.suggestion) + '</div>';
                }
                html += '</div>';
            }

            // Show screenshots
            if (cmd.artifacts && (cmd.artifacts.screenshotBefore || cmd.artifacts.screenshotAfter)) {
                html += '<div class="screenshots">';
                if (cmd.artifacts.screenshotBefore) {
                    html += '<div class="screenshot">';
                    html += '<div class="screenshot-label">Before</div>';
                    html += '<img src="' + cmd.artifacts.screenshotBefore + '" alt="Before">';
                    html += '</div>';
                }
                if (cmd.artifacts.screenshotAfter) {
                    html += '<div class="screenshot">';
                    html += '<div class="screenshot-label">After</div>';
                    html += '<img src="' + cmd.artifacts.screenshotAfter + '" alt="After">';
                    html += '</div>';
                }
                html += '</div>';
            }

            // Show YAML
            if (cmd.yaml) {
                html += '<div class="command-detail">';
                html += '<div class="command-detail-header">';
                html += '<div class="command-detail-type">YAML</div>';
                html += '</div>';
                html += '<div class="yaml-block">' + escapeHtml(cmd.yaml) + '</div>';
                html += '</div>';
            }

            // Show element info
            if (cmd.element && cmd.element.found) {
                html += '<div class="command-detail">';
                html += '<div class="command-detail-header">';
                html += '<div class="command-detail-type">Element Found</div>';
                html += '</div>';
                html += '<div class="yaml-block">';
                if (cmd.element.id) html += 'ID: ' + escapeHtml(cmd.element.id) + '\n';
                if (cmd.element.text) html += 'Text: ' + escapeHtml(cmd.element.text) + '\n';
                if (cmd.element.class) html += 'Class: ' + escapeHtml(cmd.element.class) + '\n';
                if (cmd.element.bounds) {
                    html += 'Bounds: ' + cmd.element.bounds.x + ',' + cmd.element.bounds.y + ' ' + cmd.element.bounds.width + 'x' + cmd.element.bounds.height;
                }
                html += '</div>';
                html += '</div>';
            }

            panel.innerHTML = html;
        }

        // Format duration
        function formatDuration(ms) {
            if (!ms) return '-';
            if (ms < 1000) return ms + 'ms';
            if (ms < 60000) return (ms / 1000).toFixed(1) + 's';
            return Math.floor(ms / 60000) + 'm ' + Math.floor((ms % 60000) / 1000) + 's';
        }

        // Escape HTML
        function escapeHtml(text) {
            if (!text) return '';
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Filter flows
        document.querySelectorAll('.filter-btn').forEach(btn => {
            btn.addEventListener('click', function() {
                const filter = this.dataset.filter;

                // Update button states
                document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
                this.classList.add('active');

                // Filter flows
                document.querySelectorAll('.flow-item').forEach(item => {
                    const status = item.dataset.status;
                    if (filter === 'all' || status === filter) {
                        item.style.display = 'block';
                    } else {
                        item.style.display = 'none';
                    }
                });
            });
        });

        // Auto-expand first failed flow, or first flow if all passed
        (function() {
            const failedFlow = document.querySelector('.flow-item[data-status="failed"]');
            if (failedFlow) {
                const flowIndex = failedFlow.dataset.flowIndex;
                toggleFlow(parseInt(flowIndex));
            } else {
                const firstFlow = document.querySelector('.flow-item');
                if (firstFlow) {
                    toggleFlow(0);
                }
            }
        })();
    </script>
</body>
</html>
`
