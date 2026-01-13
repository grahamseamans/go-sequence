package sequencer

import (
	"fmt"
	"sort"

	"go-sequence/midi"
	"go-sequence/widgets"
)

type NoteEvent struct {
	Start    float64 // position in beats (0.0 = start, 0.25 = 16th note)
	Duration float64 // length in beats
	Pitch    uint8   // MIDI note 0-127
	Velocity uint8   // 0-127
}

type PianoPattern struct {
	Notes  []NoteEvent
	Length float64 // pattern length in beats (e.g., 4.0 = 1 bar)
}

// View scales: beats per column (8 levels)
var ViewScales = []float64{
	0.03125, // 1/32 per col - super zoomed
	0.0625,  // 1/16 per col
	0.125,   // 1/8 per col
	0.25,    // 1/4 per col
	0.5,     // 1/2 per col
	1.0,     // 1 beat per col
	2.0,     // 2 beats per col
	4.0,     // 4 beats per col - zoomed out
}

// Edit sensitivity: movement amounts
var EditHorizSteps = []float64{
	0.015625, // 1/64
	0.03125,  // 1/32
	0.0625,   // 1/16
	0.125,    // 1/8
	0.25,     // 1/4
	0.5,      // 1/2
	1.0,      // 1 beat
}

var EditVertSteps = []int{1, 12} // semitone, octave

// Vertical view modes
const (
	ViewSmushed = 24 // 2 octaves visible
	ViewSpread  = 12 // 1 octave visible
)

type PianoRollDevice struct {
	patterns []*PianoPattern
	pattern  int     // currently playing
	next     int     // queued
	editing  int     // which pattern we're editing
	step     int     // playback step (internal)
	lastBeat float64 // for note triggering

	// Viewport
	centerBeat  float64 // where view is centered (auto-follows selection)
	centerPitch float64 // center pitch
	viewScale   int     // index into ViewScales
	viewRows    int     // ViewSmushed or ViewSpread

	// Edit sensitivity
	editHoriz int // index into EditHorizSteps
	editVert  int // index into EditVertSteps

	// Selection
	selectedNote int // -1 = none, else index into Notes

	// Held notes for note-off tracking
	heldNotes map[uint8]bool
}

func NewPianoRollDevice() *PianoRollDevice {
	p := &PianoRollDevice{
		patterns:     make([]*PianoPattern, NumPatterns),
		pattern:      0,
		next:         0,
		editing:      0,
		step:         0,
		centerBeat:   2.0,  // start centered at beat 2
		centerPitch:  60.0, // middle C
		viewScale:    2,    // 1/8 per col default
		viewRows:     ViewSpread,
		editHoriz:    2,    // 1/16 default
		editVert:     0,    // semitone default
		selectedNote: -1,
		heldNotes:    make(map[uint8]bool),
	}

	for i := range p.patterns {
		p.patterns[i] = &PianoPattern{
			Notes:  []NoteEvent{},
			Length: 4.0, // 1 bar default
		}
	}

	return p
}

func (p *PianoRollDevice) Tick(step int) []midi.Event {
	if p.step == 0 {
		p.pattern = p.next
	}

	pat := p.patterns[p.pattern]
	beat := float64(p.step) / 4.0 // 16th notes to beats
	nextBeat := float64(p.step+1) / 4.0

	var events []midi.Event

	// Note-offs for notes that should end
	for pitch := range p.heldNotes {
		for _, note := range pat.Notes {
			if note.Pitch == pitch {
				endBeat := note.Start + note.Duration
				if endBeat > p.lastBeat && endBeat <= beat {
					events = append(events, midi.Event{
						Type: midi.NoteOff,
						Note: pitch,
					})
					delete(p.heldNotes, pitch)
				}
			}
		}
	}

	// Note-ons for notes that start this tick
	for _, note := range pat.Notes {
		if note.Start >= beat && note.Start < nextBeat {
			events = append(events, midi.Event{
				Type:     midi.NoteOn,
				Note:     note.Pitch,
				Velocity: note.Velocity,
			})
			p.heldNotes[note.Pitch] = true
		}
	}

	p.lastBeat = beat
	p.step = (p.step + 1) % int(pat.Length*4)

	return events
}

func (p *PianoRollDevice) QueuePattern(pat int) (pattern, next int) {
	if pat >= 0 && pat < len(p.patterns) {
		p.next = pat
	}
	return p.pattern, p.next
}

func (p *PianoRollDevice) GetState() (pattern, next int) {
	return p.pattern, p.next
}

func (p *PianoRollDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i, pat := range p.patterns {
		if len(pat.Notes) > 0 {
			mask[i] = true
		}
	}
	return mask
}

func (p *PianoRollDevice) HandleMIDI(event midi.Event) {
	// TODO: record from MIDI keyboard
}

// formatStep formats a beat step value as a fraction
func formatStep(step float64) string {
	switch step {
	case 0.015625:
		return "1/64"
	case 0.03125:
		return "1/32"
	case 0.0625:
		return "1/16"
	case 0.125:
		return "1/8"
	case 0.25:
		return "1/4"
	case 0.5:
		return "1/2"
	case 1.0:
		return "1"
	case 2.0:
		return "2"
	case 4.0:
		return "4"
	default:
		return fmt.Sprintf("%.3f", step)
	}
}

func (p *PianoRollDevice) View() string {
	pat := p.patterns[p.editing]

	playInfo := ""
	if p.editing != p.pattern {
		playInfo = fmt.Sprintf(" (playing:%d)", p.pattern)
	}

	// Current settings
	viewScale := ViewScales[p.viewScale]
	editH := EditHorizSteps[p.editHoriz]
	editV := EditVertSteps[p.editVert]
	vertMode := "spread"
	if p.viewRows == ViewSmushed {
		vertMode = "smushed"
	}

	beat := float64(p.step) / 4.0
	out := fmt.Sprintf("PIANO  Pattern %d%s  Beat %.1f/%g\n", p.editing+1, playInfo, beat, pat.Length)
	out += fmt.Sprintf("View: %s/col %s  Edit: %s horiz, %d semi vert\n\n", formatStep(viewScale), vertMode, formatStep(editH), editV)

	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

	// Viewport dimensions
	cols := 48
	rows := p.viewRows

	// Calculate view bounds from center
	beatsPerCol := viewScale
	totalBeats := float64(cols) * beatsPerCol
	startBeat := p.centerBeat - totalBeats/2
	startPitch := int(p.centerPitch) + rows/2

	// Calculate playhead column (avoid floating point comparison issues)
	playheadCol := -1
	if p.editing == p.pattern && beat >= startBeat {
		playheadCol = int((beat - startBeat) / beatsPerCol)
	}

	// Render grid
	for row := 0; row < rows; row++ {
		pitch := uint8(startPitch - row)
		if pitch > 127 {
			continue
		}
		noteName := noteNames[pitch%12]
		octNum := pitch / 12
		out += fmt.Sprintf("%2s%d ", noteName, octNum)

		for col := 0; col < cols; col++ {
			colBeat := startBeat + float64(col)*beatsPerCol
			colBeatEnd := colBeat + beatsPerCol

			// Beyond pattern bounds?
			if colBeat < 0 || colBeat >= pat.Length {
				out += "-"
				continue
			}

			// Playhead check
			isPlayhead := col == playheadCol

			// Find all notes at this cell
			var notesHere []int
			var noteStartHere int = -1
			for i := range pat.Notes {
				n := &pat.Notes[i]
				if n.Pitch == pitch {
					noteEnd := n.Start + n.Duration
					// Note overlaps this cell?
					if n.Start < colBeatEnd && noteEnd > colBeat {
						notesHere = append(notesHere, i)
						// Note starts in this cell?
						if n.Start >= colBeat && n.Start < colBeatEnd {
							noteStartHere = i
						}
					}
				}
			}

			var char string
			if len(notesHere) > 0 {
				hasOverlap := len(notesHere) > 1

				if noteStartHere >= 0 {
					if noteStartHere == p.selectedNote {
						char = "◉"
					} else if isPlayhead {
						char = "▶"
					} else {
						char = "●"
					}
				} else {
					if hasOverlap {
						char = "═"
					} else {
						char = "─"
					}
				}
			} else {
				if isPlayhead {
					char = "▶"
				} else {
					char = "·"
				}
			}
			out += char
		}
		out += "\n"
	}

	// Selection info
	if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
		n := &pat.Notes[p.selectedNote]
		noteName := noteNames[n.Pitch%12]
		octNum := n.Pitch / 12
		out += fmt.Sprintf("\nSelected: %s%d  start:%.2f  dur:%.2f  vel:%d", noteName, octNum, n.Start, n.Duration, n.Velocity)
	}

	// Key help
	out += "\n\n"
	out += widgets.RenderKeyHelp([]widgets.KeySection{
		{Title: "Select", Keys: []widgets.KeyBinding{
			{Key: "hjkl", Desc: "select notes"},
		}},
		{Title: "Move", Keys: []widgets.KeyBinding{
			{Key: "yuio", Desc: "move note"},
			{Key: "n / m", Desc: "shorter / longer"},
		}},
		{Title: "Notes", Keys: []widgets.KeyBinding{
			{Key: "space", Desc: "add note"},
			{Key: "x", Desc: "delete note"},
		}},
		{Title: "View", Keys: []widgets.KeyBinding{
			{Key: "q / w", Desc: "zoom out/in"},
			{Key: "a / s", Desc: "smushed/spread"},
		}},
		{Title: "Grid", Keys: []widgets.KeyBinding{
			{Key: "d / f", Desc: "horiz coarse/fine"},
			{Key: "e / r", Desc: "vert coarse/fine"},
		}},
		{Title: "Pattern", Keys: []widgets.KeyBinding{
			{Key: "< / >", Desc: "prev/next pattern"},
			{Key: "[ / ]", Desc: "length -/+"},
			{Key: "c", Desc: "clear"},
		}},
	})

	// Launchpad
	out += "\n\n"
	out += widgets.RenderLaunchpad(p.HelpLayout())
	out += "\n"
	out += widgets.RenderLegend([]widgets.Zone{
		{Name: "Notes", Color: [3]uint8{80, 200, 255}, Desc: "tap to add/select notes"},
		{Name: "Scene", Color: [3]uint8{148, 18, 126}, Desc: "launch scenes"},
	})

	return out
}

func (p *PianoRollDevice) RenderLEDs() []LEDState {
	var leds []LEDState
	pat := p.patterns[p.editing]

	// Colors
	noteColor := [3]uint8{80, 200, 255}
	selectedColor := [3]uint8{255, 100, 200}
	dimColor := [3]uint8{20, 50, 70}
	playheadColor := [3]uint8{255, 255, 255}
	offColor := [3]uint8{0, 0, 0}

	// Use viewport center for Launchpad display
	basePitch := int(p.centerPitch) - 4
	viewScale := ViewScales[p.viewScale]
	startBeat := p.centerBeat - 4*viewScale
	beat := float64(p.step) / 4.0

	// Calculate playhead column
	playheadCol := -1
	if p.editing == p.pattern && beat >= startBeat {
		playheadCol = int((beat - startBeat) / viewScale)
	}

	for row := range 8 {
		pitch := uint8(basePitch + row)
		if pitch > 127 {
			continue
		}

		for col := range 8 {
			colBeat := startBeat + float64(col)*viewScale
			colBeatEnd := colBeat + viewScale
			isPlayhead := col == playheadCol

			var color [3]uint8 = dimColor
			channel := midi.ChannelStatic

			if colBeat < 0 || colBeat >= pat.Length {
				color = offColor
			} else {
				for i, n := range pat.Notes {
					if n.Pitch == pitch {
						noteEnd := n.Start + n.Duration
						if n.Start < colBeatEnd && noteEnd > colBeat {
							if i == p.selectedNote {
								color = selectedColor
							} else {
								color = noteColor
							}
							break
						}
					}
				}

				if isPlayhead {
					color = playheadColor
					channel = midi.ChannelPulse
				}
			}

			leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: channel})
		}
	}

	return leds
}

// centerOnSelection moves the viewport to center on the selected note
func (p *PianoRollDevice) centerOnSelection() {
	pat := p.patterns[p.editing]
	if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
		n := &pat.Notes[p.selectedNote]
		p.centerBeat = n.Start
		p.centerPitch = float64(n.Pitch)
	}
}

// selectNoteByTime finds the next/prev note sorted by time
func (p *PianoRollDevice) selectNoteByTime(direction int) {
	pat := p.patterns[p.editing]
	if len(pat.Notes) == 0 {
		return
	}
	p.selectedNote += direction
	if p.selectedNote < 0 {
		p.selectedNote = len(pat.Notes) - 1
	} else if p.selectedNote >= len(pat.Notes) {
		p.selectedNote = 0
	}
	p.centerOnSelection()
}

// selectNoteByPitch finds a note at higher/lower pitch near current selection
func (p *PianoRollDevice) selectNoteByPitch(direction int) {
	pat := p.patterns[p.editing]
	if len(pat.Notes) == 0 {
		return
	}

	// If nothing selected, select first note
	if p.selectedNote < 0 || p.selectedNote >= len(pat.Notes) {
		p.selectedNote = 0
		p.centerOnSelection()
		return
	}

	currentNote := pat.Notes[p.selectedNote]
	targetPitch := int(currentNote.Pitch) + direction

	// Find note closest to current time at target pitch (or nearby)
	bestIdx := -1
	bestDist := 1000.0
	for searchPitch := targetPitch; searchPitch >= 0 && searchPitch <= 127; searchPitch += direction {
		for i, n := range pat.Notes {
			if int(n.Pitch) == searchPitch {
				dist := abs(n.Start - currentNote.Start)
				if dist < bestDist {
					bestDist = dist
					bestIdx = i
				}
			}
		}
		if bestIdx >= 0 {
			break // Found a note at this pitch level
		}
	}

	if bestIdx >= 0 {
		p.selectedNote = bestIdx
		p.centerOnSelection()
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func (p *PianoRollDevice) HandleKey(key string) {
	pat := p.patterns[p.editing]
	editH := EditHorizSteps[p.editHoriz]
	editV := EditVertSteps[p.editVert]

	switch key {
	// Note selection (hjkl)
	case "h", "left":
		p.selectNoteByTime(-1)
	case "l", "right":
		p.selectNoteByTime(1)
	case "j", "down":
		p.selectNoteByPitch(-1)
	case "k", "up":
		p.selectNoteByPitch(1)

	// Move selected note (yuio - same layout as hjkl, one row up)
	case "y":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			n := &pat.Notes[p.selectedNote]
			if n.Start >= editH {
				n.Start -= editH
			} else {
				n.Start = 0
			}
			p.centerOnSelection()
		}
	case "o":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			n := &pat.Notes[p.selectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				n.Start += editH
			}
			p.centerOnSelection()
		}
	case "u":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			n := &pat.Notes[p.selectedNote]
			if int(n.Pitch) >= editV {
				n.Pitch -= uint8(editV)
			}
			p.centerOnSelection()
		}
	case "i":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			n := &pat.Notes[p.selectedNote]
			if int(n.Pitch)+editV <= 127 {
				n.Pitch += uint8(editV)
			}
			p.centerOnSelection()
		}

	// Note length (n/m)
	case "n":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			n := &pat.Notes[p.selectedNote]
			if n.Duration > editH {
				n.Duration -= editH
			}
		}
	case "m":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			n := &pat.Notes[p.selectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				n.Duration += editH
			}
		}

	// View zoom
	case "q":
		if p.viewScale < len(ViewScales)-1 {
			p.viewScale++
		}
	case "w":
		if p.viewScale > 0 {
			p.viewScale--
		}
	case "a":
		p.viewRows = ViewSmushed
	case "s":
		p.viewRows = ViewSpread

	// Edit sensitivity
	case "d":
		if p.editHoriz < len(EditHorizSteps)-1 {
			p.editHoriz++
		}
	case "f":
		if p.editHoriz > 0 {
			p.editHoriz--
		}
	case "e":
		if p.editVert < len(EditVertSteps)-1 {
			p.editVert++
		}
	case "r":
		if p.editVert > 0 {
			p.editVert--
		}

	// Add/delete notes
	case " ":
		// New note at view center
		newNote := NoteEvent{
			Start:    p.centerBeat,
			Duration: EditHorizSteps[p.editHoriz] * 4,
			Pitch:    uint8(p.centerPitch),
			Velocity: 100,
		}
		if newNote.Duration < 0.25 {
			newNote.Duration = 0.25
		}
		pat.Notes = append(pat.Notes, newNote)
		p.selectedNote = len(pat.Notes) - 1
		p.centerOnSelection()

	case "x":
		if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
			pat.Notes = append(pat.Notes[:p.selectedNote], pat.Notes[p.selectedNote+1:]...)
			if p.selectedNote >= len(pat.Notes) {
				p.selectedNote = len(pat.Notes) - 1
			}
			if p.selectedNote >= 0 {
				p.centerOnSelection()
			}
		}

	// Pattern length
	case "[":
		if pat.Length > 1.0 {
			pat.Length -= 1.0
		}
	case "]":
		if pat.Length < 64.0 {
			pat.Length += 1.0
		}

	// Clear
	case "c":
		pat.Notes = []NoteEvent{}
		p.selectedNote = -1

	// Pattern selection
	case "<":
		if p.editing > 0 {
			p.editing--
			p.selectedNote = -1
		}
	case ">", ".":
		if p.editing < NumPatterns-1 {
			p.editing++
			p.selectedNote = -1
		}
	}

	// Keep notes sorted by start time, preserving selection
	var selectedNote *NoteEvent
	if p.selectedNote >= 0 && p.selectedNote < len(pat.Notes) {
		n := pat.Notes[p.selectedNote]
		selectedNote = &n
	}

	sort.Slice(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})

	if selectedNote != nil {
		for i, n := range pat.Notes {
			if n.Start == selectedNote.Start && n.Pitch == selectedNote.Pitch {
				p.selectedNote = i
				break
			}
		}
	}
}

func (p *PianoRollDevice) HandlePad(row, col int) {
	pat := p.patterns[p.editing]

	// Map pad to viewport coordinates
	basePitch := int(p.centerPitch) - 4
	viewScale := ViewScales[p.viewScale]
	startBeat := p.centerBeat - 4*viewScale

	pitch := uint8(basePitch + row)
	beat := startBeat + float64(col)*viewScale
	beatEnd := beat + viewScale

	if beat < 0 || beat >= pat.Length || pitch > 127 {
		return
	}

	// Check if note exists here
	for i, n := range pat.Notes {
		if n.Pitch == pitch {
			noteEnd := n.Start + n.Duration
			if n.Start < beatEnd && noteEnd > beat {
				p.selectedNote = i
				p.centerOnSelection()
				return
			}
		}
	}

	// No note - add one
	newNote := NoteEvent{
		Start:    beat,
		Duration: viewScale,
		Pitch:    pitch,
		Velocity: 100,
	}
	if newNote.Duration < 0.25 {
		newNote.Duration = 0.25
	}
	pat.Notes = append(pat.Notes, newNote)
	p.selectedNote = len(pat.Notes) - 1
	p.centerOnSelection()
}

func (p *PianoRollDevice) HelpLayout() widgets.LaunchpadLayout {
	topRowColor := [3]uint8{111, 10, 126}
	gridColor := [3]uint8{80, 200, 255}
	sceneColor := [3]uint8{148, 18, 126}

	var layout widgets.LaunchpadLayout

	for i := range 8 {
		layout.TopRow[i] = widgets.PadConfig{Color: topRowColor, Tooltip: "Mode"}
	}

	for row := range 8 {
		for col := range 8 {
			layout.Grid[row][col] = widgets.PadConfig{Color: gridColor, Tooltip: "Note"}
		}
	}

	for i := range 8 {
		layout.RightCol[i] = widgets.PadConfig{Color: sceneColor, Tooltip: "Scene"}
	}

	return layout
}
