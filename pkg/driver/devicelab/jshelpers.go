// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package devicelab

import _ "embed"

// browserJSHelper is injected into Chrome browser pages on Android.
// Separate copy from both the desktop browser driver's jsHelperCode and the mobile
// webViewJSHelper. Includes dialog overrides (page-level CDP deadlocks on native dialogs)
// and the full element-finding + visibility + polling helpers from the desktop driver.
//
//go:embed browser_jshelper.js
var browserJSHelper string

// webViewJSHelper is the JS helper injected into WebView pages.
// This is intentionally a separate copy from the desktop browser CDP driver's jsHelperCode.
// The two drivers (desktop browser vs mobile WebView) are independent — changes to
// desktop browser JS should never affect mobile WebView behavior, and vice versa.
//
//go:embed webview_jshelper.js
var webViewJSHelper string
