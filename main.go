package main

import (
	"fmt"
	"os"

	"smplr/audio"
	"smplr/player"
	"smplr/smplrmidi"
	"smplr/wavfile"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

const VERSION = "v0.1.0-alpha.9"

// DecibelLevelMsg is sent when recording decibel levels are updated
type DecibelLevelMsg struct {
	Level float32
}

var (
	audioDevice string
)

var rootCmd = &cobra.Command{
	Use:   "smplr",
	Short: "A MIDI-controlled audio sampler with a terminal UI",
	Long:  `smplr is a terminal-based audio sampler that loads WAV files and triggers them via MIDI input with pitch shifting, trimming, and waveform display capabilities.`,
	Run:   runSampler,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of smplr",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("smplr", VERSION)
	},
}

var infoCmd = &cobra.Command{
	Use:   "info <wav-file>",
	Short: "Display metadata information for a WAV file",
	Args:  cobra.ExactArgs(1),
	Run:   runInfo,
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List available audio output devices",
	Run:   runDevices,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&audioDevice, "device", "", "Audio output device name (use 'smplr devices' to list available devices)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(devicesCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runInfo(cmd *cobra.Command, args []string) {
	filename := args[0]

	// Read metadata
	metadata, err := wavfile.ReadMetadata(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading metadata: %v\n", err)
		os.Exit(1)
	}

	// Output metadata
	fmt.Printf("File: %s\n", filename)
	fmt.Printf("Sample Rate: %d Hz\n", metadata.SampleRate)
	fmt.Printf("Frames: %d\n", metadata.NumFrames)
	fmt.Printf("Duration: %.2f seconds\n", metadata.Duration)
	fmt.Printf("Waveform Segments: %d\n", len(metadata.WaveformData.Peaks))
}

func runDevices(cmd *cobra.Command, args []string) {
	audioApi := audio.NewSwiftAudio()
	if err := audioApi.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing audio: %v\n", err)
		os.Exit(1)
	}

	devices, err := audioApi.GetAudioDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting audio devices: %v\n", err)
		os.Exit(1)
	}

	if len(devices) == 0 {
		fmt.Println("No audio devices found")
		return
	}

	fmt.Println("Available audio devices:")
	for _, device := range devices {
		fmt.Printf("  %s - %s\n", device.ID, device.Name)
	}
}

func runSampler(cmd *cobra.Command, args []string) {
	// Create channel for metadata loading
	metadataChan := make(chan wavfile.MetadataLoadedMsg)
	files := wavfile.LoadFiles(metadataChan)
	audioApi := audio.NewSwiftAudio()
	// Create program with initial model
	m := initialModel(&files, audioApi, audioDevice)
	p := tea.NewProgram(m, tea.WithAltScreen())
	audioApi.Init()

	// Create and register playback completion channel
	playbackCompletionChan := make(chan int)
	audio.SetPlaybackCompletionChannel(playbackCompletionChan)

	// Create and register decibel level channel
	decibelLevelChan := make(chan float32)
	audio.SetDecibelLevelChannel(decibelLevelChan)

	smplrPlayer := player.NewPlayer(&files, audioApi, p.Send)
	smplrPlayer.Start()
	stopFunc, err := smplrmidi.Start(smplrPlayer.MsgChan)
	if err != nil {
		fmt.Printf("Error starting MIDI input: %v", err)
		os.Exit(1)
	}

	// Start goroutine to forward metadata messages to the program
	go func() {
		for msg := range metadataChan {
			p.Send(msg)
		}
	}()

	// Start goroutine to forward playback completion messages to the program
	go func() {
		for playerID := range playbackCompletionChan {
			// Find the filename by looking up playerID in files list
			var filename string
			for _, file := range files {
				if file.PlayerId == playerID {
					filename = file.Name
					break
				}
			}
			if filename != "" {
				p.Send(wavfile.PlaybackFinishedMsg{Filename: filename})
			}
		}
	}()

	// Start goroutine to forward decibel level messages to the program
	go func() {
		for db := range decibelLevelChan {
			p.Send(DecibelLevelMsg{Level: db})
		}
	}()

	// Run program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		stopFunc()
		os.Exit(1)
	}
}
