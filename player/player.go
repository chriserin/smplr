package player

import (
	"smplr/audio"
	"smplr/wavfile"
	"sync"
	"time"

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
			} else if msg.Type().Is(midi.NoteOffMsg) {
				var channel, note, velocity uint8
				msg.GetNoteOff(&channel, &note, &velocity)
				p.stopNote(channel, note)
			}
		}
	}
}

type trigger struct {
	channel uint8
	note    uint8
}

var possibleTriggers = map[trigger]struct{}{}
var possibleTriggersMutex = sync.Mutex{}

func addTrigger(channel uint8, note uint8) {
	possibleTriggersMutex.Lock()
	defer possibleTriggersMutex.Unlock()
	trig := trigger{channel: channel, note: note}
	possibleTriggers[trig] = struct{}{}
}

func removeTrigger(channel uint8, note uint8) {
	possibleTriggersMutex.Lock()
	defer possibleTriggersMutex.Unlock()
	trig := trigger{channel: channel, note: note}
	delete(possibleTriggers, trig)
}

func delayedRemoveTrigger(channel uint8, note uint8) {
	// NOTE: triggers are ~20 millis so give it some grace time before eliminating the trigger possibility
	time.AfterFunc(26*time.Millisecond, func() {
		removeTrigger(channel, note)
	})
}

// playNote finds and plays the WAV file matching the MIDI channel and note
func (p *Player) playNote(channel uint8, note uint8) {
	midiChannel := int(channel) + 1
	midiNote := int(note)

	for i := range *p.files {
		file := &(*p.files)[i]
		if file.MidiChannel == midiChannel && file.MidiNote == midiNote {
			if file.Metadata != nil && !file.Corrupted {
				// Stop and restart if already playing
				if file.PlayingCount > 0 {
					p.audio.StopPlayer(file.PlayerId)
					file.PlayingCount = 0
				}
				// Use pitched file if it exists, otherwise use original
				filename := file.Name
				if file.PitchedFileName != "" {
					filename = file.PitchedFileName
				}
				// No real-time pitch shifting - files are pre-rendered
				err := p.audio.PlayRegion(file.PlayerId, filename, file.StartFrame, file.EndFrame, 0)
				if err != nil {
					panic("Error playing region: " + err.Error())
				} else {
					addTrigger(channel, note)
					delayedRemoveTrigger(channel, note)
				}
				p.sendFn(wavfile.PlaybackStartedMsg{Filename: file.Name})
			}
			return
		}
	}
}

// stopNote finds and stops the WAV file matching the MIDI channel and note
func (p *Player) stopNote(channel uint8, note uint8) {
	midiChannel := int(channel) + 1
	midiNote := int(note)

	if _, exists := possibleTriggers[trigger{channel: channel, note: note}]; exists {
		// If this note-off corresponds to a recent note-on, ignore it
		removeTrigger(channel, note)
		return
	}

	for i := range *p.files {
		file := &(*p.files)[i]
		if file.MidiChannel == midiChannel && file.MidiNote == midiNote {
			if file.PlayingCount > 0 {
				p.audio.StopPlayer(file.PlayerId)
				file.PlayingCount = 0
			}
			return
		}
	}
}
