// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"testing"
)

func TestParsePointCoords(t *testing.T) {
	tests := []struct {
		name    string
		coord   string
		screenW int
		screenH int
		wantX   int
		wantY   int
		wantErr bool
	}{
		// Percentage coordinates
		{
			name:    "percentage 50%, 50%",
			coord:   "50%, 50%",
			screenW: 1080, screenH: 1920,
			wantX: 540, wantY: 960,
		},
		{
			name:    "percentage 0%, 0%",
			coord:   "0%, 0%",
			screenW: 1080, screenH: 1920,
			wantX: 0, wantY: 0,
		},
		{
			name:    "percentage 100%, 100%",
			coord:   "100%, 100%",
			screenW: 1080, screenH: 1920,
			wantX: 1080, wantY: 1920,
		},
		{
			name:    "percentage with spaces",
			coord:   " 85% , 50% ",
			screenW: 1080, screenH: 1920,
			wantX: 918, wantY: 960,
		},
		{
			name:    "percentage 25%, 75%",
			coord:   "25%, 75%",
			screenW: 400, screenH: 800,
			wantX: 100, wantY: 600,
		},

		// Absolute pixel coordinates
		{
			name:    "absolute 123, 456",
			coord:   "123, 456",
			screenW: 1080, screenH: 1920,
			wantX: 123, wantY: 456,
		},
		{
			name:    "absolute 0, 0",
			coord:   "0, 0",
			screenW: 1080, screenH: 1920,
			wantX: 0, wantY: 0,
		},
		{
			name:    "absolute with spaces",
			coord:   " 500 , 1000 ",
			screenW: 1080, screenH: 1920,
			wantX: 500, wantY: 1000,
		},
		{
			name:    "absolute no spaces",
			coord:   "200,300",
			screenW: 1080, screenH: 1920,
			wantX: 200, wantY: 300,
		},
		{
			name:    "absolute at screen edge",
			coord:   "1080, 1920",
			screenW: 1080, screenH: 1920,
			wantX: 1080, wantY: 1920,
		},

		// Error cases
		{
			name:    "no comma",
			coord:   "123 456",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "empty string",
			coord:   "",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "three values",
			coord:   "1, 2, 3",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "invalid x percentage",
			coord:   "abc%, 50%",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "invalid y percentage",
			coord:   "50%, xyz%",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "invalid x absolute",
			coord:   "abc, 456",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "invalid y absolute",
			coord:   "123, abc",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "float absolute treated as error",
			coord:   "12.5, 45.6",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},

		// Bounds validation: negative absolute pixels
		{
			name:    "negative x absolute",
			coord:   "-10, 456",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "negative y absolute",
			coord:   "123, -20",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "x beyond screen width",
			coord:   "999, 100",
			screenW: 390, screenH: 844,
			wantErr: true,
		},
		{
			name:    "y beyond screen height",
			coord:   "100, 999",
			screenW: 390, screenH: 844,
			wantErr: true,
		},

		// Bounds validation: percentage out of range
		{
			name:    "x percentage over 100",
			coord:   "150%, 50%",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "y percentage over 100",
			coord:   "50%, 200%",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "negative x percentage",
			coord:   "-10%, 50%",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "negative y percentage",
			coord:   "50%, -5%",
			screenW: 1080, screenH: 1920,
			wantErr: true,
		},

		// Invalid screen dimensions
		{
			name:    "zero screen dimensions",
			coord:   "123, 456",
			screenW: 0, screenH: 0,
			wantErr: true,
		},
		{
			name:    "zero screen with percentage",
			coord:   "50%, 50%",
			screenW: 0, screenH: 0,
			wantErr: true,
		},

		// Mixed notation (one percentage, one absolute) - should error
		{
			name:    "mixed notation x% y absolute",
			coord:   "50%, 123",
			screenW: 1080, screenH: 1920,
			wantErr: true, // enters percentage mode, 123 > 100 range
		},
		{
			name:    "mixed notation x absolute y%",
			coord:   "123, 50%",
			screenW: 1080, screenH: 1920,
			wantErr: true, // enters percentage mode, 123 parsed as pct > 100
		},

		// Negative screen dimensions
		{
			name:    "negative screen width",
			coord:   "50%, 50%",
			screenW: -100, screenH: 1920,
			wantErr: true,
		},
		{
			name:    "negative screen height",
			coord:   "123, 456",
			screenW: 1080, screenH: -200,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, err := ParsePointCoords(tt.coord, tt.screenW, tt.screenH)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePointCoords(%q) expected error, got (%d, %d)", tt.coord, x, y)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePointCoords(%q) unexpected error: %v", tt.coord, err)
			}
			if x != tt.wantX || y != tt.wantY {
				t.Errorf("ParsePointCoords(%q) = (%d, %d), want (%d, %d)", tt.coord, x, y, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestParsePercentageCoords(t *testing.T) {
	tests := []struct {
		name    string
		coord   string
		wantX   float64
		wantY   float64
		wantErr bool
	}{
		// Valid cases
		{
			name:  "50%, 50%",
			coord: "50%, 50%",
			wantX: 0.5, wantY: 0.5,
		},
		{
			name:  "0%, 0%",
			coord: "0%, 0%",
			wantX: 0.0, wantY: 0.0,
		},
		{
			name:  "100%, 100%",
			coord: "100%, 100%",
			wantX: 1.0, wantY: 1.0,
		},
		{
			name:  "25%, 75%",
			coord: "25%, 75%",
			wantX: 0.25, wantY: 0.75,
		},
		{
			name:  "with spaces",
			coord: " 50% , 50% ",
			wantX: 0.5, wantY: 0.5,
		},
		{
			name:  "no spaces",
			coord: "10%,90%",
			wantX: 0.1, wantY: 0.9,
		},
		{
			name:  "fractional percentages",
			coord: "33.3%, 66.6%",
			wantX: 0.333, wantY: 0.666,
		},

		// Error cases
		{
			name:    "no comma",
			coord:   "50% 50%",
			wantErr: true,
		},
		{
			name:    "empty string",
			coord:   "",
			wantErr: true,
		},
		{
			name:    "three values",
			coord:   "50%, 50%, 50%",
			wantErr: true,
		},
		{
			name:    "invalid x",
			coord:   "abc%, 50%",
			wantErr: true,
		},
		{
			name:    "invalid y",
			coord:   "50%, xyz%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, err := ParsePercentageCoords(tt.coord)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePercentageCoords(%q) expected error, got (%f, %f)", tt.coord, x, y)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePercentageCoords(%q) unexpected error: %v", tt.coord, err)
			}
			const epsilon = 0.001
			if diff := x - tt.wantX; diff > epsilon || diff < -epsilon {
				t.Errorf("ParsePercentageCoords(%q) x = %f, want %f", tt.coord, x, tt.wantX)
			}
			if diff := y - tt.wantY; diff > epsilon || diff < -epsilon {
				t.Errorf("ParsePercentageCoords(%q) y = %f, want %f", tt.coord, y, tt.wantY)
			}
		})
	}
}
