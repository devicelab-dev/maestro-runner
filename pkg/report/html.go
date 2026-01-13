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
	OutputPath  string // Path to write the HTML file
	EmbedAssets bool   // Embed screenshots as base64 (makes file larger but portable)
	Title       string // Report title (default: "Test Report")
	ReportDir   string // Directory containing report.json (needed for asset paths)
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
	if err := os.WriteFile(cfg.OutputPath, []byte(html), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}

	return nil
}

// HTMLData contains all data needed for the HTML template.
type HTMLData struct {
	Title         string
	GeneratedAt   string
	Index         *Index
	Flows         []FlowHTMLData
	TotalDuration string
	PassRate      float64
	MaxDuration   int64
	StatusClass   map[Status]string
	JSONData      template.JS // JSON data for JavaScript
}

// FlowHTMLData contains flow data formatted for HTML.
type FlowHTMLData struct {
	FlowDetail
	StatusClass string
	DurationStr string
	DurationMs  int64
	DurationPct float64
	Commands    []CommandHTMLData
}

// CommandHTMLData contains command data formatted for HTML.
type CommandHTMLData struct {
	Command
	StatusClass      string
	DurationStr      string
	ScreenshotBefore string // base64 or path
	ScreenshotAfter  string // base64 or path
	HasScreenshots   bool
}

func buildHTMLData(index *Index, flows []FlowDetail, cfg HTMLConfig) HTMLData {
	statusClass := map[Status]string{
		StatusPassed:  "passed",
		StatusFailed:  "failed",
		StatusSkipped: "skipped",
		StatusRunning: "running",
		StatusPending: "pending",
	}

	// Find max duration for percentage bars
	var maxDuration int64
	for _, entry := range index.Flows {
		if entry.Duration != nil && *entry.Duration > maxDuration {
			maxDuration = *entry.Duration
		}
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

		var durationMs int64
		var durationPct float64
		if index.Flows[i].Duration != nil {
			durationMs = *index.Flows[i].Duration
			if maxDuration > 0 {
				durationPct = float64(durationMs) / float64(maxDuration) * 100
			}
		}

		flowsData[i] = FlowHTMLData{
			FlowDetail:  f,
			StatusClass: statusClass[index.Flows[i].Status],
			DurationStr: formatDuration(index.Flows[i].Duration),
			DurationMs:  durationMs,
			DurationPct: durationPct,
			Commands:    cmds,
		}
	}

	// Calculate pass rate
	var passRate float64
	if index.Summary.Total > 0 {
		passRate = float64(index.Summary.Passed) / float64(index.Summary.Total) * 100
	}

	// Calculate total duration
	var totalDurationMs int64
	if index.EndTime != nil {
		totalDurationMs = index.EndTime.Sub(index.StartTime).Milliseconds()
	}

	// Serialize index and flows to JSON for JavaScript
	jsonBytes, _ := json.Marshal(map[string]interface{}{
		"index": index,
		"flows": flows,
	})

	return HTMLData{
		Title:         cfg.Title,
		GeneratedAt:   time.Now().Format("2006-01-02 15:04:05"),
		Index:         index,
		Flows:         flowsData,
		TotalDuration: formatDuration(&totalDurationMs),
		PassRate:      passRate,
		MaxDuration:   maxDuration,
		StatusClass:   statusClass,
		JSONData:      template.JS(jsonBytes),
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
            --bg-primary: #ffffff;
            --bg-secondary: #f9fafb;
            --bg-tertiary: #f3f4f6;
            --text-primary: #000000;
            --text-secondary: rgb(75, 85, 99);
            --text-muted: rgb(107, 114, 128);
            --border-color: #e5e7eb;
            --passed: #22c55e;
            --passed-bg: rgba(34, 197, 94, 0.1);
            --failed: #ef4444;
            --failed-bg: rgba(239, 68, 68, 0.08);
            --skipped: #eab308;
            --skipped-bg: rgba(234, 179, 8, 0.1);
            --running: #06b6d4;
            --pending: #6b7280;
            --accent: #06b6d4;
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

        /* Header */
        .header {
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border-color);
            padding: 16px 24px;
        }

        .header-top {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 16px;
        }

        .header-left {
            display: flex;
            align-items: center;
            gap: 20px;
        }

        .brand {
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .brand-icon {
            width: 36px;
            height: 36px;
            display: flex;
            align-items: center;
            justify-content: center;
        }

        .brand-icon svg {
            width: 20px;
            height: 20px;
            fill: white;
        }

        .brand-text {
            display: flex;
            flex-direction: column;
        }

        .brand-name {
            font-size: 15px;
            font-weight: 600;
            color: var(--text-primary);
        }

        .brand-by {
            font-size: 11px;
            color: var(--text-primary);
        }

        .brand-link {
            color: var(--accent);
            text-decoration: none;
        }

        .brand-link:hover {
            text-decoration: underline;
        }

        .header-divider {
            width: 1px;
            height: 28px;
            background: var(--border-color);
        }

        .header-title {
            display: flex;
            flex-direction: column;
        }

        .header-title-main {
            font-size: 16px;
            font-weight: 500;
        }

        .header-title-sub {
            font-size: 12px;
            color: var(--text-secondary);
        }

        .header-right {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .platform-badge {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 6px 14px;
            background: var(--accent);
            color: white;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
        }

        .platform-badge svg {
            width: 16px;
            height: 16px;
            fill: currentColor;
        }

        .github-star {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 6px 14px;
            background: var(--bg-primary);
            color: var(--text-primary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
            text-decoration: none;
            transition: border-color 0.2s;
        }

        .github-star:hover {
            border-color: var(--accent);
        }

        .github-star svg {
            width: 16px;
            height: 16px;
            fill: currentColor;
        }

        .github-star .star-icon {
            fill: #f59e0b;
        }

        /* Dashboard */
        .dashboard {
            display: flex;
            gap: 24px;
            flex-wrap: wrap;
            align-items: center;
        }

        /* Pie Chart */
        .chart-container {
            display: flex;
            align-items: center;
            gap: 16px;
        }

        .pie-chart {
            width: 80px;
            height: 80px;
            border-radius: 50%;
            position: relative;
        }

        .pie-center {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: var(--bg-secondary);
            width: 50px;
            height: 50px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
            font-weight: 600;
        }

        .chart-legend {
            display: flex;
            flex-direction: column;
            gap: 4px;
        }

        .legend-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 13px;
        }

        .legend-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
        }

        .legend-dot.passed { background: var(--passed); }
        .legend-dot.failed { background: var(--failed); }
        .legend-dot.skipped { background: var(--skipped); }

        /* Environment Card */
        .env-card {
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 12px 16px;
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 8px 24px;
            font-size: 13px;
        }

        .env-item {
            display: flex;
            gap: 8px;
        }

        .env-label {
            color: var(--text-muted);
            min-width: 60px;
        }

        .env-value {
            color: var(--text-primary);
            font-weight: 500;
        }

        /* Main Container */
        .main-container {
            display: flex;
            height: calc(100vh - 160px);
        }

        /* Flow List */
        .flow-list {
            width: 400px;
            border-right: 1px solid var(--border-color);
            overflow-y: auto;
            background: var(--bg-secondary);
        }

        .search-box {
            padding: 12px;
            border-bottom: 1px solid var(--border-color);
            position: sticky;
            top: 0;
            background: var(--bg-secondary);
            z-index: 10;
        }

        .search-input {
            width: 100%;
            padding: 8px 12px;
            border: 1px solid var(--border-color);
            border-radius: 6px;
            font-size: 13px;
            background: var(--bg-primary);
        }

        .search-input:focus {
            outline: none;
            border-color: var(--accent);
        }

        .filters {
            padding: 8px 12px;
            display: flex;
            gap: 8px;
            border-bottom: 1px solid var(--border-color);
            background: var(--bg-secondary);
            position: sticky;
            top: 49px;
            z-index: 10;
        }

        .filter-btn {
            padding: 4px 12px;
            border: 1px solid var(--border-color);
            border-radius: 16px;
            background: var(--bg-primary);
            font-size: 12px;
            cursor: pointer;
            transition: all 0.2s;
        }

        .filter-btn:hover {
            border-color: var(--accent);
        }

        .filter-btn.active {
            background: var(--accent);
            color: white;
            border-color: var(--accent);
        }

        .filter-btn.failed.active {
            background: var(--failed);
            border-color: var(--failed);
        }

        .keyboard-hint {
            padding: 6px 12px;
            font-size: 11px;
            color: var(--text-muted);
            background: var(--bg-tertiary);
            border-bottom: 1px solid var(--border-color);
        }

        .keyboard-hint kbd {
            background: var(--bg-primary);
            padding: 2px 6px;
            border-radius: 4px;
            border: 1px solid var(--border-color);
            font-family: inherit;
            font-size: 10px;
        }

        .flow-items {
            padding: 8px;
        }

        .flow-item {
            padding: 12px;
            margin-bottom: 8px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.2s;
        }

        .flow-item:hover {
            border-color: var(--accent);
        }

        .flow-item.selected {
            border-color: var(--accent);
            box-shadow: 0 0 0 2px rgba(6, 182, 212, 0.2);
        }

        .flow-item.failed {
            background: linear-gradient(90deg, var(--failed-bg) 0%, var(--bg-primary) 50%);
        }

        .flow-item-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 6px;
        }

        .status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            flex-shrink: 0;
            margin-top: 2px;
        }

        .status-dot.passed { background: var(--passed); }
        .status-dot.failed { background: var(--failed); }
        .status-dot.skipped { background: var(--skipped); }
        .status-dot.running {
            background: transparent;
            border: 2.5px solid var(--running);
            border-top-color: transparent;
            animation: spin 0.8s linear infinite;
        }
        .status-dot.pending { background: var(--pending); }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }

        .flow-name {
            font-size: 14px;
            font-weight: 500;
            flex: 1;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .flow-meta {
            display: flex;
            align-items: center;
            gap: 12px;
            font-size: 12px;
            color: var(--text-muted);
        }

        .duration-bar {
            flex: 1;
            height: 4px;
            background: var(--bg-tertiary);
            border-radius: 2px;
            overflow: hidden;
        }

        .duration-fill {
            height: 100%;
            background: var(--accent);
            border-radius: 2px;
        }

        /* Detail Panel */
        .detail-panel {
            flex: 1;
            overflow-y: auto;
            padding: 24px;
        }

        .detail-header {
            margin-bottom: 24px;
        }

        .detail-title {
            font-size: 20px;
            font-weight: 600;
            margin-bottom: 8px;
        }

        .detail-info {
            display: flex;
            gap: 24px;
            margin-bottom: 24px;
            padding: 16px;
            background: var(--bg-secondary);
            border-radius: 8px;
        }

        .info-item {
            display: flex;
            flex-direction: column;
            gap: 4px;
        }

        .info-label {
            font-size: 11px;
            color: var(--text-muted);
            text-transform: uppercase;
        }

        .info-value {
            font-size: 14px;
            font-weight: 500;
        }

        /* Command List - Compact with expand */
        .command-list {
            display: flex;
            flex-direction: column;
            gap: 1px;
            background: var(--border-color);
            border-radius: 8px;
            overflow: hidden;
        }

        .command-item {
            background: var(--bg-primary);
            cursor: pointer;
            transition: background 0.15s;
        }

        .command-item:hover {
            background: var(--bg-secondary);
        }

        .command-item.failed {
            background: var(--failed-bg);
        }

        .command-item.failed:hover {
            background: rgba(239, 68, 68, 0.12);
        }

        .command-summary {
            display: flex;
            align-items: center;
            padding: 8px 12px;
            gap: 8px;
        }

        .command-status {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            flex-shrink: 0;
        }

        .command-status.passed { background: var(--passed); }
        .command-status.failed { background: var(--failed); }
        .command-status.skipped { background: var(--skipped); }
        .command-status.running {
            background: transparent;
            border: 2px solid var(--running);
            border-top-color: transparent;
            animation: spin 0.8s linear infinite;
        }
        .command-status.pending { background: var(--pending); }

        .command-type {
            font-size: 13px;
            font-weight: 500;
            color: var(--passed);
            min-width: 100px;
        }

        .command-item.failed .command-type {
            color: var(--failed);
        }

        .command-value {
            flex: 1;
            font-size: 13px;
            color: var(--text-secondary);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .command-duration {
            font-size: 12px;
            color: var(--text-muted);
            min-width: 50px;
            text-align: right;
        }

        .command-expand-icon {
            color: var(--text-muted);
            font-size: 10px;
            transition: transform 0.2s;
        }

        .command-item.expanded .command-expand-icon {
            transform: rotate(90deg);
        }

        .command-details {
            display: none;
            padding: 0 12px 12px 28px;
            border-top: 1px solid var(--border-color);
            background: var(--bg-secondary);
        }

        .command-item.expanded > .command-details {
            display: block;
        }

        /* When command-details only contains sub-commands, remove padding */
        .command-details:has(> .sub-commands:only-child) {
            padding: 0;
            border-top: none;
            background: transparent;
        }

        .sub-commands {
            display: flex;
            flex-direction: column;
            gap: 1px;
            background: var(--border-color);
            margin-left: 24px;
        }

        .command-yaml {
            font-family: 'SF Mono', Monaco, Consolas, monospace;
            font-size: 12px;
            color: var(--text-secondary);
            white-space: pre-wrap;
            word-break: break-all;
            background: var(--bg-tertiary);
            padding: 8px;
            border-radius: 4px;
            margin-top: 8px;
        }

        .command-error {
            margin-top: 8px;
            padding: 10px;
            background: var(--failed-bg);
            border: 1px solid var(--failed);
            border-radius: 6px;
        }

        .error-type {
            font-size: 11px;
            font-weight: 600;
            color: var(--failed);
            text-transform: uppercase;
            margin-bottom: 4px;
        }

        .error-message {
            font-size: 13px;
            color: var(--text-primary);
            margin-bottom: 6px;
        }

        .error-suggestion {
            font-size: 12px;
            color: var(--text-secondary);
            font-style: italic;
        }

        .command-screenshots {
            display: flex;
            gap: 12px;
            margin-top: 8px;
        }

        .screenshot {
            max-width: 150px;
            border-radius: 6px;
            border: 1px solid var(--border-color);
            cursor: pointer;
        }

        .screenshot:hover {
            border-color: var(--accent);
        }

        .command-element {
            margin-top: 8px;
            font-size: 12px;
            color: var(--text-muted);
        }

        .command-element span {
            color: var(--text-secondary);
        }

        /* Empty State */
        .empty-state {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: 100%;
            color: var(--text-muted);
        }

        .empty-state-icon {
            font-size: 48px;
            margin-bottom: 16px;
        }

        /* Image Modal */
        .image-modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0, 0, 0, 0.9);
            z-index: 1000;
            align-items: center;
            justify-content: center;
            cursor: pointer;
        }

        .image-modal.active {
            display: flex;
        }

        .image-modal img {
            max-width: 90%;
            max-height: 90%;
            border-radius: 8px;
        }

    </style>
</head>
<body>
    <!-- Header with Dashboard -->
    <div class="header">
        <div class="header-top">
            <div class="header-left">
                <div class="brand">
                    <div class="brand-icon">
                        <img src="data:image/jpeg;base64,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" alt="DeviceLab" style="width:32px;height:32px;" />
                    </div>
                    <div class="brand-text">
                        <span class="brand-name">maestro-runner</span>
                        <span class="brand-by">by <a href="https://devicelab.dev/" target="_blank" class="brand-link">DeviceLab</a></span>
                    </div>
                </div>
                <div class="header-divider"></div>
                <div class="header-title">
                    <span class="header-title-main">{{.Title}}</span>
                    <span class="header-title-sub">{{.GeneratedAt}}</span>
                </div>
            </div>
            <div class="header-right">
                <div class="platform-badge">
                    {{if eq .Index.Device.Platform "android"}}
                    <svg viewBox="0 0 24 24"><path d="M17.6 9.48l1.84-3.18c.16-.31.04-.69-.26-.85-.29-.15-.65-.06-.83.22l-1.88 3.24c-1.4-.59-2.95-.92-4.62-.92-1.67 0-3.22.33-4.62.92L5.26 5.67c-.18-.28-.54-.37-.83-.22-.3.16-.42.54-.26.85L6.4 9.48C3.3 11.25 1.28 14.44 1 18h22c-.28-3.56-2.3-6.75-5.4-8.52zM7 15.25c-.69 0-1.25-.56-1.25-1.25s.56-1.25 1.25-1.25 1.25.56 1.25 1.25-.56 1.25-1.25 1.25zm10 0c-.69 0-1.25-.56-1.25-1.25s.56-1.25 1.25-1.25 1.25.56 1.25 1.25-.56 1.25-1.25 1.25z"/></svg>
                    {{else}}
                    <svg viewBox="0 0 24 24"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>
                    {{end}}
                    <span>{{if eq .Index.Device.Platform "android"}}Android{{else}}iOS{{end}}</span>
                </div>
                <a href="https://github.com/AlessandroOddworx/maestro-runner" target="_blank" class="github-star">
                    <svg viewBox="0 0 16 16"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
                    <svg class="star-icon" viewBox="0 0 16 16"><path d="M8 .25a.75.75 0 01.673.418l1.882 3.815 4.21.612a.75.75 0 01.416 1.279l-3.046 2.97.719 4.192a.75.75 0 01-1.088.791L8 12.347l-3.766 1.98a.75.75 0 01-1.088-.79l.72-4.194L.818 6.374a.75.75 0 01.416-1.28l4.21-.611L7.327.668A.75.75 0 018 .25z"/></svg>
                    <span>Star</span>
                </a>
            </div>
        </div>
        <div class="dashboard">
            <!-- Pie Chart -->
            <div class="chart-container">
                <div class="pie-chart" id="pie-chart"></div>
                <div class="chart-legend" id="chart-legend">
                    <div class="legend-item" id="legend-running" style="display: none;">
                        <span class="legend-dot" style="background: var(--running);"></span>
                        <span>0 running</span>
                    </div>
                    <div class="legend-item" id="legend-passed">
                        <span class="legend-dot passed"></span>
                        <span>{{.Index.Summary.Passed}} passed</span>
                    </div>
                    <div class="legend-item" id="legend-failed">
                        <span class="legend-dot failed"></span>
                        <span>{{.Index.Summary.Failed}} failed</span>
                    </div>
                    <div class="legend-item" id="legend-skipped" style="{{if eq .Index.Summary.Skipped 0}}display: none;{{end}}">
                        <span class="legend-dot skipped"></span>
                        <span>{{.Index.Summary.Skipped}} skipped</span>
                    </div>
                </div>
            </div>

            <!-- Environment Card -->
            <div class="env-card">
                <div class="env-item">
                    <span class="env-label">Device</span>
                    <span class="env-value">{{.Index.Device.Name}}</span>
                </div>
                <div class="env-item">
                    <span class="env-label">Platform</span>
                    <span class="env-value">{{.Index.Device.Platform}} {{.Index.Device.OSVersion}}</span>
                </div>
                <div class="env-item">
                    <span class="env-label">App</span>
                    <span class="env-value">{{if .Index.App.Name}}{{.Index.App.Name}}{{else}}{{.Index.App.ID}}{{end}}</span>
                </div>
                <div class="env-item">
                    <span class="env-label">Driver</span>
                    <span class="env-value">{{.Index.MaestroRunner.Driver}}</span>
                </div>
            </div>
        </div>
    </div>

    <!-- Main Container -->
    <div class="main-container">
        <!-- Flow List -->
        <div class="flow-list">
            <div class="search-box">
                <input type="text" class="search-input" id="search-input" placeholder="Search tests... (Press /)" />
            </div>
            <div class="filters">
                <button class="filter-btn active" data-filter="all">All ({{.Index.Summary.Total}})</button>
                <button class="filter-btn failed" data-filter="failed">Failed ({{.Index.Summary.Failed}})</button>
                <button class="filter-btn" data-filter="passed">Passed ({{.Index.Summary.Passed}})</button>
            </div>
            <div class="keyboard-hint">
                <kbd>j</kbd>/<kbd>k</kbd> navigate &nbsp; <kbd>n</kbd> next failure &nbsp; <kbd>Enter</kbd> expand
            </div>
            <div class="flow-items" id="flow-items">
                {{range $fi, $flow := .Flows}}
                <div class="flow-item {{$flow.StatusClass}}" data-flow-index="{{$fi}}" data-status="{{$flow.StatusClass}}" data-name="{{$flow.Name}}">
                    <div class="flow-item-header">
                        <span class="status-dot {{$flow.StatusClass}}"></span>
                        <span class="flow-name">{{$flow.Name}}</span>
                    </div>
                    <div class="flow-meta">
                        <span>{{len $flow.Commands}} steps</span>
                        <div class="duration-bar">
                            <div class="duration-fill" style="width: {{printf "%.1f" $flow.DurationPct}}%"></div>
                        </div>
                        <span>{{$flow.DurationStr}}</span>
                    </div>
                </div>
                {{end}}
            </div>
        </div>

        <!-- Detail Panel -->
        <div class="detail-panel" id="detail-panel">
            <div class="empty-state" id="empty-state">
                <div class="empty-state-icon">📋</div>
                <div>Select a test to view details</div>
            </div>
            <div id="detail-content" style="display: none;">
                <div class="detail-header">
                    <div class="detail-title" id="detail-title"></div>
                </div>
                <div class="detail-info" id="detail-info"></div>
                <div class="command-list" id="command-list"></div>
            </div>
        </div>
    </div>

    <!-- Image Modal -->
    <div class="image-modal" id="image-modal" onclick="closeModal()">
        <img id="modal-image" src="" alt="Screenshot">
    </div>


    <script>
        // Initial data from generation time
        let reportData = {{.JSONData}};
        let selectedFlowIndex = -1;

        // Live update tracking
        let lastUpdateSeq = reportData.index.updateSeq || 0;
        let lastFlowSeq = {};
        reportData.index.flows.forEach(f => { lastFlowSeq[f.id] = f.updateSeq || 0; });
        let isPolling = true;
        const POLL_INTERVAL = 500; // ms

        // Update pie chart and legend
        function updatePieChart() {
            const total = reportData.index.summary.total || 1;
            const passed = reportData.index.summary.passed || 0;
            const failed = reportData.index.summary.failed || 0;
            const skipped = reportData.index.summary.skipped || 0;
            const running = reportData.index.summary.running || 0;

            const passedPct = (passed / total) * 100;
            const failedPct = (failed / total) * 100;
            const skippedPct = (skipped / total) * 100;
            const runningPct = (running / total) * 100;

            const pieChart = document.getElementById('pie-chart');
            pieChart.style.background = 'conic-gradient(' +
                'var(--passed) 0% ' + passedPct + '%, ' +
                'var(--failed) ' + passedPct + '% ' + (passedPct + failedPct) + '%, ' +
                'var(--running) ' + (passedPct + failedPct) + '% ' + (passedPct + failedPct + runningPct) + '%, ' +
                'var(--skipped) ' + (passedPct + failedPct + runningPct) + '% ' + (passedPct + failedPct + runningPct + skippedPct) + '%, ' +
                'var(--pending) ' + (passedPct + failedPct + runningPct + skippedPct) + '% 100%)';

            pieChart.innerHTML = '<div class="pie-center">' + Math.round(passedPct) + '%</div>';

            // Update legend
            document.getElementById('legend-passed').querySelector('span:last-child').textContent = passed + ' passed';
            document.getElementById('legend-failed').querySelector('span:last-child').textContent = failed + ' failed';

            const runningEl = document.getElementById('legend-running');
            if (running > 0) {
                runningEl.style.display = '';
                runningEl.querySelector('span:last-child').textContent = running + ' running';
            } else {
                runningEl.style.display = 'none';
            }

            const skippedEl = document.getElementById('legend-skipped');
            if (skipped > 0) {
                skippedEl.style.display = '';
                skippedEl.querySelector('span:last-child').textContent = skipped + ' skipped';
            } else {
                skippedEl.style.display = 'none';
            }
        }
        updatePieChart();

        // Update flow list items from index data
        function updateFlowList() {
            const flowItems = document.querySelectorAll('.flow-item');
            flowItems.forEach((item, i) => {
                const entry = reportData.index.flows[i];
                if (!entry) return;

                // Update status classes
                const oldStatus = item.dataset.status;
                const newStatus = entry.status;
                if (oldStatus !== newStatus) {
                    item.classList.remove(oldStatus);
                    item.classList.add(newStatus);
                    item.dataset.status = newStatus;

                    // Update status dot
                    const dot = item.querySelector('.status-dot');
                    if (dot) {
                        dot.classList.remove(oldStatus);
                        dot.classList.add(newStatus);
                    }
                }

                // Update duration
                const durationSpan = item.querySelector('.flow-meta span:last-child');
                if (durationSpan && entry.duration) {
                    durationSpan.textContent = formatDuration(entry.duration);
                }
            });

            // Update filter button counts
            const allBtn = document.querySelector('.filter-btn[data-filter="all"]');
            const failedBtn = document.querySelector('.filter-btn[data-filter="failed"]');
            const passedBtn = document.querySelector('.filter-btn[data-filter="passed"]');
            if (allBtn) allBtn.textContent = 'All (' + reportData.index.summary.total + ')';
            if (failedBtn) failedBtn.textContent = 'Failed (' + reportData.index.summary.failed + ')';
            if (passedBtn) passedBtn.textContent = 'Passed (' + reportData.index.summary.passed + ')';
        }

        // Update detail panel if a flow changed
        function updateDetailIfNeeded(changedFlowIds) {
            if (selectedFlowIndex < 0) return;
            const selectedFlow = reportData.index.flows[selectedFlowIndex];
            if (selectedFlow && changedFlowIds.includes(selectedFlow.id)) {
                renderDetail(selectedFlowIndex);
            }
        }

        // Fetch updated report.json and flow details
        async function poll() {
            if (!isPolling) return;

            try {
                // Fetch index
                const indexResp = await fetch('report.json?t=' + Date.now());
                if (!indexResp.ok) {
                    schedulePoll();
                    return;
                }
                const newIndex = await indexResp.json();

                // Check if anything changed
                if (newIndex.updateSeq === lastUpdateSeq) {
                    schedulePoll();
                    return;
                }

                // Find changed flows
                const changedFlowIds = [];
                for (const entry of newIndex.flows) {
                    const oldSeq = lastFlowSeq[entry.id] || 0;
                    if (entry.updateSeq > oldSeq) {
                        changedFlowIds.push(entry.id);
                        lastFlowSeq[entry.id] = entry.updateSeq;
                    }
                }

                // Fetch changed flow details
                for (const flowId of changedFlowIds) {
                    const flowIdx = newIndex.flows.findIndex(f => f.id === flowId);
                    if (flowIdx >= 0) {
                        const flowResp = await fetch(newIndex.flows[flowIdx].dataFile + '?t=' + Date.now());
                        if (flowResp.ok) {
                            reportData.flows[flowIdx] = await flowResp.json();
                        }
                    }
                }

                // Update local data
                reportData.index = newIndex;
                lastUpdateSeq = newIndex.updateSeq;

                // Update UI
                updatePieChart();
                updateFlowList();
                updateDetailIfNeeded(changedFlowIds);

                // Stop polling if terminal
                if (isTerminalStatus(newIndex.status)) {
                    isPolling = false;
                    return;
                }
            } catch (e) {
                // Ignore fetch errors, will retry
            }

            schedulePoll();
        }

        function isTerminalStatus(status) {
            return status === 'passed' || status === 'failed' || status === 'skipped';
        }

        function schedulePoll() {
            if (isPolling) {
                setTimeout(poll, POLL_INTERVAL);
            }
        }

        // Start polling if served via HTTP (file:// users can refresh manually)
        if (window.location.protocol !== 'file:' && !isTerminalStatus(reportData.index.status)) {
            schedulePoll();
        } else {
            isPolling = false;
        }

        // Flow item click handlers
        document.querySelectorAll('.flow-item').forEach(item => {
            item.addEventListener('click', () => selectFlow(parseInt(item.dataset.flowIndex)));
        });

        // Filter buttons
        document.querySelectorAll('.filter-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                filterFlows(btn.dataset.filter);
            });
        });

        // Search
        document.getElementById('search-input').addEventListener('input', (e) => {
            const query = e.target.value.toLowerCase();
            document.querySelectorAll('.flow-item').forEach(item => {
                const name = item.dataset.name.toLowerCase();
                item.style.display = name.includes(query) ? '' : 'none';
            });
        });

        // Keyboard navigation
        document.addEventListener('keydown', (e) => {
            if (e.target.tagName === 'INPUT') {
                if (e.key === 'Escape') e.target.blur();
                return;
            }

            const visibleFlows = Array.from(document.querySelectorAll('.flow-item')).filter(f => f.style.display !== 'none');
            const currentIdx = visibleFlows.findIndex(f => f.classList.contains('selected'));

            switch (e.key) {
                case '/':
                    e.preventDefault();
                    document.getElementById('search-input').focus();
                    break;
                case 'j':
                    if (currentIdx < visibleFlows.length - 1) {
                        selectFlow(parseInt(visibleFlows[currentIdx + 1].dataset.flowIndex));
                    } else if (currentIdx === -1 && visibleFlows.length > 0) {
                        selectFlow(parseInt(visibleFlows[0].dataset.flowIndex));
                    }
                    break;
                case 'k':
                    if (currentIdx > 0) {
                        selectFlow(parseInt(visibleFlows[currentIdx - 1].dataset.flowIndex));
                    }
                    break;
                case 'n':
                    const failedFlows = visibleFlows.filter(f => f.dataset.status === 'failed');
                    if (failedFlows.length > 0) {
                        const currentFailedIdx = failedFlows.findIndex(f => f.classList.contains('selected'));
                        const nextIdx = (currentFailedIdx + 1) % failedFlows.length;
                        selectFlow(parseInt(failedFlows[nextIdx].dataset.flowIndex));
                    }
                    break;
                case 'Escape':
                    closeModal();
                    break;
            }
        });

        function filterFlows(filter) {
            document.querySelectorAll('.flow-item').forEach(item => {
                if (filter === 'all') {
                    item.style.display = '';
                } else {
                    item.style.display = item.dataset.status === filter ? '' : 'none';
                }
            });
        }

        function selectFlow(index) {
            selectedFlowIndex = index;
            document.querySelectorAll('.flow-item').forEach(f => f.classList.remove('selected'));
            const item = document.querySelector('.flow-item[data-flow-index="' + index + '"]');
            if (item) {
                item.classList.add('selected');
                item.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            }

            // Update URL hash
            window.location.hash = 'flow-' + index;

            renderDetail(index);
        }

        function renderDetail(flowIndex) {
            const flow = reportData.flows[flowIndex];
            const indexEntry = reportData.index.flows[flowIndex];
            if (!flow) return;

            document.getElementById('empty-state').style.display = 'none';
            document.getElementById('detail-content').style.display = '';

            document.getElementById('detail-title').textContent = flow.name;

            // Info section
            const infoHtml = '<div class="info-item"><span class="info-label">Status</span><span class="info-value">' + indexEntry.status + '</span></div>' +
                '<div class="info-item"><span class="info-label">Duration</span><span class="info-value">' + formatDuration(indexEntry.duration) + '</span></div>' +
                '<div class="info-item"><span class="info-label">Steps</span><span class="info-value">' + flow.commands.length + '</span></div>' +
                '<div class="info-item"><span class="info-label">Source</span><span class="info-value">' + flow.sourceFile + '</span></div>';
            document.getElementById('detail-info').innerHTML = infoHtml;

            // Commands - compact format with sub-commands support
            document.getElementById('command-list').innerHTML = renderCommands(flow.commands, flowIndex, 0);
        }

        function renderCommands(commands, flowIndex, depth) {
            let html = '';
            commands.forEach((cmd, i) => {
                html += renderCommand(cmd, flowIndex, i, depth);
            });
            return html;
        }

        function renderCommand(cmd, flowIndex, index, depth) {
            const status = cmd.status || 'pending';
            const keyValue = extractKeyValue(cmd);
            const hasSubCommands = cmd.subCommands && cmd.subCommands.length > 0;
            const hasDetails = cmd.yaml || cmd.error || (cmd.artifacts && (cmd.artifacts.screenshotBefore || cmd.artifacts.screenshotAfter));
            const isExpandable = hasDetails || hasSubCommands;

            let html = '<div class="command-item ' + status + (hasSubCommands ? ' has-subcommands' : '') + '" id="flow-' + flowIndex + '-cmd-' + index + '-d' + depth + '" onclick="toggleCommand(this, event)">';

            // Summary line (always visible)
            html += '<div class="command-summary">' +
                '<span class="command-status ' + status + '"></span>' +
                '<span class="command-type">' + escapeHtml(cmd.type) + '</span>' +
                '<span class="command-value">' + escapeHtml(keyValue) + '</span>' +
                '<span class="command-duration">' + formatDuration(cmd.duration) + '</span>';
            if (isExpandable) {
                html += '<span class="command-expand-icon">▶</span>';
            }
            html += '</div>';

            // Details and sub-commands (hidden by default)
            if (isExpandable) {
                html += '<div class="command-details">';

                if (cmd.yaml && !hasSubCommands) {
                    html += '<div class="command-yaml">' + escapeHtml(cmd.yaml) + '</div>';
                }

                if (cmd.error) {
                    html += '<div class="command-error">' +
                        '<div class="error-type">' + escapeHtml(cmd.error.type) + '</div>' +
                        '<div class="error-message">' + escapeHtml(cmd.error.message) + '</div>';
                    if (cmd.error.suggestion) {
                        html += '<div class="error-suggestion">💡 ' + escapeHtml(cmd.error.suggestion) + '</div>';
                    }
                    html += '</div>';
                }

                if (cmd.artifacts && (cmd.artifacts.screenshotBefore || cmd.artifacts.screenshotAfter)) {
                    html += '<div class="command-screenshots">';
                    if (cmd.artifacts.screenshotBefore) {
                        html += '<img class="screenshot" src="' + cmd.artifacts.screenshotBefore + '" onclick="event.stopPropagation(); openModal(this.src)" title="Before">';
                    }
                    if (cmd.artifacts.screenshotAfter) {
                        html += '<img class="screenshot" src="' + cmd.artifacts.screenshotAfter + '" onclick="event.stopPropagation(); openModal(this.src)" title="After">';
                    }
                    html += '</div>';
                }

                // Render sub-commands recursively
                if (hasSubCommands) {
                    html += '<div class="sub-commands">';
                    html += renderCommands(cmd.subCommands, flowIndex, depth + 1);
                    html += '</div>';
                }

                html += '</div>';
            }

            html += '</div>';
            return html;
        }

        function extractKeyValue(cmd) {
            // Extract the most meaningful value to show in the summary
            if (cmd.params) {
                if (cmd.params.selector && cmd.params.selector.value) {
                    return cmd.params.selector.value;
                }
                if (cmd.params.text) {
                    return cmd.params.text;
                }
                if (cmd.params.direction) {
                    return cmd.params.direction;
                }
            }
            // Fallback: try to extract from label or return empty
            if (cmd.label && cmd.label !== cmd.type) {
                return cmd.label;
            }
            return '';
        }

        function toggleCommand(element, event) {
            if (event) event.stopPropagation();
            element.classList.toggle('expanded');
        }

        function formatDuration(ms) {
            if (!ms) return '-';
            if (ms < 1000) return ms + 'ms';
            if (ms < 60000) return (ms / 1000).toFixed(1) + 's';
            return Math.floor(ms / 60000) + 'm ' + Math.floor((ms % 60000) / 1000) + 's';
        }

        function escapeHtml(text) {
            if (!text) return '';
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function openModal(src) {
            document.getElementById('modal-image').src = src;
            document.getElementById('image-modal').classList.add('active');
        }

        function closeModal() {
            document.getElementById('image-modal').classList.remove('active');
        }

        // Handle URL hash for deep links
        function handleHash() {
            const hash = window.location.hash;
            if (hash.startsWith('#flow-')) {
                const parts = hash.substring(1).split('-');
                if (parts.length >= 2) {
                    const flowIndex = parseInt(parts[1]);
                    selectFlow(flowIndex);

                    if (parts.length >= 4 && parts[2] === 'cmd') {
                        const cmdIndex = parseInt(parts[3]);
                        setTimeout(() => {
                            const cmdEl = document.getElementById('flow-' + flowIndex + '-cmd-' + cmdIndex);
                            if (cmdEl) {
                                cmdEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
                                cmdEl.style.boxShadow = '0 0 0 2px var(--accent)';
                            }
                        }, 100);
                    }
                }
            }
        }

        window.addEventListener('hashchange', handleHash);

        // Select first flow by default if no hash
        if (!window.location.hash && reportData.flows.length > 0) {
            selectFlow(0);
        } else {
            handleHash();
        }
    </script>
</body>
</html>
`
