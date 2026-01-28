package sequencer

import (
	"fmt"

	"go-sequence/midi"
	"go-sequence/widgets"
)

type SessionDevice struct {
	manager *Manager

	// UI state
	cursorRow  int // pattern
	cursorCol  int // track
	viewRows   int // how many rows to show (default 8)
	viewOffset int // scroll offset
}

func NewSessionDevice(manager *Manager) *SessionDevice {
	return &SessionDevice{
		manager:    manager,
		cursorRow:  0,
		cursorCol:  0,
		viewRows:   8,
		viewOffset: 0,
	}
}

// getTrackPatternState returns (pattern, next) for a track by reading global state
func (s *SessionDevice) getTrackPatternState(trackIdx int) (pattern, next int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return 0, 0
	}
	ts := S.Tracks[trackIdx]
	switch ts.Type {
	case DeviceTypeDrum:
		if ts.Drum != nil {
			return ts.Drum.PlayingPatternIdx, ts.Drum.Next
		}
	case DeviceTypePiano:
		if ts.Piano != nil {
			return ts.Piano.Pattern, ts.Piano.Next
		}
	case DeviceTypeMetropolix:
		if ts.Metropolix != nil {
			return ts.Metropolix.Pattern, ts.Metropolix.Next
		}
	}
	return 0, 0
}

// queuePattern queues a pattern on a device
func (s *SessionDevice) queuePattern(trackIdx, patternIdx int) {
	dev := s.manager.GetDevice(trackIdx)
	if dev != nil {
		dev.QueuePattern(patternIdx, S.Tick)
	}
}

// Device interface implementation - queue-based (stubs for non-music device)

func (s *SessionDevice) FillUntil(tick int64)           {}
func (s *SessionDevice) PeekNextEvent() *midi.Event     { return nil }
func (s *SessionDevice) PopNextEvent() *midi.Event      { return nil }
func (s *SessionDevice) ClearQueue()                    {}
func (s *SessionDevice) QueuePattern(p int, atTick int64) {}
func (s *SessionDevice) CurrentPattern() int            { return 0 }
func (s *SessionDevice) NextPattern() int               { return -1 }
func (s *SessionDevice) ContentMask() []bool            { return make([]bool, NumPatterns) }

func (s *SessionDevice) HandleMIDI(event midi.Event) {
	if event.Type == midi.NoteOn && int(event.Channel) < 8 {
		s.queuePattern(int(event.Channel), int(event.Note))
	}
}

func (s *SessionDevice) ToggleRecording() {}
func (s *SessionDevice) TogglePreview()   {}
func (s *SessionDevice) IsRecording() bool { return false }
func (s *SessionDevice) IsPreviewing() bool { return false }

func (s *SessionDevice) View() string {
	var out string
	out += "SESSION  Clip Launcher\n\n"
	out += "       "
	for i := 0; i < 8; i++ {
		ts := S.Tracks[i]
		if ts.Name != "" {
			out += fmt.Sprintf(" %s ", ts.Name[:min(2, len(ts.Name))])
		} else {
			out += fmt.Sprintf(" T%d ", i+1)
		}
	}
	out += "\n"

	masks := make([][]bool, 8)
	for i := 0; i < 8; i++ {
		dev := s.manager.GetDevice(i)
		if dev != nil {
			masks[i] = dev.ContentMask()
		} else {
			masks[i] = make([]bool, NumPatterns)
		}
	}

	for row := s.viewOffset; row < s.viewOffset+s.viewRows && row < NumPatterns; row++ {
		out += fmt.Sprintf("Pat %2d: ", row+1)
		for col := 0; col < 8; col++ {
			pattern, next := s.getTrackPatternState(col)
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

	// Legend
	out += "\n▶ playing  ◆ queued  · has content  - empty track\n"

	// Key help
	out += "\n"
	out += widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: "h / l", Desc: "move cursor left/right (tracks)"},
			{Key: "j / k", Desc: "move cursor up/down (patterns)"},
			{Key: "space", Desc: "launch clip"},
			{Key: "1-8", Desc: "focus device on that track"},
		}},
	})

	// Launchpad
	out += "\n\n"
	out += s.renderLaunchpadHelp()

	return out
}

func (s *SessionDevice) RenderLEDs() []LEDState {
	var leds []LEDState

	// Colors matching HelpLayout - RGB values
	clipsPlaying := [3]uint8{71, 13, 121}      // purple - playing with content
	clipsPlayingEmpty := [3]uint8{40, 40, 40}  // gray - playing but empty
	clipsBright := [3]uint8{140, 26, 242}      // bright purple - has content
	clipsQueued := [3]uint8{255, 200, 0}       // yellow - queued
	clipsDim := [3]uint8{20, 4, 30}            // very dim purple - empty slot
	sceneColor := [3]uint8{148, 18, 126}       // scene buttons

	masks := make([][]bool, 8)
	for i := 0; i < 8; i++ {
		dev := s.manager.GetDevice(i)
		if dev != nil {
			masks[i] = dev.ContentMask()
		} else {
			masks[i] = make([]bool, NumPatterns)
		}
	}

	// Main grid - clips
	for col := 0; col < 8; col++ {
		pattern, next := s.getTrackPatternState(col)

		for lpRow := 0; lpRow < 8; lpRow++ {
			patternRow := s.viewOffset + (7 - lpRow)

			var color [3]uint8 = clipsDim // empty slots still visible
			var channel uint8 = midi.ChannelStatic

			if patternRow < NumPatterns {
				hasContent := masks[col][patternRow]

				if pattern == patternRow {
					if hasContent {
						// Playing with content - bright pulsing
						color = clipsPlaying
						channel = midi.ChannelPulse
					} else {
						// Playing but empty - gray
						color = clipsPlayingEmpty
					}
				} else if next == patternRow && next != pattern {
					if hasContent {
						// Queued with content
						color = clipsQueued
						channel = midi.ChannelPulse
					} else {
						// Queued but empty
						color = clipsDim
					}
				} else if hasContent {
					// Has content but not playing
					color = clipsBright
				}
				// Empty + not playing stays clipsDim
			}

			leds = append(leds, LEDState{Row: lpRow, Col: col, Color: color, Channel: channel})
		}
	}

	// Right column - scene launch buttons
	for row := 0; row < 8; row++ {
		leds = append(leds, LEDState{Row: row, Col: 8, Color: sceneColor, Channel: midi.ChannelStatic})
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
		if s.cursorCol < 7 {
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
		s.queuePattern(s.cursorCol, s.cursorRow)
	}
}

func (s *SessionDevice) HandlePad(row, col int) {
	patternRow := s.viewOffset + (7 - row)
	if col < 8 && patternRow < NumPatterns {
		s.queuePattern(col, patternRow)
	}
}

func (s *SessionDevice) renderLaunchpadHelp() string {
	// Define colors
	clipColor := [3]uint8{71, 13, 121}     // clips with content
	playingColor := [3]uint8{71, 13, 121}  // currently playing
	queuedColor := [3]uint8{255, 200, 0}   // queued for playback
	emptyColor := [3]uint8{20, 4, 30}      // empty slot
	topRowColor := [3]uint8{111, 10, 126}  // top row mode buttons
	sceneColor := [3]uint8{148, 18, 126}   // scene launch buttons

	var out string

	// Top row
	topColors := make([][3]uint8, 8)
	for i := 0; i < 8; i++ {
		topColors[i] = topRowColor
	}
	out += widgets.RenderPadRow(topColors) + "\n"

	// Main grid with right column
	var grid [8][8][3]uint8
	var rightCol [8][3]uint8

	// Get content masks for all tracks
	masks := make([][]bool, 8)
	for i := 0; i < 8; i++ {
		dev := s.manager.GetDevice(i)
		if dev != nil {
			masks[i] = dev.ContentMask()
		} else {
			masks[i] = make([]bool, NumPatterns)
		}
	}

	// Build the grid with actual clip state
	for lpRow := 0; lpRow < 8; lpRow++ {
		patternRow := s.viewOffset + (7 - lpRow)

		for col := 0; col < 8; col++ {
			pattern, next := s.getTrackPatternState(col)

			// Default to empty
			color := emptyColor

			if patternRow < NumPatterns {
				hasContent := masks[col][patternRow]

				if pattern == patternRow {
					// Currently playing
					color = playingColor
				} else if next == patternRow && next != pattern {
					// Queued
					color = queuedColor
				} else if hasContent {
					// Has content
					color = clipColor
				}
			}

			grid[lpRow][col] = color
		}

		// Right column - scene buttons
		rightCol[lpRow] = sceneColor
	}

	out += widgets.RenderPadGrid(grid, &rightCol) + "\n"

	// Legend
	out += widgets.RenderLegendItem(clipColor, "Clips", "tap to launch clip") + "\n"
	out += widgets.RenderLegendItem(playingColor, "Playing", "currently playing clip") + "\n"
	out += widgets.RenderLegendItem(queuedColor, "Queued", "queued for next bar") + "\n"
	out += widgets.RenderLegendItem(emptyColor, "Empty", "no content") + "\n"
	out += widgets.RenderLegendItem(sceneColor, "Scene", "launch entire row")

	return out
}
