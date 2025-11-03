# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run

Build the project:
```bash
./build.sh
```

This compiles the Swift audio bridge (AudioBridge.o) and builds the Go application.

Run the application:
```bash
./smplr
```

## Architecture Overview

**smplr** is a MIDI-controlled audio sampler with a terminal UI. The architecture bridges Go and Swift to combine Go's ecosystem with macOS CoreAudio's low-latency playback.

### Language Bridge (Go ↔ Swift)

The core architectural pattern is a CGO bridge between Go and Swift:

- **audio/audio.go**: Defines the `Audio` interface and implements `SwiftAudio` using CGO to call Swift functions
- **audio/AudioBridge.swift**: Swift implementation using AVFoundation for audio playback, recording, and file operations
- **build.sh**: Compiles Swift to object file (AudioBridge.o) which is linked into the Go binary via `#cgo LDFLAGS`

The bridge uses C function pointers for callbacks (playback completion notifications flow from Swift → C wrapper → Go).

### Component Structure

**Main packages:**

- **main.go**: Entry point, initializes Bubble Tea TUI, connects all components via channels
- **player/**: MIDI message processor, maps MIDI notes to WAV files and triggers playback
- **smplrmidi/**: MIDI input handling using rtmididrv (creates virtual MIDI input port)
- **wavfile/**: WAV file metadata reading, waveform visualization data pre-calculation
- **audio/**: Audio interface with two implementations:
  - `StubAudio`: No-op implementation for testing
  - `SwiftAudio`: Production implementation via CGO bridge

**View layer:**

- **view.go**: Main TUI rendering (file list with MIDI mappings)
- **viewwave.go**: Waveform visualization with start/end markers
- **update.go**: Bubble Tea update logic, keyboard handling, state management

### Data Flow

1. WAV files loaded from current directory with async metadata loading
2. MIDI messages arrive via `smplrmidi` → sent to `player` via channel
3. `player` matches MIDI note to WavFile, calls audio interface to trigger playback
4. Swift audio engine plays region with pitch shift, sends completion callback
5. Completion flows back through CGO → channel → Bubble Tea update

### Key Concepts

- **Player IDs**: Each WAV file gets a player ID from the audio engine for efficient playback
- **Region playback**: Files can have start/end frame markers for trimming
- **Pitch shifting**: Semitone-based pitch control per file (stored as cents: semitones × 100)
- **Async metadata**: Files appear immediately in UI, metadata loads in background goroutines
- **Playback counting**: `PlayingCount` reference tracks active playbacks per file for UI indicators

### State Management

The Bubble Tea model in update.go manages:
- File list with MIDI mappings (channel, note, pitch)
- UI cursor and editing mode (inline editing of MIDI parameters)
- Recording state
- Waveform marker editing (start/end frames, step size)

All state changes flow through the Update function, with external events (MIDI, metadata loading, playback completion) sent via channels to `p.Send()`.
