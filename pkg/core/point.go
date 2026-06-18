// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePointCoords parses a "x, y" coordinate string into absolute pixel values.
//
// If the string contains '%', values are treated as percentages of screenW/screenH.
// Otherwise, values are treated as absolute pixel coordinates.
//
// Both modes validate against screen bounds. screenW and screenH are always required.
//
// Examples:
//
//	"50%, 75%"  with screen 1080x1920 → (540, 1440, nil)
//	"123, 456"  with screen 1080x1920 → (123, 456, nil)
//	"9999, 456" with screen 1080x1920 → error: x exceeds screen width
func ParsePointCoords(coord string, screenW, screenH int) (int, int, error) {
	parts := strings.Split(coord, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected 'x, y' format, got: %s", coord)
	}

	if screenW <= 0 || screenH <= 0 {
		return 0, 0, fmt.Errorf("invalid screen dimensions: %dx%d", screenW, screenH)
	}

	xStr := strings.TrimSpace(parts[0])
	yStr := strings.TrimSpace(parts[1])

	if strings.Contains(coord, "%") {
		// Percentage mode
		xStr = strings.TrimSpace(strings.TrimSuffix(xStr, "%"))
		yStr = strings.TrimSpace(strings.TrimSuffix(yStr, "%"))

		xPct, err := strconv.ParseFloat(xStr, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid x percentage: %s", parts[0])
		}

		yPct, err := strconv.ParseFloat(yStr, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid y percentage: %s", parts[1])
		}

		if xPct < 0 || xPct > 100 {
			return 0, 0, fmt.Errorf("x percentage out of range (0-100): %.1f", xPct)
		}
		if yPct < 0 || yPct > 100 {
			return 0, 0, fmt.Errorf("y percentage out of range (0-100): %.1f", yPct)
		}

		x := int(float64(screenW) * xPct / 100.0)
		y := int(float64(screenH) * yPct / 100.0)
		return x, y, nil
	}

	// Absolute pixel mode
	x, err := strconv.Atoi(xStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x coordinate: %s", parts[0])
	}

	y, err := strconv.Atoi(yStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y coordinate: %s", parts[1])
	}

	if x < 0 || x > screenW {
		return 0, 0, fmt.Errorf("x coordinate %d out of screen bounds (0-%d)", x, screenW)
	}
	if y < 0 || y > screenH {
		return 0, 0, fmt.Errorf("y coordinate %d out of screen bounds (0-%d)", y, screenH)
	}

	return x, y, nil
}

// ParsePercentageCoords parses a "x%, y%" coordinate string into decimal fractions (0.0-1.0).
//
// Examples:
//
//	"50%, 50%"  → (0.5, 0.5, nil)
//	"25%, 75%"  → (0.25, 0.75, nil)
//	"100%, 0%"  → (1.0, 0.0, nil)
func ParsePercentageCoords(coord string) (float64, float64, error) {
	coord = strings.ReplaceAll(coord, " ", "")
	parts := strings.Split(coord, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinate format: %s", coord)
	}

	xStr := strings.TrimSuffix(parts[0], "%")
	yStr := strings.TrimSuffix(parts[1], "%")

	x, err := strconv.ParseFloat(xStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x coordinate: %s", parts[0])
	}

	y, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y coordinate: %s", parts[1])
	}

	return x / 100.0, y / 100.0, nil
}
