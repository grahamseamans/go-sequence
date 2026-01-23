package sequencer

import (
	"fmt"

	"go-sequence/midi"
	"go-sequence/widgets"
)

// DrumDevice reads/writes from central DrumState
type DrumDevice struct {
	state       *DrumState
	previewChan chan int // sends slot index when preview sound should play

	// Confirmation dialog
	confirmMode   bool
	confirmMsg    string
	confirmAction func()
}

// NewDrumDevice creates a device that operates on the given state
func NewDrumDevice(state *DrumState) *DrumDevice {
	return &DrumDevice{
		state:       state,
		previewChan: make(chan int, 16),
	}
}

// PreviewChan returns the channel for preview events (slot indices)
func (d *DrumDevice) PreviewChan() <-chan int {
	return d.previewChan
}

// Device interface implementation

func (d *DrumDevice) Tick(step int) []midi.Event {
	s := d.state
	pat := &s.Patterns[s.Pattern]
	masterLen := pat.MasterLength()

	// Pattern switch at master length boundary
	if s.Step%masterLen == 0 {
		s.Pattern = s.Next
		pat = &s.Patterns[s.Pattern]
	}

	var events []midi.Event
	for i := 0; i < 16; i++ {
		track := &pat.Tracks[i]
		// Each track loops at its own length (polymeters!)
		trackStep := s.Step % track.Length
		step := &track.Steps[trackStep]
		if step.Active {
			events = append(events, midi.Event{
				Type:     midi.NoteOn,
				Note:     uint8(i), // Output slot index, Manager translates via kit
				Velocity: step.Velocity,
			})
		}
	}

	// Keep counting - don't reset at masterLen (breaks polymeters)
	s.Step = (s.Step + 1) % 65536
	return events
}

func (d *DrumDevice) QueuePattern(p int) (pattern, next int) {
	if p >= 0 && p < NumPatterns {
		d.state.Next = p
	}
	return d.state.Pattern, d.state.Next
}

func (d *DrumDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i := range d.state.Patterns {
		pat := &d.state.Patterns[i]
		for t := 0; t < 16; t++ {
			for s := 0; s < pat.Tracks[t].Length; s++ {
				if pat.Tracks[t].Steps[s].Active {
					mask[i] = true
					break
				}
			}
			if mask[i] {
				break
			}
		}
	}
	return mask
}

func (d *DrumDevice) HandleMIDI(event midi.Event) {
	// TODO: record mode - quantize incoming hits to steps
}

func (d *DrumDevice) ToggleRecording() {
	d.state.Recording = !d.state.Recording
}

func (d *DrumDevice) TogglePreview() {
	d.state.Preview = !d.state.Preview
}

func (d *DrumDevice) IsRecording() bool {
	return d.state.Recording
}

func (d *DrumDevice) IsPreviewing() bool {
	return d.state.Preview
}

func (d *DrumDevice) View() string {
	s := d.state
	pat := &s.Patterns[s.Editing]

	// Header - show editing vs playing
	playInfo := ""
	if s.Editing != s.Pattern {
		playInfo = fmt.Sprintf(" (playing:%d)", s.Pattern)
	}
	selectedTrack := &pat.Tracks[s.Selected]
	selectedStep := s.Step % selectedTrack.Length
	out := fmt.Sprintf("DRUM  Pattern %d%s  Step %d/%d  Track %d\n\n", s.Editing+1, playInfo, selectedStep+1, selectedTrack.Length, s.Selected+1)

	// Confirmation dialog takes over
	if d.confirmMode {
		out += "─────────────────────────────────────────────────\n"
		out += fmt.Sprintf("\n%s\n\n", d.confirmMsg)
		out += "  [y] Yes    [n] No\n"
		out += "\n─────────────────────────────────────────────────\n"
		return out
	}

	// 16x32 grid - single char per cell
	for t := 0; t < 16; t++ {
		track := &pat.Tracks[t]
		trackStep := s.Step % track.Length
		out += fmt.Sprintf("%2d ", t+1)

		for step := 0; step < 32; step++ {
			isCursor := t == s.Selected && step == s.Cursor

			var char string
			if step >= track.Length {
				if isCursor {
					char = "□"
				} else {
					char = "-"
				}
			} else if step == trackStep {
				if isCursor {
					char = "▷"
				} else {
					char = "▶"
				}
			} else if track.Steps[step].Active {
				if isCursor {
					char = "◉"
				} else {
					char = "●"
				}
			} else {
				if isCursor {
					char = "○"
				} else {
					char = "·"
				}
			}

			out += char
		}
		out += "\n"
	}

	// Key help
	out += "\n"
	out += widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: "h / l", Desc: "move cursor left/right through steps"},
			{Key: "j / k", Desc: "select track up/down"},
			{Key: "space", Desc: "toggle step on/off"},
			{Key: "[ / ]", Desc: "shorten/lengthen track"},
			{Key: "c", Desc: "clear current track"},
			{Key: "< / >", Desc: "previous/next pattern"},
		}},
	})

	// Launchpad
	out += "\n\n"
	out += d.renderLaunchpadHelp()

	return out
}

func (d *DrumDevice) RenderLEDs() []LEDState {
	var leds []LEDState
	s := d.state
	pat := &s.Patterns[s.Editing]
	track := &pat.Tracks[s.Selected]

	// Colors
	stepsColor := [3]uint8{234, 73, 116}
	stepsEmpty := [3]uint8{80, 30, 50}
	trackHasContent := [3]uint8{148, 18, 126}
	trackEmpty := [3]uint8{40, 10, 30}
	trackSelected := [3]uint8{255, 255, 255}
	commandsColor := [3]uint8{253, 157, 110}
	playheadColor := [3]uint8{255, 255, 255}
	offColor := [3]uint8{0, 0, 0}

	// Top 4 rows (rows 4-7): steps for selected track
	trackStep := s.Step % track.Length
	for stepIdx := 0; stepIdx < 32; stepIdx++ {
		row := 7 - (stepIdx / 8)
		col := stepIdx % 8

		var color [3]uint8 = offColor
		var channel uint8 = midi.ChannelStatic

		if stepIdx >= track.Length {
			color = offColor
		} else if stepIdx == trackStep {
			color = playheadColor
			channel = midi.ChannelPulse
		} else if track.Steps[stepIdx].Active {
			color = stepsColor
		} else {
			color = stepsEmpty
		}

		leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: channel})
	}

	// Bottom-left 4x4: track select
	for t := 0; t < 16; t++ {
		row := t / 4
		col := t % 4

		hasContent := false
		for step := 0; step < pat.Tracks[t].Length; step++ {
			if pat.Tracks[t].Steps[step].Active {
				hasContent = true
				break
			}
		}

		var color [3]uint8
		if t == s.Selected {
			color = trackSelected
		} else if hasContent {
			color = trackHasContent
		} else {
			color = trackEmpty
		}

		leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
	}

	// Bottom-right 4x4: commands
	previewActive := [3]uint8{0, 255, 0}   // green when on
	recordActive := [3]uint8{255, 0, 0}    // red when on
	for row := 0; row < 4; row++ {
		for col := 4; col < 8; col++ {
			color := commandsColor
			// Preview button (row 3, col 4)
			if row == 3 && col == 4 && s.Preview {
				color = previewActive
			}
			// Record button (row 3, col 5)
			if row == 3 && col == 5 && s.Recording {
				color = recordActive
			}
			leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
		}
	}

	return leds
}

// IsInputMode returns true if in confirm mode
func (d *DrumDevice) IsInputMode() bool {
	return d.confirmMode
}

func (d *DrumDevice) HandleKey(key string) {
	// Confirmation mode
	if d.confirmMode {
		switch key {
		case "y", "Y":
			if d.confirmAction != nil {
				d.confirmAction()
			}
			d.confirmMode = false
			d.confirmAction = nil
		case "n", "N", "esc", "q":
			d.confirmMode = false
			d.confirmAction = nil
		}
		return
	}

	s := d.state
	pat := &s.Patterns[s.Editing]
	track := &pat.Tracks[s.Selected]

	switch key {
	case "h", "left":
		if s.Cursor > 0 {
			s.Cursor--
		}
	case "l", "right":
		if s.Cursor < track.Length-1 {
			s.Cursor++
		}
	case " ":
		if s.Cursor < track.Length {
			track.Steps[s.Cursor].Active = !track.Steps[s.Cursor].Active
		}
	case "j", "down":
		if s.Selected < 15 {
			s.Selected++
			if s.Cursor >= s.Patterns[s.Editing].Tracks[s.Selected].Length {
				s.Cursor = s.Patterns[s.Editing].Tracks[s.Selected].Length - 1
			}
		}
	case "k", "up":
		if s.Selected > 0 {
			s.Selected--
			if s.Cursor >= s.Patterns[s.Editing].Tracks[s.Selected].Length {
				s.Cursor = s.Patterns[s.Editing].Tracks[s.Selected].Length - 1
			}
		}
	case "[":
		if track.Length > 1 {
			track.Length--
			if s.Cursor >= track.Length {
				s.Cursor = track.Length - 1
			}
		}
	case "]":
		if track.Length < 32 {
			track.Length++
		}
	case "c":
		d.confirmClearTrack()
	case "C":
		d.confirmClearPattern()
	case "<", ",":
		if s.Editing > 0 {
			s.Editing--
		}
	case ">", ".":
		if s.Editing < NumPatterns-1 {
			s.Editing++
		}
	}
}

func (d *DrumDevice) confirmClearTrack() {
	s := d.state
	pat := &s.Patterns[s.Editing]
	track := &pat.Tracks[s.Selected]

	// Check if track has any content
	hasContent := false
	for i := 0; i < track.Length; i++ {
		if track.Steps[i].Active {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return // nothing to clear
	}

	d.confirmMsg = fmt.Sprintf("Clear track %d?", s.Selected+1)
	d.confirmAction = func() {
		for step := 0; step < 32; step++ {
			track.Steps[step].Active = false
		}
	}
	d.confirmMode = true
}

func (d *DrumDevice) confirmClearPattern() {
	s := d.state
	pat := &s.Patterns[s.Editing]

	// Check if pattern has any content
	hasContent := false
	for t := 0; t < 16 && !hasContent; t++ {
		for step := 0; step < pat.Tracks[t].Length; step++ {
			if pat.Tracks[t].Steps[step].Active {
				hasContent = true
				break
			}
		}
	}
	if !hasContent {
		return // nothing to clear
	}

	d.confirmMsg = fmt.Sprintf("Clear pattern %d?", s.Editing+1)
	d.confirmAction = func() {
		for t := 0; t < 16; t++ {
			for step := 0; step < 32; step++ {
				pat.Tracks[t].Steps[step].Active = false
			}
		}
	}
	d.confirmMode = true
}

func (d *DrumDevice) HandlePad(row, col int) {
	s := d.state
	pat := &s.Patterns[s.Editing]

	// Top 4 rows: step toggle
	if row >= 4 && row <= 7 {
		stepIdx := (7-row)*8 + col
		track := &pat.Tracks[s.Selected]
		if stepIdx < track.Length {
			track.Steps[stepIdx].Active = !track.Steps[stepIdx].Active
			s.Cursor = stepIdx
		}
		return
	}

	// Bottom-left 4x4: track select (and record/preview)
	if row < 4 && col < 4 {
		trackIdx := row*4 + col
		if trackIdx < 16 {
			// Always select the track
			s.Selected = trackIdx
			if s.Cursor >= pat.Tracks[s.Selected].Length {
				s.Cursor = pat.Tracks[s.Selected].Length - 1
			}

			// If preview mode, play the sound
			if s.Preview {
				select {
				case d.previewChan <- trackIdx:
				default:
				}
			}

			// If recording while playing, write a step at current position
			if s.Recording && S.Playing {
				track := &pat.Tracks[trackIdx]
				stepIdx := s.Step % track.Length
				track.Steps[stepIdx].Active = true
				track.Steps[stepIdx].Velocity = 100 // default velocity
			}
		}
		return
	}

	// Bottom-right 4x4: command pads
	if row < 4 && col >= 4 {
		track := &pat.Tracks[s.Selected]
		switch {
		// Row 0: Clear Track, Clear Pattern, Copy, Paste
		case row == 0 && col == 4: // Clear Track
			d.confirmClearTrack()
		case row == 0 && col == 5: // Clear Pattern
			d.confirmClearPattern()
		// Row 1: Nudge Left, Nudge Right, Length -, Length +
		case row == 1 && col == 6: // Length -
			if track.Length > 1 {
				track.Length--
				if s.Cursor >= track.Length {
					s.Cursor = track.Length - 1
				}
			}
		case row == 1 && col == 7: // Length +
			if track.Length < 32 {
				track.Length++
			}
		// Row 3: Preview, Record, Mute, Solo
		case row == 3 && col == 4: // Preview toggle
			s.Preview = !s.Preview
		case row == 3 && col == 5: // Record toggle
			s.Recording = !s.Recording
		}
		return
	}
}

func (d *DrumDevice) renderLaunchpadHelp() string {
	// Colors
	topRowColor := [3]uint8{111, 10, 126}
	stepsColor := [3]uint8{234, 73, 116}
	trackColor := [3]uint8{148, 18, 126}
	commandsColor := [3]uint8{253, 157, 110}
	sceneColor := [3]uint8{71, 13, 121}

	// Build the grid
	var grid [8][8][3]uint8
	var rightCol [8][3]uint8

	// Top 4 rows: steps
	for row := 4; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = stepsColor
		}
	}

	// Bottom-left 4x4: track select
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			grid[row][col] = trackColor
		}
	}

	// Bottom-right 4x4: commands
	for row := 0; row < 4; row++ {
		for col := 4; col < 8; col++ {
			grid[row][col] = commandsColor
		}
	}

	// Right column: scenes
	for i := 0; i < 8; i++ {
		rightCol[i] = sceneColor
	}

	// Top row
	topRow := make([][3]uint8, 8)
	for i := range topRow {
		topRow[i] = topRowColor
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, &rightCol) + "\n\n"

	// Legend
	out += widgets.RenderLegendItem(stepsColor, "Steps", "tap to toggle steps 1-32") + "\n"
	out += widgets.RenderLegendItem(trackColor, "Track", "select track 1-16 (plays sound in preview mode)") + "\n"
	out += widgets.RenderLegendItem(commandsColor, "Commands", "") + "\n"
	out += `    Row 3: [Preview] [Record]  (Mute)   (Solo)
    Row 2: (Vel -)   (Vel +)   (-)      (-)
    Row 1: (Nudge<)  (Nudge>)  [Len -]  [Len +]
    Row 0: [ClrTrk]  [ClrPat]  (Copy)   (Paste)
    [ ] = implemented, ( ) = not yet` + "\n"
	out += widgets.RenderLegendItem(sceneColor, "Scene", "launch scenes")

	return out
}
