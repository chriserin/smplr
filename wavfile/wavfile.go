package wavfile

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PlaybackStartedMsg struct {
	Filename string
}

type PlaybackFinishedMsg struct {
	Filename string
}

// WaveformData contains pre-calculated waveform visualization data
type WaveformData struct {
	Peaks []float64 // Peak amplitude for each display segment
}

// Metadata contains information about a WAV file
type Metadata struct {
	SampleRate   uint32
	NumFrames    int
	Duration     float64
	WaveformData WaveformData
}

// WavFile represents a WAV file with its MIDI mapping and playback state
type WavFile struct {
	PlayingCount    int // Reference count of active playbacks
	Loading         bool
	MidiChannel     int
	MidiNote        int
	Pitch           int    // Pitch shift in semitones (-12 to 12)
	PitchedFileName string // Path to offline-rendered pitched file, empty if pitch is 0
	StartFrame      int
	EndFrame        int
	PlayerId        int
	Metadata        *Metadata
	Name            string
}

type wavHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}

// MetadataLoadedMsg is sent when a WAV file's metadata has been loaded
type MetadataLoadedMsg struct {
	Filename string
	Metadata *Metadata
	Err      error
}

// isPitchedFile checks if a filename matches the pattern for auto-generated pitched files
func isPitchedFile(filename string) bool {
	return strings.Contains(filename, "_pitch_")
}

// GeneratePitchedFilename creates a filename for a pitched version of the audio file
func GeneratePitchedFilename(originalFilename string, pitch int) string {
	if pitch == 0 {
		return ""
	}

	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)
	cents := pitch * 100

	var sign string
	if cents >= 0 {
		sign = "+"
	} else {
		sign = ""
	}

	return fmt.Sprintf("%s_pitch_%s%d%s", nameWithoutExt, sign, cents, ext)
}

// PitchedFileExists checks if a pitched file already exists on disk
func PitchedFileExists(filename string) bool {
	if filename == "" {
		return false
	}
	_, err := os.Stat(filename)
	return err == nil
}

// RemoveAllPitchedVersions removes all pitched versions of the given original file
func RemoveAllPitchedVersions(originalFilename string) error {
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)
	pattern := fmt.Sprintf("%s_pitch_*%s", nameWithoutExt, ext)

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find pitched files: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			return fmt.Errorf("failed to remove %s: %w", match, err)
		}
	}

	return nil
}

// LoadFiles loads all WAV files from the current directory
// and assigns incremental MIDI note numbers starting from 1.
// It returns WavFile structs without metadata immediately.
// Metadata is loaded concurrently in background goroutines.
// Excludes auto-generated pitched files (files with "_pitch_" in the name).
func LoadFiles(metadataChan chan<- MetadataLoadedMsg) []WavFile {
	entries, err := os.ReadDir(".")
	if err != nil {
		return []WavFile{}
	}

	// Collect WAV file names and create WavFile structs without metadata
	var wavFiles []WavFile
	note := 1
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".wav") {
			// Skip auto-generated pitched files
			if isPitchedFile(entry.Name()) {
				continue
			}

			wavFiles = append(wavFiles, WavFile{
				Name:        entry.Name(),
				MidiChannel: 1,
				MidiNote:    note,
				StartFrame:  0,
				EndFrame:    0,
				Metadata:    nil, // Will be loaded in background
				Loading:     true,
			})
			note++
		}
	}

	// Start background goroutines to load metadata for each file
	for _, file := range wavFiles {
		go func(filename string) {
			metadata, err := ReadMetadata(filename)
			metadataChan <- MetadataLoadedMsg{
				Filename: filename,
				Metadata: metadata,
				Err:      err,
			}
		}(file.Name)
	}

	return wavFiles
}

// FindMaxMidiNote returns the largest MIDI note value in a slice of WavFiles
func FindMaxMidiNote(files []WavFile) int {
	maxNote := 0
	for _, file := range files {
		if file.MidiNote > maxNote {
			maxNote = file.MidiNote
		}
	}
	return maxNote
}

// MoveMarker moves the specified marker (start or end) by the given direction and step size
func (w *WavFile) MoveMarker(activeMarker string, direction int, stepSize int) {
	if w.Metadata == nil {
		return
	}

	metadata := w.Metadata

	// Move the active marker by stepSize frames
	switch activeMarker {
	case "start":
		w.StartFrame += direction * stepSize

		// Clamp to valid range
		if w.StartFrame < 0 {
			w.StartFrame = 0
		}
		if w.StartFrame >= metadata.NumFrames {
			w.StartFrame = metadata.NumFrames - 1
		}
		// Don't let start marker move past end marker
		if w.StartFrame > w.EndFrame {
			w.StartFrame = w.EndFrame
		}
	case "end":
		w.EndFrame += direction * stepSize

		// Clamp to valid range
		if w.EndFrame < 0 {
			w.EndFrame = 0
		}
		if w.EndFrame >= metadata.NumFrames {
			w.EndFrame = metadata.NumFrames - 1
		}
		// Don't let end marker move past start marker
		if w.EndFrame < w.StartFrame {
			w.EndFrame = w.StartFrame
		}
	}
}

// ReadMetadata reads a WAV file and returns its metadata
func ReadMetadata(filename string) (*Metadata, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read RIFF header
	var chunkID [4]byte
	var chunkSize uint32
	var format [4]byte

	binary.Read(file, binary.LittleEndian, &chunkID)
	binary.Read(file, binary.LittleEndian, &chunkSize)
	binary.Read(file, binary.LittleEndian, &format)

	if string(chunkID[:]) != "RIFF" || string(format[:]) != "WAVE" {
		return nil, fmt.Errorf("not a valid WAV file")
	}

	var header wavHeader
	var dataSize uint32
	foundFmt := false
	foundData := false

	// Read all chunks
	for !foundData {
		var subchunkID [4]byte
		var subchunkSize uint32

		if err := binary.Read(file, binary.LittleEndian, &subchunkID); err != nil {
			return nil, fmt.Errorf("error reading chunk ID: %w", err)
		}
		if err := binary.Read(file, binary.LittleEndian, &subchunkSize); err != nil {
			return nil, fmt.Errorf("error reading chunk size: %w", err)
		}

		chunkName := string(subchunkID[:])

		switch chunkName {
		case "fmt ":
			binary.Read(file, binary.LittleEndian, &header.AudioFormat)
			binary.Read(file, binary.LittleEndian, &header.NumChannels)
			binary.Read(file, binary.LittleEndian, &header.SampleRate)
			binary.Read(file, binary.LittleEndian, &header.ByteRate)
			binary.Read(file, binary.LittleEndian, &header.BlockAlign)
			binary.Read(file, binary.LittleEndian, &header.BitsPerSample)
			// Skip any extra bytes in fmt chunk
			if subchunkSize > 16 {
				file.Seek(int64(subchunkSize-16), io.SeekCurrent)
			}
			foundFmt = true
		case "data":
			dataSize = subchunkSize
			foundData = true
		default:
			// Skip unknown chunk
			file.Seek(int64(subchunkSize), io.SeekCurrent)
		}
	}

	if !foundFmt {
		return nil, fmt.Errorf("fmt chunk not found")
	}

	// Read samples
	numSamples := int(dataSize) / int(header.BlockAlign)
	samples := make([]float64, numSamples)

	switch header.BitsPerSample {
	case 16:
		for i := range numSamples {
			var sample int16
			if err := binary.Read(file, binary.LittleEndian, &sample); err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			samples[i] = float64(sample) / 32768.0

			// Skip other channels if stereo
			if header.NumChannels > 1 {
				file.Seek(int64((header.NumChannels-1)*2), io.SeekCurrent)
			}
		}
	case 8:
		for i := range numSamples {
			var sample uint8
			if err := binary.Read(file, binary.LittleEndian, &sample); err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			samples[i] = (float64(sample) - 128.0) / 128.0

			if header.NumChannels > 1 {
				file.Seek(int64(header.NumChannels-1), io.SeekCurrent)
			}
		}
	case 24:
		for i := range numSamples {
			var bytes [3]byte
			if err := binary.Read(file, binary.LittleEndian, &bytes); err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			// Convert 24-bit little-endian to int32
			sample := int32(bytes[0]) | int32(bytes[1])<<8 | int32(bytes[2])<<16
			// Sign extend from 24-bit to 32-bit
			if sample&0x800000 != 0 {
				sample |= ^0xFFFFFF
			}
			samples[i] = float64(sample) / 8388608.0

			// Skip other channels if stereo
			if header.NumChannels > 1 {
				file.Seek(int64((header.NumChannels-1)*3), io.SeekCurrent)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported bit depth: %d", header.BitsPerSample)
	}

	duration := float64(len(samples)) / float64(header.SampleRate)

	// Pre-calculate waveform data for visualization
	// Use a reasonable number of segments for display (e.g., 2000 segments = 1000 char width * 2)
	maxSegments := 2000
	waveformData := calculateWaveformData(samples, maxSegments)

	return &Metadata{
		SampleRate:   header.SampleRate,
		NumFrames:    len(samples),
		Duration:     duration,
		WaveformData: waveformData,
	}, nil
}

// calculateWaveformData pre-calculates peak values for waveform display
func calculateWaveformData(samples []float64, numSegments int) WaveformData {
	if len(samples) == 0 {
		return WaveformData{Peaks: []float64{}}
	}

	// Don't create more segments than samples
	if numSegments > len(samples) {
		numSegments = len(samples)
	}

	peaks := make([]float64, numSegments)
	samplesPerSegment := len(samples) / numSegments
	if samplesPerSegment < 1 {
		samplesPerSegment = 1
	}

	for i := 0; i < numSegments; i++ {
		start := i * samplesPerSegment
		end := start + samplesPerSegment
		if end > len(samples) {
			end = len(samples)
		}

		// Find max absolute value in this segment
		maxAbs := 0.0
		for j := start; j < end; j++ {
			abs := samples[j]
			if abs < 0 {
				abs = -abs
			}
			if abs > maxAbs {
				maxAbs = abs
			}
		}
		peaks[i] = maxAbs
	}

	return WaveformData{Peaks: peaks}
}
