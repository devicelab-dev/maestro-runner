// Package cdp provides a browser automation driver using Rod (go-rod/rod) + CDP.
package cdp

import _ "embed"

// jsHelperCode is injected via page.EvalOnNewDocument() to persist across navigations.
// This is the last-resort fallback for finding elements when both the AX tree
// and page.Search() miss (rare, ~5% of cases).
//
//go:embed jshelper.js
var jsHelperCode string
