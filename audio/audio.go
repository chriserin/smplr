package audio

/*
#cgo darwin,arm64 LDFLAGS: ${SRCDIR}/AudioBridge.o /opt/homebrew/opt/rubberband/lib/librubberband.a /opt/homebrew/opt/libsamplerate/lib/libsamplerate.a -framework Accelerate
#cgo darwin,amd64 LDFLAGS: ${SRCDIR}/AudioBridge.o /usr/local/opt/rubberband/lib/librubberband.a /usr/local/opt/libsamplerate/lib/libsamplerate.a -framework Accelerate
#include <stdlib.h>

// Forward declare the Go callbacks
extern void goPlaybackFinished(int playerID);
extern void goDecibelLevel(float db);

// C wrapper function that will be passed to Swift
static void cPlaybackFinishedCallback(int playerID) {
    goPlaybackFinished(playerID);
}

// C wrapper function for decibel level callback
static void cDecibelLevelCallback(float db) {
    goDecibelLevel(db);
}

// Helper function to get the function pointer
static void* getCPlaybackFinishedCallback() {
    return (void*)cPlaybackFinishedCallback;
}

// Helper function to get the decibel callback function pointer
static void* getCDecibelLevelCallback() {
    return (void*)cDecibelLevelCallback;
}

// Declare Swift functions
extern int SwiftAudio_init(void);
extern int SwiftAudio_start(void);
extern int SwiftAudio_createPlayer(const char* filename);
extern int SwiftAudio_destroyPlayer(int playerID);
extern int SwiftAudio_stopPlayer(int playerID);
extern int SwiftAudio_record(const char* filename);
extern int SwiftAudio_stopRecording(void);
extern int SwiftAudio_playFile(int playerID, const char* filename, float cents);
extern int SwiftAudio_playRegion(int playerID, const char* filename, int startFrame, int endFrame, float cents);
extern int SwiftAudio_trimFile(const char* filename, int startFrame, int endFrame);
extern int SwiftAudio_renderPitchedFile(const char* sourceFilename, const char* targetFilename, float cents);
extern void SwiftAudio_setCompletionCallback(void (*callback)(int));
extern void SwiftAudio_setDecibelCallback(void (*callback)(float));
extern char* SwiftAudio_getAudioDevices(void);
*/
import "C"
import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"
)

// Global channels for notifications
var playbackCompletionChan chan int
var decibelLevelChan chan float32

//export goPlaybackFinished
func goPlaybackFinished(playerID C.int) {
	if playbackCompletionChan != nil {
		playbackCompletionChan <- int(playerID)
	}
}

//export goDecibelLevel
func goDecibelLevel(db C.float) {
	if decibelLevelChan != nil {
		decibelLevelChan <- float32(db)
	}
}

// SetPlaybackCompletionChannel sets the channel for playback completion notifications
func SetPlaybackCompletionChannel(ch chan int) {
	playbackCompletionChan = ch
	// Register the callback with Swift using the C wrapper
	callbackPtr := C.getCPlaybackFinishedCallback()
	C.SwiftAudio_setCompletionCallback((*[0]byte)(callbackPtr))
}

// SetDecibelLevelChannel sets the channel for decibel level notifications
func SetDecibelLevelChannel(ch chan float32) {
	decibelLevelChan = ch
	// Register the callback with Swift using the C wrapper
	callbackPtr := C.getCDecibelLevelCallback()
	C.SwiftAudio_setDecibelCallback((*[0]byte)(callbackPtr))
}

// AudioDevice represents an audio output device
type AudioDevice struct {
	ID   string
	Name string
}

// Audio defines the interface for audio recording and playback operations
// This will eventually be implemented as a bridge to Swift code using MacOS AV API
type Audio interface {
	Init() error
	Start() error
	CreatePlayer(filename string) (int, error)
	DestroyPlayer(playerID int) error
	StopPlayer(playerID int) error
	Record(filename string) error
	StopRecording() error
	PlayFile(playerID int, filename string, cents float32) error
	PlayRegion(playerID int, filename string, startFrame int, endFrame int, cents float32) error
	TrimFile(filename string, startFrame int, endFrame int) error
	RenderPitchedFile(sourceFilename string, targetFilename string, cents float32) error
	GetAudioDevices() ([]AudioDevice, error)
}

// StubAudio is a stub implementation of the Audio interface
type StubAudio struct {
	isRecording       bool
	recordingFilename string
}

// NewStubAudio creates a new stub audio implementation
func NewStubAudio() *StubAudio {
	return &StubAudio{
		isRecording:       false,
		recordingFilename: "",
	}
}

// Init initializes the stub audio system
func (a *StubAudio) Init() error {
	// Stub implementation - nothing to initialize
	return nil
}

// Start starts the stub audio system
func (a *StubAudio) Start() error {
	// Stub implementation - nothing to start
	return nil
}

// CreatePlayer creates a new audio player and returns its ID
func (a *StubAudio) CreatePlayer(filename string) (int, error) {
	// Stub implementation - return a dummy ID
	return 1, nil
}

// DestroyPlayer destroys the audio player with the given ID
func (a *StubAudio) DestroyPlayer(playerID int) error {
	// Stub implementation - nothing to destroy
	return nil
}

// StopPlayer stops playback for the given player ID
func (a *StubAudio) StopPlayer(playerID int) error {
	// Stub implementation - nothing to stop
	return nil
}

// Record starts recording audio to the specified file
// filename should have a .wav extension and will be saved in the current directory
func (a *StubAudio) Record(filename string) error {
	if a.isRecording {
		return nil // Already recording
	}
	a.isRecording = true
	a.recordingFilename = filename
	// Stub implementation - will be replaced with Swift bridge
	return nil
}

// StopRecording stops the current recording
// In stub mode, copies melissa.wav to the recording filename
func (a *StubAudio) StopRecording() error {
	if !a.isRecording {
		return nil // Not currently recording
	}

	// Copy melissa.wav to the recording filename
	if a.recordingFilename != "" {
		srcFile, err := os.Open("melissa.wav")
		if err != nil {
			a.isRecording = false
			a.recordingFilename = ""
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(a.recordingFilename)
		if err != nil {
			a.isRecording = false
			a.recordingFilename = ""
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		if err != nil {
			a.isRecording = false
			a.recordingFilename = ""
			return err
		}
	}

	a.isRecording = false
	a.recordingFilename = ""
	return nil
}

// PlayFile plays the entire audio file
// Stub implementation - will be replaced with Swift bridge
func (a *StubAudio) PlayFile(playerID int, filename string, cents float32) error {
	// Stub implementation - just returns nil
	// Real implementation would play the file
	return nil
}

// PlayRegion plays a region of the audio file from startFrame to endFrame
// Stub implementation - will be replaced with Swift bridge
func (a *StubAudio) PlayRegion(playerID int, filename string, startFrame int, endFrame int, cents float32) error {
	fmt.Fprintln(os.Stderr, "playing", filename)
	// Stub implementation - just returns nil
	// Real implementation would play from startFrame to endFrame
	return nil
}

// RenderPitchedFile creates a new audio file with pitch shifting applied offline
func (a *StubAudio) RenderPitchedFile(sourceFilename string, targetFilename string, cents float32) error {
	// Stub implementation - just copy the source file to target
	srcFile, err := os.Open(sourceFilename)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(targetFilename)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// GetAudioDevices returns a list of available audio output devices
func (a *StubAudio) GetAudioDevices() ([]AudioDevice, error) {
	// Stub implementation - return fake devices
	return []AudioDevice{
		{ID: "stub-device-1", Name: "Stub Audio Device 1"},
		{ID: "stub-device-2", Name: "Stub Audio Device 2"},
	}, nil
}

// TrimFile rewrites the audio file to only contain frames from startFrame to endFrame
func (a *StubAudio) TrimFile(filename string, startFrame int, endFrame int) error {
	// Open the original file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
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
		return fmt.Errorf("not a valid WAV file")
	}

	// Read fmt chunk
	var audioFormat uint16
	var numChannels uint16
	var sampleRate uint32
	var byteRate uint32
	var blockAlign uint16
	var bitsPerSample uint16

	foundFmt := false
	foundData := false

	for !foundData {
		var subchunkID [4]byte
		var subchunkSize uint32

		if err := binary.Read(file, binary.LittleEndian, &subchunkID); err != nil {
			return fmt.Errorf("error reading chunk ID: %w", err)
		}
		if err := binary.Read(file, binary.LittleEndian, &subchunkSize); err != nil {
			return fmt.Errorf("error reading chunk size: %w", err)
		}

		chunkName := string(subchunkID[:])

		switch chunkName {
		case "fmt ":
			binary.Read(file, binary.LittleEndian, &audioFormat)
			binary.Read(file, binary.LittleEndian, &numChannels)
			binary.Read(file, binary.LittleEndian, &sampleRate)
			binary.Read(file, binary.LittleEndian, &byteRate)
			binary.Read(file, binary.LittleEndian, &blockAlign)
			binary.Read(file, binary.LittleEndian, &bitsPerSample)
			if subchunkSize > 16 {
				file.Seek(int64(subchunkSize-16), io.SeekCurrent)
			}
			foundFmt = true
		case "data":
			foundData = true
		default:
			file.Seek(int64(subchunkSize), io.SeekCurrent)
		}
	}

	if !foundFmt {
		return fmt.Errorf("fmt chunk not found")
	}

	// Read all samples
	numSamplesToRead := (endFrame - startFrame + 1)

	// Seek to the start frame
	file.Seek(int64(startFrame*int(blockAlign)), io.SeekCurrent)

	// Read the trimmed samples
	sampleData := make([]byte, numSamplesToRead*int(blockAlign))
	_, err = io.ReadFull(file, sampleData)
	if err != nil {
		return fmt.Errorf("error reading samples: %w", err)
	}

	file.Close()

	// Calculate new sizes
	newDataSize := uint32(len(sampleData))
	newChunkSize := 36 + newDataSize

	// Write to temporary file
	tempFilename := filename + ".tmp"
	outFile, err := os.Create(tempFilename)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	// Write RIFF header
	outFile.Write([]byte("RIFF"))
	binary.Write(outFile, binary.LittleEndian, newChunkSize)
	outFile.Write([]byte("WAVE"))

	// Write fmt chunk
	outFile.Write([]byte("fmt "))
	binary.Write(outFile, binary.LittleEndian, uint32(16))
	binary.Write(outFile, binary.LittleEndian, audioFormat)
	binary.Write(outFile, binary.LittleEndian, numChannels)
	binary.Write(outFile, binary.LittleEndian, sampleRate)
	binary.Write(outFile, binary.LittleEndian, byteRate)
	binary.Write(outFile, binary.LittleEndian, blockAlign)
	binary.Write(outFile, binary.LittleEndian, bitsPerSample)

	// Write data chunk
	outFile.Write([]byte("data"))
	binary.Write(outFile, binary.LittleEndian, newDataSize)
	outFile.Write(sampleData)

	outFile.Close()

	// Replace original file with temp file
	err = os.Rename(tempFilename, filename)
	if err != nil {
		os.Remove(tempFilename)
		return fmt.Errorf("failed to replace original file: %w", err)
	}

	return nil
}

// SwiftAudio is a Swift bridge implementation of the Audio interface
type SwiftAudio struct{ Started bool }

// NewSwiftAudio creates a new Swift audio implementation
func NewSwiftAudio() *SwiftAudio {
	return &SwiftAudio{}
}

// Init initializes the Swift audio system
func (a *SwiftAudio) Init() error {
	result := C.SwiftAudio_init()
	if result != 0 {
		return fmt.Errorf("failed to initialize audio system")
	}
	return nil
}

// Start starts the Swift audio engine
func (a *SwiftAudio) Start() error {
	if a.Started {
		return nil // Already started
	}
	result := C.SwiftAudio_start()
	if result != 0 {
		return fmt.Errorf("failed to start audio engine")
	}
	a.Started = true
	return nil
}

// CreatePlayer creates a new audio player and returns its ID
func (a *SwiftAudio) CreatePlayer(filename string) (int, error) {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	result := C.SwiftAudio_createPlayer(cFilename)
	if result < 0 {
		return 0, fmt.Errorf("failed to create audio player")
	}
	return int(result), nil
}

// DestroyPlayer destroys the audio player with the given ID
func (a *SwiftAudio) DestroyPlayer(playerID int) error {
	result := C.SwiftAudio_destroyPlayer(C.int(playerID))
	if result != 0 {
		return fmt.Errorf("failed to destroy audio player")
	}
	return nil
}

// StopPlayer stops playback for the given player ID
func (a *SwiftAudio) StopPlayer(playerID int) error {
	result := C.SwiftAudio_stopPlayer(C.int(playerID))
	if result != 0 {
		return fmt.Errorf("failed to stop audio player")
	}
	return nil
}

// Record starts recording audio to the specified file
func (a *SwiftAudio) Record(filename string) error {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	result := C.SwiftAudio_record(cFilename)
	if result != 0 {
		return fmt.Errorf("failed to start recording")
	}
	return nil
}

// StopRecording stops the current recording
func (a *SwiftAudio) StopRecording() error {
	result := C.SwiftAudio_stopRecording()
	if result != 0 {
		return fmt.Errorf("failed to stop recording")
	}
	return nil
}

// PlayFile plays the entire audio file
func (a *SwiftAudio) PlayFile(playerID int, filename string, cents float32) error {
	if !a.Started {
		return fmt.Errorf("audio engine not started")
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	result := C.SwiftAudio_playFile(C.int(playerID), cFilename, C.float(cents))
	if result != 0 {
		return fmt.Errorf("failed to play file")
	}
	return nil
}

// PlayRegion plays a region of the audio file from startFrame to endFrame
func (a *SwiftAudio) PlayRegion(playerID int, filename string, startFrame int, endFrame int, cents float32) error {
	if !a.Started {
		return fmt.Errorf("audio engine not started")
	}
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	result := C.SwiftAudio_playRegion(C.int(playerID), cFilename, C.int(startFrame), C.int(endFrame), C.float(cents))
	if result != 0 {
		return fmt.Errorf("failed to play region")
	}
	return nil
}

// TrimFile rewrites the audio file to only contain frames from startFrame to endFrame
func (a *SwiftAudio) TrimFile(filename string, startFrame int, endFrame int) error {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	result := C.SwiftAudio_trimFile(cFilename, C.int(startFrame), C.int(endFrame))
	if result != 0 {
		return fmt.Errorf("failed to trim file")
	}
	return nil
}

// RenderPitchedFile creates a new audio file with pitch shifting applied offline
func (a *SwiftAudio) RenderPitchedFile(sourceFilename string, targetFilename string, cents float32) error {
	cSource := C.CString(sourceFilename)
	defer C.free(unsafe.Pointer(cSource))

	cTarget := C.CString(targetFilename)
	defer C.free(unsafe.Pointer(cTarget))

	result := C.SwiftAudio_renderPitchedFile(cSource, cTarget, C.float(cents))
	if result != 0 {
		return fmt.Errorf("failed to render pitched file")
	}
	return nil
}

// GetAudioDevices returns a list of available audio output devices
func (a *SwiftAudio) GetAudioDevices() ([]AudioDevice, error) {
	cDevices := C.SwiftAudio_getAudioDevices()
	if cDevices == nil {
		return nil, fmt.Errorf("failed to get audio devices")
	}
	defer C.free(unsafe.Pointer(cDevices))

	devicesStr := C.GoString(cDevices)
	if devicesStr == "" {
		return []AudioDevice{}, nil
	}

	var devices []AudioDevice
	lines := strings.Split(strings.TrimSpace(devicesStr), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) == 2 {
			devices = append(devices, AudioDevice{
				ID:   parts[0],
				Name: parts[1],
			})
		}
	}

	return devices, nil
}
