package player

import (
	"smplr/audio"
	"smplr/wavfile"

	tea "github.com/charmbracelet/bubbletea"
	"gitlab.com/gomidi/midi/v2"
)

// Player handles MIDI input and plays corresponding WAV files
type Player struct {
	files    *[]wavfile.WavFile
	audio    audio.Audio
	MsgChan  chan midi.Message
	stopChan chan struct{}
	sendFn   func(msg tea.Msg)
}

// NewPlayer creates a new MIDI player
func NewPlayer(files *[]wavfile.WavFile, audio audio.Audio, sendFn func(msg tea.Msg)) *Player {
	return &Player{
		files:    files,
		audio:    audio,
		MsgChan:  make(chan midi.Message),
		stopChan: make(chan struct{}),
		sendFn:   sendFn,
	}
}

// Start initializes MIDI input and starts the player loop
func (p *Player) Start() error {
	go p.playerLoop()

	return nil
}

// Stop stops the MIDI player
func (p *Player) Stop() {
	close(p.stopChan)
	close(p.MsgChan)
}

// playerLoop processes MIDI messages and plays corresponding files
func (p *Player) playerLoop() {

	for {
		select {
		case <-p.stopChan:
			return
		case msg := <-p.MsgChan:
			if msg.Type().Is(midi.NoteOnMsg) {
				var channel, note, velocity uint8
				msg.GetNoteOn(&channel, &note, &velocity)
				p.playNote(channel, note)
			}
		}
	}
}

// playNote finds and plays the WAV file matching the MIDI channel and note
func (p *Player) playNote(channel uint8, note uint8) {
	midiChannel := int(channel) + 1
	midiNote := int(note)

	for _, file := range *p.files {
		if file.MidiChannel == midiChannel && file.MidiNote == midiNote {
			if file.Metadata != nil {
				cents := float32(file.Pitch * 100)
				err := p.audio.PlayRegion(file.PlayerId, file.Name, file.StartFrame, file.EndFrame, cents)
				if err != nil {
					panic("Error playing region: " + err.Error())
				}
				p.sendFn(wavfile.PlaybackStartedMsg{Filename: file.Name})
			}
			return
		}
	}
}
