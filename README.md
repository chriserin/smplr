# smplr

A MIDI-controlled audio sampler with a terminal user interface.

## Features

- Load and trigger WAV file samples via MIDI input
- Interactive terminal UI powered by Bubble Tea
- Low-latency audio playback using Swift's CoreAudio

## Prerequisites

- Go 1.21 or later
- Swift compiler (macOS)
- MIDI controller (optional)

## Building

```bash
./build.sh
```

This compiles the Swift audio bridge and builds the Go application.

## Running

```bash
./smplr
```

The application will load WAV files from the current directory and map them to MIDI inputs.

## Architecture

- **Go**: Main application, TUI, and MIDI handling
- **Swift**: Low-level audio playback via CoreAudio
- **Bubble Tea**: Terminal UI framework
