package mappings

import (
	tea "github.com/charmbracelet/bubbletea"
)

type Command int

const (
	Unknown Command = iota
	Quit
	CursorUp
	CursorDown
	EditChannel
	EditNote
	EditPitch
	Enter
	Escape
	Backspace
	NumberInput
	Recording
	MarkerLeft
	MarkerRight
	MarkerStepIncrease
	MarkerStepDecrease
	SelectStartMarker
	SelectEndMarker
	PlayFile
	PlayRegion
	TrimFile
)

type Mapping struct {
	Command   Command
	LastValue string
}

func ProcessKey(msg tea.KeyMsg, editing bool) Mapping {
	keyStr := msg.String()

	if editing {
		return processEditingKey(keyStr)
	}

	return processNavigationKey(keyStr)
}

func processEditingKey(keyStr string) Mapping {
	switch keyStr {
	case "enter":
		return Mapping{Command: Enter, LastValue: keyStr}
	case "esc":
		return Mapping{Command: Escape, LastValue: keyStr}
	case "backspace":
		return Mapping{Command: Backspace, LastValue: keyStr}
	case "-":
		return Mapping{Command: NumberInput, LastValue: keyStr}
	default:
		// Check if it's a number
		if len(keyStr) == 1 && keyStr[0] >= '0' && keyStr[0] <= '9' {
			return Mapping{Command: NumberInput, LastValue: keyStr}
		}
		return Mapping{Command: Unknown, LastValue: keyStr}
	}
}

func processNavigationKey(keyStr string) Mapping {
	switch keyStr {
	case "ctrl+c", "q":
		return Mapping{Command: Quit, LastValue: keyStr}
	case "up", "k":
		return Mapping{Command: CursorUp, LastValue: keyStr}
	case "down", "j":
		return Mapping{Command: CursorDown, LastValue: keyStr}
	case "c":
		return Mapping{Command: EditChannel, LastValue: keyStr}
	case "n":
		return Mapping{Command: EditNote, LastValue: keyStr}
	case "p":
		return Mapping{Command: EditPitch, LastValue: keyStr}
	case "r":
		return Mapping{Command: Recording, LastValue: keyStr}
	case "h":
		return Mapping{Command: MarkerLeft, LastValue: keyStr}
	case "l":
		return Mapping{Command: MarkerRight, LastValue: keyStr}
	case "+", "=":
		return Mapping{Command: MarkerStepIncrease, LastValue: keyStr}
	case "-":
		return Mapping{Command: MarkerStepDecrease, LastValue: keyStr}
	case "<":
		return Mapping{Command: SelectStartMarker, LastValue: keyStr}
	case ">":
		return Mapping{Command: SelectEndMarker, LastValue: keyStr}
	case " ":
		return Mapping{Command: PlayFile, LastValue: keyStr}
	case "a":
		return Mapping{Command: PlayRegion, LastValue: keyStr}
	case "t":
		return Mapping{Command: TrimFile, LastValue: keyStr}
	default:
		return Mapping{Command: Unknown, LastValue: keyStr}
	}
}
