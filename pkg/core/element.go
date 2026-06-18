// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

// Element is an abstraction over native (UIAutomator2) and web (Rod/CDP) elements.
// Used by commands that need to interact with the focused element (inputText, eraseText, etc.)
type Element interface {
	// Info returns the element's metadata (text, bounds, visibility, etc.)
	Info() *ElementInfo

	// Text returns the element's text content.
	Text() (string, error)

	// Input types text into the element.
	Input(text string) error

	// Clear clears the element's text content.
	Clear() error
}
