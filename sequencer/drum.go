package sequencer

import (
	"fmt"

	"go-sequence/midi"
	"go-sequence/widgets"
)

type DrumStep struct {
	Active   bool
	Velocity uint8
	Nudge    int8 // -64 to +63 timing offset
}

type DrumTrack struct {
	Steps  [32]DrumStep
	Length int   // 1-32, defaults to 16
	Note   uint8 // MIDI note for this sound
}

type DrumPattern struct {
	Tracks [16]DrumTrack
}

func (p *DrumPattern) MasterLength() int {
	max := 1
	for i := 0; i < 16; i++ {
		if p.Tracks[i].Length > max {
			max = p.Tracks[i].Length
		}
	}
	return max
}

type DrumDevice struct {
	patterns []*DrumPattern
	pattern  int // currently playing
	next     int // queued (defaults to pattern)
	step     int // global step counter
	selected int // which track (0-15) we're editing
	editing  int // which pattern we're editing (independent of playing)

	// UI state
	cursor int // step cursor for keyboard nav
}

func NewDrumDevice() *DrumDevice {
	d := &DrumDevice{
		patterns: make([]*DrumPattern, NumPatterns),
		pattern:  0,
		next:     0,
		step:     0,
		selected: 0,
		editing:  0,
		cursor:   0,
	}

	// Initialize patterns with default GM drum notes
	gmNotes := []uint8{
		36, // Kick
		38, // Snare
		42, // Closed HH
		46, // Open HH
		41, // Low Tom
		43, // Mid Tom
		45, // High Tom
		49, // Crash
		51, // Ride
		39, // Clap
		56, // Cowbell
		75, // Clave
		54, // Tambourine
		69, // Cabasa
		70, // Maracas
		37, // Rimshot
	}

	for i := range d.patterns {
		d.patterns[i] = &DrumPattern{}
		for t := 0; t < 16; t++ {
			d.patterns[i].Tracks[t] = DrumTrack{
				Length: 16,
				Note:   gmNotes[t],
			}
			// Init steps with default velocity
			for s := 0; s < 32; s++ {
				d.patterns[i].Tracks[t].Steps[s] = DrumStep{
					Active:   false,
					Velocity: 100,
					Nudge:    0,
				}
			}
		}
	}

	return d
}

// Device interface implementation

func (d *DrumDevice) Tick(step int) []midi.Event {
	pat := d.patterns[d.pattern]
	masterLen := pat.MasterLength()

	// Pattern switch at master length boundary
	if d.step%masterLen == 0 {
		d.pattern = d.next
		pat = d.patterns[d.pattern] // refresh in case it changed
	}

	var events []midi.Event
	for i := 0; i < 16; i++ {
		track := &pat.Tracks[i]
		// Each track loops at its own length (polymeters!)
		trackStep := d.step % track.Length
		s := track.Steps[trackStep]
		if s.Active {
			events = append(events, midi.Event{
				Type:     midi.NoteOn,
				Note:     track.Note,
				Velocity: s.Velocity,
			})
		}
	}

	// Keep counting - don't reset at masterLen (breaks polymeters)
	// Wrap at large value to prevent overflow
	d.step = (d.step + 1) % 65536
	return events
}

func (d *DrumDevice) QueuePattern(p int) (pattern, next int) {
	if p >= 0 && p < len(d.patterns) {
		d.next = p
	}
	return d.pattern, d.next
}

func (d *DrumDevice) GetState() (pattern, next int) {
	return d.pattern, d.next
}

func (d *DrumDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i, pat := range d.patterns {
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

func (d *DrumDevice) View() string {
	pat := d.patterns[d.editing]

	// Header - show editing vs playing
	playInfo := ""
	if d.editing != d.pattern {
		playInfo = fmt.Sprintf(" (playing:%d)", d.pattern)
	}
	selectedTrack := &pat.Tracks[d.selected]
	selectedStep := d.step % selectedTrack.Length
	out := fmt.Sprintf("DRUM  Pattern %d%s  Step %d/%d  Track %d\n\n", d.editing+1, playInfo, selectedStep+1, selectedTrack.Length, d.selected+1)

	// 16x32 grid - single char per cell
	for t := 0; t < 16; t++ {
		track := &pat.Tracks[t]
		trackStep := d.step % track.Length // each track has its own playhead
		out += fmt.Sprintf("%2d ", t+1)

		for s := 0; s < 32; s++ {
			isCursor := t == d.selected && s == d.cursor

			var char string
			if s >= track.Length {
				if isCursor {
					char = "□" // cursor beyond
				} else {
					char = "-" // beyond
				}
			} else if s == trackStep {
				if isCursor {
					char = "▷" // cursor on playhead
				} else {
					char = "▶" // playhead
				}
			} else if track.Steps[s].Active {
				if isCursor {
					char = "◉" // cursor on active
				} else {
					char = "●" // active
				}
			} else {
				if isCursor {
					char = "○" // cursor on empty
				} else {
					char = "·" // empty
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
	out += widgets.RenderLaunchpad(d.HelpLayout())
	out += "\n"
	out += widgets.RenderLegend([]widgets.Zone{
		{Name: "Steps", Color: [3]uint8{234, 73, 116}, Desc: "tap to toggle steps 1-32"},
		{Name: "Track", Color: [3]uint8{148, 18, 126}, Desc: "tap to select track 1-16"},
		{Name: "Commands", Color: [3]uint8{253, 157, 110}, Desc: "clear, nudge, length, velocity"},
		{Name: "Scene", Color: [3]uint8{71, 13, 121}, Desc: "launch scenes"},
	})

	return out
}

func (d *DrumDevice) RenderLEDs() []LEDState {
	var leds []LEDState
	pat := d.patterns[d.editing]
	track := &pat.Tracks[d.selected]

	// Colors - TODO: move to theme
	stepsColor := [3]uint8{234, 73, 116}        // pink - active step
	stepsEmpty := [3]uint8{80, 30, 50}          // dim pink - empty but in bounds
	trackHasContent := [3]uint8{148, 18, 126}   // purple - track has notes
	trackEmpty := [3]uint8{40, 10, 30}          // dim - empty track
	trackSelected := [3]uint8{255, 255, 255}    // white - selected track
	commandsColor := [3]uint8{253, 157, 110}    // orange
	playheadColor := [3]uint8{255, 255, 255}    // white
	offColor := [3]uint8{0, 0, 0}               // black - beyond track length

	// Top 4 rows (rows 4-7): steps for selected track
	trackStep := d.step % track.Length // polymeter: each track has its own position
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

	// Bottom-left 4x4 (rows 0-3, cols 0-3): track select
	for t := 0; t < 16; t++ {
		row := t / 4
		col := t % 4

		hasContent := false
		for s := 0; s < pat.Tracks[t].Length; s++ {
			if pat.Tracks[t].Steps[s].Active {
				hasContent = true
				break
			}
		}

		var color [3]uint8
		if t == d.selected {
			color = trackSelected // white
		} else if hasContent {
			color = trackHasContent // purple
		} else {
			color = trackEmpty // dim
		}

		leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
	}

	// Bottom-right 4x4: commands
	for row := 0; row < 4; row++ {
		for col := 4; col < 8; col++ {
			leds = append(leds, LEDState{Row: row, Col: col, Color: commandsColor, Channel: midi.ChannelStatic})
		}
	}

	return leds
}

func (d *DrumDevice) HandleKey(key string) {
	pat := d.patterns[d.editing]
	track := &pat.Tracks[d.selected]

	switch key {
	case "h", "left":
		if d.cursor > 0 {
			d.cursor--
		}
	case "l", "right":
		if d.cursor < track.Length-1 {
			d.cursor++
		}
	case " ":
		if d.cursor < track.Length {
			track.Steps[d.cursor].Active = !track.Steps[d.cursor].Active
		}
	case "j", "down":
		if d.selected < 15 {
			d.selected++
			if d.cursor >= d.patterns[d.editing].Tracks[d.selected].Length {
				d.cursor = d.patterns[d.editing].Tracks[d.selected].Length - 1
			}
		}
	case "k", "up":
		if d.selected > 0 {
			d.selected--
			if d.cursor >= d.patterns[d.editing].Tracks[d.selected].Length {
				d.cursor = d.patterns[d.editing].Tracks[d.selected].Length - 1
			}
		}
	case "[":
		if track.Length > 1 {
			track.Length--
			if d.cursor >= track.Length {
				d.cursor = track.Length - 1
			}
		}
	case "]":
		if track.Length < 32 {
			track.Length++
		}
	case "c":
		for s := 0; s < 32; s++ {
			track.Steps[s].Active = false
		}
	case "<", ",":
		if d.editing > 0 {
			d.editing--
		}
	case ">", ".":
		if d.editing < NumPatterns-1 {
			d.editing++
		}
	}
}

func (d *DrumDevice) HandlePad(row, col int) {
	pat := d.patterns[d.editing]

	// Top 4 rows (rows 4-7): step toggle
	if row >= 4 && row <= 7 {
		stepIdx := (7-row)*8 + col
		track := &pat.Tracks[d.selected]
		if stepIdx < track.Length {
			track.Steps[stepIdx].Active = !track.Steps[stepIdx].Active
			d.cursor = stepIdx
		}
		return
	}

	// Bottom-left 4x4 (rows 0-3, cols 0-3): track select
	if row < 4 && col < 4 {
		trackIdx := row*4 + col
		if trackIdx < 16 {
			d.selected = trackIdx
			if d.cursor >= pat.Tracks[d.selected].Length {
				d.cursor = pat.Tracks[d.selected].Length - 1
			}
		}
		return
	}

	// Bottom-right 4x4: commands (TODO)
}

func (d *DrumDevice) HelpLayout() widgets.LaunchpadLayout {
	topRowColor := [3]uint8{111, 10, 126}
	stepsColor := [3]uint8{234, 73, 116}
	trackSelectColor := [3]uint8{148, 18, 126}
	commandsColor := [3]uint8{253, 157, 110}
	sceneColor := [3]uint8{71, 13, 121}

	var layout widgets.LaunchpadLayout

	for i := 0; i < 8; i++ {
		layout.TopRow[i] = widgets.PadConfig{Color: topRowColor, Tooltip: "Mode"}
	}

	for row := 4; row < 8; row++ {
		for col := 0; col < 8; col++ {
			layout.Grid[row][col] = widgets.PadConfig{Color: stepsColor, Tooltip: "Steps"}
		}
	}

	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			layout.Grid[row][col] = widgets.PadConfig{Color: trackSelectColor, Tooltip: "Track Select"}
		}
	}

	layout.Grid[0][4] = widgets.PadConfig{Color: commandsColor, Tooltip: "Clear Track"}
	layout.Grid[0][5] = widgets.PadConfig{Color: commandsColor, Tooltip: "Clear Pattern"}
	layout.Grid[0][6] = widgets.PadConfig{Color: commandsColor, Tooltip: "Copy"}
	layout.Grid[0][7] = widgets.PadConfig{Color: commandsColor, Tooltip: "Paste"}
	layout.Grid[1][4] = widgets.PadConfig{Color: commandsColor, Tooltip: "Nudge Left"}
	layout.Grid[1][5] = widgets.PadConfig{Color: commandsColor, Tooltip: "Nudge Right"}
	layout.Grid[1][6] = widgets.PadConfig{Color: commandsColor, Tooltip: "Length -"}
	layout.Grid[1][7] = widgets.PadConfig{Color: commandsColor, Tooltip: "Length +"}
	layout.Grid[2][4] = widgets.PadConfig{Color: commandsColor, Tooltip: "Velocity -"}
	layout.Grid[2][5] = widgets.PadConfig{Color: commandsColor, Tooltip: "Velocity +"}
	layout.Grid[2][6] = widgets.PadConfig{Color: commandsColor, Tooltip: "Swing -"}
	layout.Grid[2][7] = widgets.PadConfig{Color: commandsColor, Tooltip: "Swing +"}
	layout.Grid[3][4] = widgets.PadConfig{Color: commandsColor, Tooltip: "Undo"}
	layout.Grid[3][5] = widgets.PadConfig{Color: commandsColor, Tooltip: "Redo"}
	layout.Grid[3][6] = widgets.PadConfig{Color: commandsColor, Tooltip: "Mute"}
	layout.Grid[3][7] = widgets.PadConfig{Color: commandsColor, Tooltip: "Solo"}

	for i := 0; i < 8; i++ {
		layout.RightCol[i] = widgets.PadConfig{Color: sceneColor, Tooltip: "Scene"}
	}

	return layout
}
