package main

import (
	"fmt"
	"strings"

	"bubbletea-poc/wavfile"
)

func renderBrailleWaveform(peaks []float64, width int) string {
	if len(peaks) == 0 {
		return ""
	}

	var b strings.Builder

	// Braille base character (U+2800)
	const brailleBase = 0x2800

	// Braille dot positions (bit flags):
	// 0 3    left column: dots 0,1,2,6
	// 1 4    right column: dots 3,4,5,7
	// 2 5
	// 6 7
	dotPattern := []int{0x01, 0x02, 0x04, 0x40, 0x08, 0x10, 0x20, 0x80}

	// Multiple rows of braille for more vertical depth
	brailleHeight := 4
	totalLevels := brailleHeight * 4 // 4 dots per column per character

	// Each braille char shows 2 columns of waveform
	peaksPerColumn := len(peaks) / (width * 2)
	if peaksPerColumn < 1 {
		peaksPerColumn = 1
	}

	// Create grid of braille characters
	grid := make([][]rune, brailleHeight)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = rune(brailleBase)
		}
	}

	for brailleCol := 0; brailleCol < width; brailleCol++ {
		// Process 2 columns (left and right dots)
		for subCol := 0; subCol < 2; subCol++ {
			peakCol := brailleCol*2 + subCol
			start := peakCol * peaksPerColumn
			end := start + peaksPerColumn
			if end > len(peaks) {
				end = len(peaks)
			}

			if start < len(peaks) {
				// Find max value in this range of peaks
				maxAbs := 0.0
				for i := start; i < end; i++ {
					if peaks[i] > maxAbs {
						maxAbs = peaks[i]
					}
				}

				// Map to total vertical levels
				level := int(maxAbs * float64(totalLevels-1))
				if level >= totalLevels {
					level = totalLevels - 1
				}

				// Fill dots from bottom up
				for l := 0; l <= level; l++ {
					row := brailleHeight - 1 - (l / 4)
					dotInChar := 3 - (l % 4)
					dotIndex := subCol*4 + dotInChar

					currentChar := int(grid[row][brailleCol] - brailleBase)
					currentChar |= dotPattern[dotIndex]
					grid[row][brailleCol] = rune(brailleBase + currentChar)
				}
			}
		}
	}

	// Build braille grid output
	for _, row := range grid {
		b.WriteString(string(row))
		b.WriteString("\n")
	}

	return b.String()
}

// RenderWaveformForFile renders a waveform in braille with metadata
func RenderWaveformForFile(metadata *wavfile.Metadata, width int, startFrame int, endFrame int, activeMarker string, markerStepSize int) string {
	if metadata == nil || len(metadata.WaveformData.Peaks) == 0 {
		return "Loading waveform... ↻"
	}

	var b strings.Builder

	// Info bar
	b.WriteString(fmt.Sprintf("Duration: %.2fs | Frames: %d | Sample Rate: %d Hz | Step: %d frames\n",
		metadata.Duration, metadata.NumFrames, metadata.SampleRate, markerStepSize))

	// Waveform
	b.WriteString(renderBrailleWaveform(metadata.WaveformData.Peaks, width))

	// Build marker line showing both start and end markers
	markerLine := make([]rune, width)
	for i := range markerLine {
		markerLine[i] = ' '
	}

	// Calculate positions for start and end markers
	startPos := int(float64(startFrame) / float64(metadata.NumFrames) * float64(width*2))
	startCharPos := startPos / 2

	endPos := int(float64(endFrame) / float64(metadata.NumFrames) * float64(width*2))
	endCharPos := endPos / 2

	// Place markers (active marker uses ▲, inactive uses ▽)
	if startCharPos >= 0 && startCharPos < width {
		if activeMarker == "start" {
			markerLine[startCharPos] = '▲'
		} else {
			markerLine[startCharPos] = '▽'
		}
	}

	if endCharPos >= 0 && endCharPos < width {
		if activeMarker == "end" {
			markerLine[endCharPos] = '▲'
		} else {
			markerLine[endCharPos] = '▽'
		}
	}

	b.WriteString(string(markerLine) + "\n")

	// Display frame numbers under their respective markers
	infoLine := make([]rune, width)
	for i := range infoLine {
		infoLine[i] = ' '
	}

	startInfo := fmt.Sprintf("Start: %d", startFrame)
	endInfo := fmt.Sprintf("End: %d", endFrame)

	// Calculate centered positions for each info display
	startInfoPos := startCharPos - len(startInfo)/2
	if startInfoPos < 0 {
		startInfoPos = 0
	}
	if startInfoPos+len(startInfo) > width {
		startInfoPos = width - len(startInfo)
		if startInfoPos < 0 {
			startInfoPos = 0
		}
	}

	endInfoPos := endCharPos - len(endInfo)/2
	if endInfoPos < 0 {
		endInfoPos = 0
	}
	if endInfoPos+len(endInfo) > width {
		endInfoPos = width - len(endInfo)
		if endInfoPos < 0 {
			endInfoPos = 0
		}
	}

	// Check if the info displays would overlap
	if startInfoPos+len(startInfo) <= endInfoPos || endInfoPos+len(endInfo) <= startInfoPos {
		// No overlap, write both
		for i, ch := range startInfo {
			if startInfoPos+i < width {
				infoLine[startInfoPos+i] = ch
			}
		}
		for i, ch := range endInfo {
			if endInfoPos+i < width {
				infoLine[endInfoPos+i] = ch
			}
		}
	} else {
		// Overlap detected, show combined info
		combinedInfo := fmt.Sprintf("Start: %d | End: %d", startFrame, endFrame)
		for i, ch := range combinedInfo {
			if i < width {
				infoLine[i] = ch
			}
		}
	}

	b.WriteString(string(infoLine) + "\n")

	return b.String()
}
