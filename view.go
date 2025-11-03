package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	var b strings.Builder
	var listContent strings.Builder

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("170")).
		Bold(true)

	editingStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230"))

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33"))

	// Header row (outside viewport, always visible)
	header := fmt.Sprintf("%-40s  %-7s  %-5s  %-5s", "Name", "Channel", "Note", "Pitch")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render(strings.Repeat("-", 64)))
	b.WriteString("\n")

	if len(*m.files) == 0 {
		listContent.WriteString("No .wav files found in current directory.\n")
	} else {
		// File rows (inside viewport)
		for i, file := range *m.files {
			cursor := "  "
			if m.cursor == i && !m.recording {
				cursor = "> "
			}

			channelStr := fmt.Sprintf("%d", file.MidiChannel)
			noteStr := fmt.Sprintf("%d", file.MidiNote)
			pitchStr := fmt.Sprintf("%d", file.Pitch)

			// Highlight field being edited
			if m.cursor == i && m.editing && !m.recording {
				switch m.editField {
				case "channel":
					channelStr = editingStyle.Render(fmt.Sprintf("%s_", m.editValue))
				case "note":
					noteStr = editingStyle.Render(fmt.Sprintf("%s_", m.editValue))
				case "pitch":
					pitchStr = editingStyle.Render(fmt.Sprintf("%s_", m.editValue))
				}
			}

			name := file.Name
			playingIcon := "  "
			if file.PlayingCount > 0 {
				greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
				playingIcon = greenStyle.Render("● ")
			}
			loadingIcon := ""
			if file.Loading {
				loadingIcon = "↻ "
			}
			nameWithIcon := loadingIcon + name
			if len(nameWithIcon) > 38 {
				nameWithIcon = nameWithIcon[:35] + "..."
			}

			line := fmt.Sprintf("%s%-40s  %-7s  %-5s  %-5s", cursor, nameWithIcon, channelStr, noteStr, pitchStr)

			if m.cursor == i && !m.editing && !m.recording {
				listContent.WriteString(fmt.Sprintf("%s%s\n", playingIcon, selectedStyle.Render(line)))
			} else {
				listContent.WriteString(fmt.Sprintf("%s%s\n", playingIcon, line))
			}
		}
	}

	// Update viewport content
	m.viewport.SetContent(listContent.String())

	// Build final view (viewport after header)
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	if m.recording {
		recordingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		b.WriteString(recordingStyle.Render("● RECORDING") + "\n")
	}

	// Display error message if present
	if m.currentError != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		b.WriteString(errorStyle.Render("ERROR: "+m.currentError) + "\n")
	}

	// Display waveform for the selected file (not while recording)
	if !m.recording && len(*m.files) > 0 && m.cursor >= 0 && m.cursor < len(*m.files) {
		b.WriteString("\n")
		waveform := RenderWaveformForFile(
			(*m.files)[m.cursor].Metadata,
			m.windowWidth,
			(*m.files)[m.cursor].StartFrame,
			(*m.files)[m.cursor].EndFrame,
			m.activeMarker,
			m.markerStepSize,
		)
		b.WriteString(waveform)
	}

	return b.String()
}
