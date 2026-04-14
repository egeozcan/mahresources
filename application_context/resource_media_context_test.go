package application_context

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/disintegration/imaging"
	"mahresources/models"
)

func TestGetJPEGQuality(t *testing.T) {
	tests := []struct {
		name     string
		width    uint
		height   uint
		expected int
	}{
		// Test smallest dimension threshold (≤100)
		{"100x100", 100, 100, 70},
		{"50x50", 50, 50, 70},
		{"100x50", 100, 50, 70},
		{"50x100", 50, 100, 70},

		// Test 101-200 threshold
		{"150x150", 150, 150, 75},
		{"200x200", 200, 200, 75},
		{"101x50", 101, 50, 75},
		{"50x101", 50, 101, 75},

		// Test 201-400 threshold
		{"300x300", 300, 300, 80},
		{"400x400", 400, 400, 80},
		{"201x100", 201, 100, 80},
		{"100x400", 100, 400, 80},

		// Test >400 threshold
		{"500x500", 500, 500, 85},
		{"800x600", 800, 600, 85},
		{"401x100", 401, 100, 85},
		{"100x401", 100, 401, 85},

		// Edge cases
		{"0x0", 0, 0, 70},
		{"1x1", 1, 1, 70},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJPEGQuality(tt.width, tt.height)
			if result != tt.expected {
				t.Errorf("getJPEGQuality(%d, %d) = %d, expected %d",
					tt.width, tt.height, result, tt.expected)
			}
		})
	}
}

func TestGetJPEGQuality_UsesMaxDimension(t *testing.T) {
	// Verify that the function uses the maximum of width/height
	// A 50x500 image should use quality for 500 (>400), not 50 (≤100)
	result := getJPEGQuality(50, 500)
	if result != 85 {
		t.Errorf("getJPEGQuality(50, 500) = %d, expected 85 (should use max dimension)", result)
	}

	result = getJPEGQuality(500, 50)
	if result != 85 {
		t.Errorf("getJPEGQuality(500, 50) = %d, expected 85 (should use max dimension)", result)
	}
}

func TestTruncateStderr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "short error",
			maxLen:   100,
			expected: "short error",
		},
		{
			name:     "exact length unchanged",
			input:    "exactly ten",
			maxLen:   11,
			expected: "exactly ten",
		},
		{
			name:     "long string truncated",
			input:    "this is a very long error message that should be truncated",
			maxLen:   20,
			expected: "this is a very long ... (truncated)",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "maxLen zero truncates everything",
			input:    "some text",
			maxLen:   0,
			expected: "... (truncated)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateStderr(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateStderr(%q, %d) = %q, expected %q",
					tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestParseFFmpegError(t *testing.T) {
	tests := []struct {
		name              string
		stderr            string
		wantNeedsTempFile bool
		wantCategory      string
	}{
		{
			name:              "moov atom not found",
			stderr:            "[mov,mp4,m4a,3gp,3g2,mj2 @ 0x7f8b8c004000] moov atom not found\npipe:: Invalid data found when processing input",
			wantNeedsTempFile: true,
			wantCategory:      "moov atom not found",
		},
		{
			name:              "invalid data found",
			stderr:            "Invalid data found when processing input",
			wantNeedsTempFile: true,
			wantCategory:      "invalid input data",
		},
		{
			name:              "codec parameters not found",
			stderr:            "Could not find codec parameters for stream 0 (Video: none)",
			wantNeedsTempFile: true,
			wantCategory:      "codec parameters not found",
		},
		{
			name:              "pipe EOF",
			stderr:            "pipe:: End of file",
			wantNeedsTempFile: true,
			wantCategory:      "pipe EOF",
		},
		{
			name:              "pipe invalid data",
			stderr:            "pipe:: Invalid data found",
			wantNeedsTempFile: true,
			wantCategory:      "pipe invalid data",
		},
		{
			name:              "normal codec error - no temp file needed",
			stderr:            "Decoder (codec h264) not found for input stream",
			wantNeedsTempFile: false,
			wantCategory:      "",
		},
		{
			name:              "empty stderr",
			stderr:            "",
			wantNeedsTempFile: false,
			wantCategory:      "",
		},
		{
			name:              "case insensitive matching",
			stderr:            "MOOV ATOM NOT FOUND",
			wantNeedsTempFile: true,
			wantCategory:      "moov atom not found",
		},
		{
			name:              "immediate exit requested",
			stderr:            "Immediate exit requested",
			wantNeedsTempFile: true,
			wantCategory:      "immediate exit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNeedsTempFile, gotCategory := parseFFmpegError(tt.stderr)
			if gotNeedsTempFile != tt.wantNeedsTempFile {
				t.Errorf("parseFFmpegError() needsTempFile = %v, want %v", gotNeedsTempFile, tt.wantNeedsTempFile)
			}
			if gotCategory != tt.wantCategory {
				t.Errorf("parseFFmpegError() category = %q, want %q", gotCategory, tt.wantCategory)
			}
		})
	}
}

func TestComputeActualTargetDims(t *testing.T) {
	tests := []struct {
		name              string
		resourceW, resourceH uint
		reqW, reqH        uint
		wantW, wantH      uint
	}{
		// Both axes zero, known aspect — full size scaled to fit MaxThumb (600).
		{"both zero, 1920x1080", 1920, 1080, 0, 0, 600, 338},
		{"both zero, 1000x1000", 1000, 1000, 0, 0, 600, 600},

		// Height only, known aspect — width derived from aspect ratio, no
		// implicit cap on the derived axis (matches imaging.Resize behavior).
		{"h-only, 1920x1080", 1920, 1080, 0, 400, 711, 400},
		{"h-only, 2000x1500 (the bug repro)", 2000, 1500, 0, 400, 533, 400},
		{"h-only portrait, 1000x2000", 1000, 2000, 0, 400, 200, 400},

		// Width only, known aspect — height derived.
		{"w-only, 1920x1080", 1920, 1080, 200, 0, 200, 113},

		// Both axes specified — passed through, capped, square shape preserved.
		{"forced square 64x64", 1920, 1080, 64, 64, 64, 64},
		{"forced square 96x96", 1920, 1080, 96, 96, 96, 96},
		{"forced 700x700, capped to MaxThumb", 1920, 1080, 700, 700, 600, 600},

		// Requested axis above cap is clamped first; derived axis follows.
		{"req w above cap derives capped", 1920, 1080, 700, 0, 600, 338},
		{"req h above cap derives capped", 1920, 1080, 0, 700, 1067, 600},

		// Resource dims unknown — pass-through (zeros allowed for resize lib to derive).
		{"unknown resource, h-only", 0, 0, 0, 400, 0, 400},
		{"unknown resource, w-only", 0, 0, 200, 0, 200, 0},
		{"unknown resource, both zero", 0, 0, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := models.Resource{Width: tt.resourceW, Height: tt.resourceH}
			gotW, gotH := computeActualTargetDims(res, tt.reqW, tt.reqH)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Errorf("computeActualTargetDims(W=%d,H=%d, req=%d,%d) = (%d, %d); want (%d, %d)",
					tt.resourceW, tt.resourceH, tt.reqW, tt.reqH, gotW, gotH, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestDecodeImageDimensions(t *testing.T) {
	// Build a tiny 17x29 JPEG in memory and assert the helper recovers those dims.
	src := image.NewRGBA(image.Rect(0, 0, 17, 29))
	for x := 0; x < 17; x++ {
		for y := 0; y < 29; y++ {
			src.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 5), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, src, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	w, h, err := decodeImageDimensions(buf.Bytes())
	if err != nil {
		t.Fatalf("decodeImageDimensions: %v", err)
	}
	if w != 17 || h != 29 {
		t.Errorf("decodeImageDimensions = (%d, %d); want (17, 29)", w, h)
	}

	// Invalid bytes should error, not panic.
	if _, _, err := decodeImageDimensions([]byte("not an image")); err == nil {
		t.Error("expected error for invalid bytes; got nil")
	}
}
