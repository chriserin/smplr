package main

import (
	"fmt"
	"time"

	"smplr/audio"
	"smplr/mappings"
	"smplr/wavfile"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	files             *[]wavfile.WavFile
	cursor            int
	editing           bool
	editField         string // "channel" or "note"
	editValue         string
	recording         bool
	recordingFilename string
	audio             audio.Audio
	viewport          viewport.Model
	ready             bool
	windowWidth       int
	markerStepSize    int    // number of frames to move marker with h/l
	activeMarker      string // "start" or "end"
	currentError      string // error message to display
}

func initialModel(files *[]wavfile.WavFile, audio audio.Audio) model {
	vp := viewport.New(80, 10)
	vp.YPosition = 0

	return model{
		files:             files,
		cursor:            0,
		editing:           false,
		editField:         "",
		editValue:         "",
		recording:         false,
		recordingFilename: "",
		audio:             audio,
		viewport:          vp,
		ready:             false,
		windowWidth:       80,
		markerStepSize:    1,
		activeMarker:      "start",
	}
}

// handlePitchChange handles offline rendering when pitch changes
func (m *model) handlePitchChange(fileIndex int, newPitch int) error {
	file := &(*m.files)[fileIndex]

	// Generate pitched filename
	pitchedFilename := wavfile.GeneratePitchedFilename(file.Name, newPitch)

	// If pitch is 0, use original file
	if newPitch == 0 {
		file.PitchedFileName = ""

		// Recreate player with original file
		if file.PlayerId != 0 {
			m.audio.DestroyPlayer(file.PlayerId)
		}
		playerID, err := m.audio.CreatePlayer(file.Name)
		if err != nil {
			return fmt.Errorf("failed to recreate player: %w", err)
		}
		file.PlayerId = playerID

		return nil
	}

	// Check if pitched file already exists
	if !wavfile.PitchedFileExists(pitchedFilename) {
		// Render pitched file
		cents := float32(newPitch * 100)

		err := m.audio.RenderPitchedFile(file.Name, pitchedFilename, cents)

		if err != nil {
			return fmt.Errorf("failed to render pitched file: %w", err)
		}
	}

	// Update PitchedFileName
	file.PitchedFileName = pitchedFilename

	// Recreate player with pitched file
	if file.PlayerId != 0 {
		m.audio.DestroyPlayer(file.PlayerId)
	}

	playerID, err := m.audio.CreatePlayer(pitchedFilename)
	if err != nil {
		return fmt.Errorf("failed to create player for pitched file: %w", err)
	}
	file.PlayerId = playerID

	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wavfile.PlaybackStartedMsg:
		for i := range *m.files {
			if (*m.files)[i].Name == msg.Filename {
				(*m.files)[i].PlayingCount++
				break
			}
		}
		return m, nil
	case wavfile.PlaybackFinishedMsg:
		for i := range *m.files {
			if (*m.files)[i].Name == msg.Filename {
				if (*m.files)[i].PlayingCount > 0 {
					(*m.files)[i].PlayingCount--
				}
				break
			}
		}
		return m, nil
	case wavfile.MetadataLoadedMsg:
		// Find the WavFile with matching name and attach metadata
		for i := range *m.files {
			if (*m.files)[i].Name == msg.Filename {
				if msg.Err == nil {
					(*m.files)[i].Metadata = msg.Metadata
					// Set EndFrame to the end of the file
					if msg.Metadata != nil {
						(*m.files)[i].EndFrame = msg.Metadata.NumFrames - 1
					}
				}
				// Create player and load buffer for low-latency playback
				playerId, err := m.audio.CreatePlayer((*m.files)[i].Name)
				if err != nil {
					panic(err)
				}
				(*m.files)[i].PlayerId = playerId
				(*m.files)[i].Loading = false
				// Update marker step size if this is the currently selected file
				if i == m.cursor {
					m.updateMarkerStepSize()
				}
				break
			}
		}

		// Check if all files have finished loading
		allLoaded := true
		for i := range *m.files {
			if (*m.files)[i].Loading {
				allLoaded = false
				break
			}
		}

		// If all files are loaded, start the audio engine
		if allLoaded {
			if err := m.audio.Start(); err != nil {
				panic(err)
			}
		}

		return m, nil

	case tea.WindowSizeMsg:
		headerHeight := 2    // header line + separator
		footerHeight := 1    // blank line after viewport
		recordingHeight := 1 // recording status (if shown)
		waveformHeight := 8  // blank line + info bar + 4 lines of braille + marker line + frame number
		reservedHeight := headerHeight + footerHeight + recordingHeight + waveformHeight

		viewportHeight := msg.Height - reservedHeight
		if viewportHeight < 3 {
			viewportHeight = 3 // minimum height
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width, viewportHeight)
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = viewportHeight
		}
		m.windowWidth = msg.Width
		// Update marker step size when window width changes
		m.updateMarkerStepSize()

	case tea.KeyMsg:
		mapping := mappings.ProcessKey(msg, m.editing)
		if m.editing {
			return m.handleEditingInput(mapping)
		}
		return m.handleNavigationInput(mapping)
	}
	return m, nil
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m *model) scrollToSelection() {
	if m.cursor < 0 || len((*m.files)) == 0 {
		return
	}

	// Calculate the line number in the viewport content
	// Since header is outside viewport, cursor line is just the cursor index
	cursorLine := m.cursor

	// Ensure the cursor is visible in the viewport
	if cursorLine < m.viewport.YOffset {
		// Cursor is above visible area, scroll up
		m.viewport.YOffset = cursorLine
	} else if cursorLine >= m.viewport.YOffset+m.viewport.Height {
		// Cursor is below visible area, scroll down
		m.viewport.YOffset = cursorLine - m.viewport.Height + 1
	}

	// Ensure YOffset doesn't go negative
	if m.viewport.YOffset < 0 {
		m.viewport.YOffset = 0
	}

	// Update marker step size to move by one character
	m.updateMarkerStepSize()
}

func (m *model) updateMarkerStepSize() {
	if m.cursor < 0 || m.cursor >= len((*m.files)) {
		return
	}

	if (*m.files)[m.cursor].Metadata == nil {
		return
	}

	metadata := (*m.files)[m.cursor].Metadata

	// Calculate frames per character: each character position represents NumFrames/width frames
	// This makes h/l move the marker by one character width
	framesPerChar := metadata.NumFrames / m.windowWidth
	if framesPerChar < 1 {
		framesPerChar = 1
	}

	m.markerStepSize = framesPerChar
}

func (m *model) moveMarker(direction int) {
	if m.cursor < 0 || m.cursor >= len((*m.files)) {
		return
	}

	(*m.files)[m.cursor].MoveMarker(m.activeMarker, direction, m.markerStepSize)
}

func (m model) handleEditingInput(mapping mappings.Mapping) (tea.Model, tea.Cmd) {
	switch mapping.Command {
	case mappings.Enter:
		// Save the edited value
		if m.editValue != "" {
			var value int
			fmt.Sscanf(m.editValue, "%d", &value)

			if m.editField == "channel" && value >= 1 && value <= 16 {
				(*m.files)[m.cursor].MidiChannel = value
			} else if m.editField == "note" && value >= 0 && value <= 127 {
				(*m.files)[m.cursor].MidiNote = value
			} else if m.editField == "pitch" && value >= -12 && value <= 12 {
				(*m.files)[m.cursor].Pitch = value

				// Handle offline rendering for pitch change
				err := m.handlePitchChange(m.cursor, value)
				if err != nil {
					fmt.Printf("Error handling pitch change: %v\n", err)
				}
			}
		}
		m.editing = false
		m.editValue = ""
		m.editField = ""

	case mappings.Escape:
		// Cancel editing
		m.editing = false
		m.editValue = ""
		m.editField = ""

	case mappings.Backspace:
		if len(m.editValue) > 0 {
			m.editValue = m.editValue[:len(m.editValue)-1]
		}

	case mappings.NumberInput:
		m.editValue += mapping.LastValue
	}
	return m, nil
}

func (m model) handleNavigationInput(mapping mappings.Mapping) (tea.Model, tea.Cmd) {
	// Clear any error on key press
	m.currentError = ""

	switch mapping.Command {
	case mappings.Quit:
		return m, tea.Quit

	case mappings.CursorUp:
		if !m.recording && m.cursor > 0 {
			m.cursor--
			m.scrollToSelection()
		}

	case mappings.CursorDown:
		if !m.recording && m.cursor < len((*m.files))-1 {
			m.cursor++
			m.scrollToSelection()
		}

	case mappings.EditChannel:
		// Edit channel
		if len((*m.files)) > 0 {
			m.editing = true
			m.editField = "channel"
			m.editValue = ""
		}

	case mappings.EditNote:
		// Edit note
		if len((*m.files)) > 0 {
			m.editing = true
			m.editField = "note"
			m.editValue = ""
		}

	case mappings.EditPitch:
		// Edit pitch
		if len((*m.files)) > 0 {
			m.editing = true
			m.editField = "pitch"
			m.editValue = ""
		}

	case mappings.Recording:
		if !m.recording {
			// Start recording with timestamp-based filename
			m.recording = true
			m.cursor = -1 // Deselect all files while recording
			m.recordingFilename = fmt.Sprintf("recording_%s.wav", time.Now().Format("20060102_150405"))
			m.audio.Record(m.recordingFilename)
		} else {
			// Stop recording and add the new file to the list
			m.recording = false
			m.audio.StopRecording()
			if m.recordingFilename != "" {
				// Find the largest midi note and add 1
				maxNote := wavfile.FindMaxMidiNote((*m.files))
				// Load metadata for the new recording
				metadata, err := wavfile.ReadMetadata(m.recordingFilename)
				if err != nil {
					metadata = nil
				}
				endFrame := 0
				if metadata != nil {
					endFrame = metadata.NumFrames - 1
				}
				*m.files = append(*m.files, wavfile.WavFile{
					Name:        m.recordingFilename,
					MidiChannel: 1,
					MidiNote:    maxNote + 1,
					StartFrame:  0,
					EndFrame:    endFrame,
					Metadata:    metadata,
					Loading:     false,
				})
				// Select the newly added file
				m.cursor = len(*m.files) - 1
				m.scrollToSelection() // This will call updateMarkerStepSize()
				m.recordingFilename = ""
			}
		}

	case mappings.MarkerLeft:
		if !m.recording && len(*m.files) > 0 && m.cursor >= 0 && m.cursor < len(*m.files) {
			m.moveMarker(-1)
		}

	case mappings.MarkerRight:
		if !m.recording && len(*m.files) > 0 && m.cursor >= 0 && m.cursor < len(*m.files) {
			m.moveMarker(1)
		}

	case mappings.MarkerStepIncrease:
		// Double the step size
		m.markerStepSize *= 2
		if m.markerStepSize > 1000000 {
			m.markerStepSize = 1000000 // Cap at 1 million frames
		}

	case mappings.MarkerStepDecrease:
		// Halve the step size
		m.markerStepSize /= 2
		if m.markerStepSize < 1 {
			m.markerStepSize = 1 // Minimum 1 frame
		}

	case mappings.SelectStartMarker:
		m.activeMarker = "start"

	case mappings.SelectEndMarker:
		m.activeMarker = "end"

	case mappings.PlayFile:
		if !m.recording && len(*m.files) > 0 && m.cursor >= 0 && m.cursor < len(*m.files) {
			// Use pitched file if it exists, otherwise use original
			filename := (*m.files)[m.cursor].Name
			if (*m.files)[m.cursor].PitchedFileName != "" {
				filename = (*m.files)[m.cursor].PitchedFileName
			}
			// No real-time pitch shifting - files are pre-rendered
			err := m.audio.PlayFile((*m.files)[m.cursor].PlayerId, filename, 0)
			if err != nil {
				panic("Error playing file from update: " + err.Error())
			}
			(*m.files)[m.cursor].PlayingCount++
		}

	case mappings.PlayRegion:
		if !m.recording && len(*m.files) > 0 && m.cursor >= 0 && m.cursor < len(*m.files) {
			// Use pitched file if it exists, otherwise use original
			filename := (*m.files)[m.cursor].Name
			if (*m.files)[m.cursor].PitchedFileName != "" {
				filename = (*m.files)[m.cursor].PitchedFileName
			}
			// No real-time pitch shifting - files are pre-rendered
			err := m.audio.PlayRegion(
				(*m.files)[m.cursor].PlayerId,
				filename,
				(*m.files)[m.cursor].StartFrame,
				(*m.files)[m.cursor].EndFrame,
				0,
			)
			if err != nil {
				panic("Error playing region from update: " + err.Error())
			}
			(*m.files)[m.cursor].PlayingCount++
		}

	case mappings.TrimFile:
		if !m.recording && len(*m.files) > 0 && m.cursor >= 0 && m.cursor < len(*m.files) {
			if (*m.files)[m.cursor].Pitch != 0 {
				m.currentError = "Cannot trim file with non-zero pitch. Reset pitch to 0 first."
				return m, nil
			}
			err := m.audio.TrimFile(
				(*m.files)[m.cursor].Name,
				(*m.files)[m.cursor].StartFrame,
				(*m.files)[m.cursor].EndFrame,
			)
			if err == nil {
				// Remove all pitched versions of this file
				if err := wavfile.RemoveAllPitchedVersions((*m.files)[m.cursor].Name); err != nil {
					m.currentError = fmt.Sprintf("Warning: failed to remove pitched versions: %v", err)
				}

				// Reload metadata after trimming
				metadata, err := wavfile.ReadMetadata((*m.files)[m.cursor].Name)
				if err == nil {
					(*m.files)[m.cursor].Metadata = metadata
					// Reset markers to the start and end of the new file
					(*m.files)[m.cursor].StartFrame = 0
					(*m.files)[m.cursor].EndFrame = metadata.NumFrames - 1
					// Update marker step size for the new file length
					m.updateMarkerStepSize()
				}
			}
		}
	}
	return m, nil
}
