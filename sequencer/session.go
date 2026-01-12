package sequencer

import (
	"fmt"

	"go-sequence/midi"
	"go-sequence/widgets"
)

type SessionDevice struct {
	devices []Device

	// UI state
	cursorRow  int // pattern
	cursorCol  int // device
	viewRows   int // how many rows to show (default 8)
	viewOffset int // scroll offset
}

func NewSessionDevice(devices []Device) *SessionDevice {
	return &SessionDevice{
		devices:    devices,
		cursorRow:  0,
		cursorCol:  0,
		viewRows:   8,
		viewOffset: 0,
	}
}

// Device interface implementation

func (s *SessionDevice) Tick(step int) []midi.Event {
	// Session doesn't output MIDI
	return nil
}

func (s *SessionDevice) QueuePattern(p int) (pattern, next int) {
	return 0, 0
}

func (s *SessionDevice) GetState() (pattern, next int) {
	return 0, 0
}

func (s *SessionDevice) ContentMask() []bool {
	return make([]bool, NumPatterns)
}

func (s *SessionDevice) HandleMIDI(event midi.Event) {
	if event.Type == midi.NoteOn && int(event.Channel) < len(s.devices) {
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

	masks := make([][]bool, len(s.devices))
	for i, dev := range s.devices {
		masks[i] = dev.ContentMask()
	}

	for row := s.viewOffset; row < s.viewOffset+s.viewRows && row < NumPatterns; row++ {
		out += fmt.Sprintf("Pat %2d: ", row+1)
		for col, dev := range s.devices {
			pattern, next := dev.GetState()
			hasContent := masks[col][row]

			char := " "
			if hasContent {
				char = "·"
			}
			if pattern == row {
				char = "▶"
			} else if next == row && next != pattern {
				char = "◆"
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

func (s *SessionDevice) RenderLEDs() []LEDState {
	var leds []LEDState

	masks := make([][]bool, len(s.devices))
	for i, dev := range s.devices {
		masks[i] = dev.ContentMask()
	}

	for col, dev := range s.devices {
		pattern, next := dev.GetState()

		for lpRow := 0; lpRow < 8; lpRow++ {
			patternRow := s.viewOffset + (7 - lpRow)

			var color uint8 = midi.ColorOff
			var channel uint8 = 0

			if patternRow < NumPatterns {
				hasContent := masks[col][patternRow]

				if pattern == patternRow {
					color = midi.ColorGreen
				} else if next == patternRow && next != pattern {
					color = midi.ColorYellow
					channel = 2
				} else if hasContent {
					color = midi.ColorDim
				}
			}

			leds = append(leds, LEDState{Row: lpRow, Col: col, Color: color, Channel: channel})
		}
	}

	return leds
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
			if s.cursorRow >= s.viewOffset+s.viewRows {
				s.viewOffset = s.cursorRow - s.viewRows + 1
			}
		}
	case "k", "up":
		if s.cursorRow > 0 {
			s.cursorRow--
			if s.cursorRow < s.viewOffset {
				s.viewOffset = s.cursorRow
			}
		}
	case " ", "enter":
		if s.cursorCol < len(s.devices) {
			s.devices[s.cursorCol].QueuePattern(s.cursorRow)
		}
	}
}

func (s *SessionDevice) HandlePad(row, col int) {
	patternRow := s.viewOffset + (7 - row)
	if col < len(s.devices) && patternRow < NumPatterns {
		s.devices[col].QueuePattern(patternRow)
	}
}

func (s *SessionDevice) HelpLayout() widgets.LaunchpadLayout {
	topRowColor := [3]uint8{111, 10, 126}
	clipsColor := [3]uint8{71, 13, 121}
	sceneColor := [3]uint8{148, 18, 126}

	var layout widgets.LaunchpadLayout

	for i := 0; i < 8; i++ {
		layout.TopRow[i] = widgets.PadConfig{Color: topRowColor, Tooltip: "Mode"}
	}

	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			layout.Grid[row][col] = widgets.PadConfig{Color: clipsColor, Tooltip: "Launch Clip"}
		}
	}

	for i := 0; i < 8; i++ {
		layout.RightCol[i] = widgets.PadConfig{Color: sceneColor, Tooltip: "Launch Scene"}
	}

	return layout
}
