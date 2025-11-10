package smplrmidi

import (
	"fmt"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

func Start(midiChannel chan midi.Message) (func(), error) {

	largestID := FindLargestSmplrMidiID()

	driver, err := rtmididrv.New()
	if err != nil {
		fmt.Println("Can't open MIDI driver:", err)

	}
	in, err := driver.OpenVirtualIn(fmt.Sprintf("smplr-midi-in-%d", largestID+1))
	if err != nil {
		fmt.Println("Can't open virtual MIDI input port:", err)
	}

	// Listen for MIDI messages
	stop, err := midi.ListenTo(in, func(msg midi.Message, timestampms int32) {
		var channel, note, velocity uint8

		switch {
		case msg.GetNoteOn(&channel, &note, &velocity):
			midiChannel <- msg
		case msg.GetNoteOff(&channel, &note, &velocity):
			midiChannel <- msg
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to listen to MIDI input: %w", err)
	}
	return stop, nil
}

func FindLargestSmplrMidiID() int {
	outports := midi.GetOutPorts()
	var largestSmplrID int
	for _, outport := range outports {
		name := outport.String()
		var smplrID int
		n, err := fmt.Sscanf(name, "smplr-midi-in-%d", &smplrID)
		if err == nil && n == 1 {
			if smplrID > largestSmplrID {
				largestSmplrID = smplrID
			}
		}
	}
	return largestSmplrID
}
