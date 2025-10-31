package main

import (
	"fmt"
	"os"

	"smplr/audio"
	"smplr/player"
	"smplr/smplrmidi"
	"smplr/wavfile"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Create channel for metadata loading
	metadataChan := make(chan wavfile.MetadataLoadedMsg)
	files := wavfile.LoadFiles(metadataChan)
	audioApi := audio.NewSwiftAudio()
	// Create program with initial model
	m := initialModel(&files, audioApi)
	p := tea.NewProgram(m, tea.WithAltScreen())
	audioApi.Init()

	// Create and register playback completion channel
	playbackCompletionChan := make(chan int)
	audio.SetPlaybackCompletionChannel(playbackCompletionChan)

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

	// Run program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		stopFunc()
		os.Exit(1)
	}
}
