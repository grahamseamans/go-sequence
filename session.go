package main

import "fmt"

type SessionDevice struct {
	controller *Controller
	devices    []Device

	// UI state
	cursorRow   int // pattern
	cursorCol   int // device
	viewRows    int // how many rows to show (default 8)
	viewOffset  int // scroll offset
}

func NewSessionDevice(controller *Controller, devices []Device) *SessionDevice {
	return &SessionDevice{
		controller: controller,
		devices:    devices,
		cursorRow:  0,
		cursorCol:  0,
		viewRows:   8,
		viewOffset: 0,
	}
}

// Device interface implementation

func (s *SessionDevice) Tick(step int) []MIDIEvent {
	// Session doesn't output MIDI
	return nil
}

func (s *SessionDevice) QueuePattern(p int) (pattern, next int) {
	// Session doesn't have patterns itself
	return 0, 0
}

func (s *SessionDevice) GetState() (pattern, next int) {
	return 0, 0
}

func (s *SessionDevice) ContentMask() []bool {
	// Session doesn't have patterns itself
	return make([]bool, NumPatterns)
}

func (s *SessionDevice) HandleMIDI(event MIDIEvent) {
	// External MIDI can trigger clips
	// Channel = device, Note = pattern
	if event.Type == NoteOn && int(event.Channel) < len(s.devices) {
		s.devices[event.Channel].QueuePattern(int(event.Note))
	}
}

func (s *SessionDevice) View() string {
	var out string
	out += "SESSION\n"
	out += "       "
	for i := range s.devices {
		out += fmt.Sprintf(" D%d ", i+1)
	}
	out += "\n"

	// Get content masks for all devices
	masks := make([][]bool, len(s.devices))
	for i, dev := range s.devices {
		masks[i] = dev.ContentMask()
	}

	for row := s.viewOffset; row < s.viewOffset+s.viewRows && row < NumPatterns; row++ {
		out += fmt.Sprintf("Pat %2d: ", row+1)
		for col, dev := range s.devices {
			pattern, next := dev.GetState()
			hasContent := masks[col][row]

			char := " " // empty
			if hasContent {
				char = "·" // has content but not playing
			}
			if pattern == row {
				char = "▶" // playing
			} else if next == row && next != pattern {
				char = "◆" // queued
			}

			if row == s.cursorRow && col == s.cursorCol {
				out += fmt.Sprintf("[%s] ", char)
			} else {
				out += fmt.Sprintf(" %s  ", char)
			}
		}
		out += "\n"
	}

	return out
}

func (s *SessionDevice) HandleKey(key string) {
	switch key {
	case "h", "left":
		if s.cursorCol > 0 {
			s.cursorCol--
		}
	case "l", "right":
		if s.cursorCol < len(s.devices)-1 {
			s.cursorCol++
		}
	case "j", "down":
		if s.cursorRow < NumPatterns-1 {
			s.cursorRow++
			// scroll if needed
			if s.cursorRow >= s.viewOffset+s.viewRows {
				s.viewOffset = s.cursorRow - s.viewRows + 1
			}
		}
	case "k", "up":
		if s.cursorRow > 0 {
			s.cursorRow--
			// scroll if needed
			if s.cursorRow < s.viewOffset {
				s.viewOffset = s.cursorRow
			}
		}
	case " ", "enter":
		// Queue pattern on selected device
		if s.cursorCol < len(s.devices) {
			s.devices[s.cursorCol].QueuePattern(s.cursorRow)
		}
	}
}

func (s *SessionDevice) HandlePad(row, col int) {
	// col = device, row = pattern (inverted because row 0 is bottom on Launchpad)
	// Map to current view
	patternRow := s.viewOffset + (7 - row)
	if col < len(s.devices) && patternRow < NumPatterns {
		s.devices[col].QueuePattern(patternRow)
	}
}

func (s *SessionDevice) UpdateLEDs() {
	if s.controller == nil {
		return
	}

	// Get content masks
	masks := make([][]bool, len(s.devices))
	for i, dev := range s.devices {
		masks[i] = dev.ContentMask()
	}

	for col, dev := range s.devices {
		pattern, next := dev.GetState()

		for lpRow := 0; lpRow < 8; lpRow++ {
			patternRow := s.viewOffset + (7 - lpRow)
			if patternRow >= NumPatterns {
				s.controller.SetPad(lpRow, col, ColorOff, 0)
				continue
			}

			hasContent := masks[col][patternRow]
			var color uint8 = ColorOff
			var channel uint8 = 0

			if pattern == patternRow {
				color = ColorGreen
			} else if next == patternRow && next != pattern {
				color = ColorYellow
				channel = 2 // pulsing
			} else if hasContent {
				color = ColorDim
			}

			s.controller.SetPad(lpRow, col, color, channel)
		}
	}
}
