package sequencer

import (
	"fmt"
	"sort"
	"sync"

	"go-sequence/midi"
	"go-sequence/widgets"
)

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

// PianoRollDevice reads/writes from central PianoState
type PianoRollDevice struct {
	state        *PianoState
	heldNotes    map[uint8]bool            // runtime only - for note-off tracking during playback
	pendingNotes map[uint8]*NoteEventState // runtime only - for recording note-on/note-off pairs

	// Queue-based playback - protected by queueMu (held ONLY during swap, not generation)
	queueMu          sync.RWMutex
	queue            []midi.Event // events sorted by tick
	queuedUntilTick  int64        // how far we've filled the queue
	patternStartTick int64        // tick when current pattern started
	onQueueChange    func()       // callback to wake manager when queue needs recalc

	// Pattern switching
	nextPatternTick int64 // tick when next pattern should start (-1 if none)
}

// NewPianoRollDevice creates a device that operates on the given state
func NewPianoRollDevice(state *PianoState) *PianoRollDevice {
	return &PianoRollDevice{
		state:           state,
		heldNotes:       make(map[uint8]bool),
		pendingNotes:    make(map[uint8]*NoteEventState),
		nextPatternTick: -1,
	}
}

// SetOnQueueChange sets the callback for when the queue needs recalculation
func (p *PianoRollDevice) SetOnQueueChange(fn func()) {
	p.onQueueChange = fn
}

// currentBeat returns the current playback beat derived from global tick
func (p *PianoRollDevice) currentBeat() float64 {
	ticksSinceStart := S.Tick - p.patternStartTick
	if ticksSinceStart < 0 {
		ticksSinceStart = 0
	}
	pat := &p.state.Patterns[p.state.Pattern]
	patternTicks := int64(pat.Length * float64(PPQ))
	tickInPattern := ticksSinceStart % patternTicks
	return float64(tickInPattern) / float64(PPQ)
}

// GeneratePattern generates all MIDI events for a pattern starting at startTick.
// This is the ONLY place pattern data → events conversion happens.
func (p *PianoRollDevice) GeneratePattern(patternNum int, startTick int64) []midi.Event {
	pat := &p.state.Patterns[patternNum]
	ticksPerBeat := int64(PPQ)

	var events []midi.Event

	for _, note := range pat.Notes {
		// Note on
		noteTick := startTick + int64(note.Start*float64(ticksPerBeat))
		events = append(events, midi.Event{
			Tick:     noteTick,
			Type:     midi.NoteOn,
			Note:     note.Pitch,
			Velocity: note.Velocity,
		})

		// Note off
		noteEndTick := startTick + int64((note.Start+note.Duration)*float64(ticksPerBeat))
		events = append(events, midi.Event{
			Tick: noteEndTick,
			Type: midi.NoteOff,
			Note: note.Pitch,
		})
	}

	// Sort by tick (notes may not be in time order)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return events
}

// patternLengthTicks returns the length of a pattern in ticks
func (p *PianoRollDevice) patternLengthTicks(patternNum int) int64 {
	pat := &p.state.Patterns[patternNum]
	return int64(pat.Length * float64(PPQ))
}

// Device interface implementation - queue-based

// FillUntil fills the event queue with events up to the given tick
func (p *PianoRollDevice) FillUntil(tick int64) {
	// Read current state
	p.queueMu.RLock()
	queuedUntil := p.queuedUntilTick
	patternStart := p.patternStartTick
	nextPatTick := p.nextPatternTick
	p.queueMu.RUnlock()

	if queuedUntil >= tick {
		return // already filled
	}

	// Generate events OUTSIDE the lock
	var newEvents []midi.Event
	currentPattern := p.state.Pattern
	for queuedUntil < tick {
		// Check for pattern switch at boundary
		if nextPatTick >= 0 && queuedUntil >= nextPatTick {
			p.state.Pattern = p.state.Next
			currentPattern = p.state.Pattern
			patternStart = nextPatTick
			nextPatTick = -1
		}

		events := p.GeneratePattern(currentPattern, queuedUntil)
		newEvents = append(newEvents, events...)
		queuedUntil += p.patternLengthTicks(currentPattern)
	}

	// Swap in new events (brief lock)
	p.queueMu.Lock()
	p.queue = append(p.queue, newEvents...)
	p.queuedUntilTick = queuedUntil
	p.patternStartTick = patternStart
	p.nextPatternTick = nextPatTick
	p.queueMu.Unlock()
}

// regeneratePatternInQueue replaces events for a pattern if it's currently in the queue.
// Called from UI thread - generates events WITHOUT holding lock, then swaps.
func (p *PianoRollDevice) regeneratePatternInQueue(patternNum int) {
	if patternNum != p.state.Pattern {
		return // only regenerate if it's the playing pattern
	}

	patternTicks := p.patternLengthTicks(patternNum)

	// --- Read current state (brief lock) ---
	p.queueMu.RLock()
	oldQueue := p.queue
	oldQueuedUntil := p.queuedUntilTick
	patternStart := p.patternStartTick
	p.queueMu.RUnlock()

	// --- Generate new queue OUTSIDE the lock (this is the slow part) ---
	var newQueue []midi.Event

	// Keep events before current pattern start
	for _, e := range oldQueue {
		if e.Tick < patternStart {
			newQueue = append(newQueue, e)
		}
	}

	// Regenerate from pattern start to where we had queued
	newQueuedUntil := patternStart
	for newQueuedUntil < oldQueuedUntil {
		events := p.GeneratePattern(p.state.Pattern, newQueuedUntil)
		newQueue = append(newQueue, events...)
		newQueuedUntil += patternTicks
	}

	// --- Swap in new queue (brief lock) ---
	p.queueMu.Lock()
	p.queue = newQueue
	p.queuedUntilTick = newQueuedUntil
	p.queueMu.Unlock()

	// --- Wake dispatch loop to recalculate next event ---
	if p.onQueueChange != nil {
		p.onQueueChange()
	}
}

// PeekNextEvent returns the next event without removing it
func (p *PianoRollDevice) PeekNextEvent() *midi.Event {
	p.queueMu.RLock()
	defer p.queueMu.RUnlock()

	if len(p.queue) == 0 {
		return nil
	}
	return &p.queue[0]
}

// PopNextEvent removes and returns the next event
func (p *PianoRollDevice) PopNextEvent() *midi.Event {
	p.queueMu.Lock()
	defer p.queueMu.Unlock()

	if len(p.queue) == 0 {
		return nil
	}
	event := p.queue[0]
	p.queue = p.queue[1:]
	return &event
}

// ClearQueue clears all queued events (for stop/restart)
func (p *PianoRollDevice) ClearQueue() {
	p.queueMu.Lock()
	defer p.queueMu.Unlock()

	p.queue = nil
	p.queuedUntilTick = 0
	p.patternStartTick = 0
	p.nextPatternTick = -1
	p.heldNotes = make(map[uint8]bool)
}

// QueuePattern queues a pattern change at the next boundary after atTick
func (p *PianoRollDevice) QueuePattern(patIdx int, atTick int64) {
	if patIdx < 0 || patIdx >= NumPatterns {
		return
	}
	p.state.Next = patIdx

	// Find next pattern boundary
	pat := &p.state.Patterns[p.state.Pattern]
	ticksPerBeat := int64(PPQ)
	patternTicks := int64(pat.Length * float64(ticksPerBeat))

	// Read state under lock
	p.queueMu.RLock()
	patternStart := p.patternStartTick
	queuedUntil := p.queuedUntilTick
	p.queueMu.RUnlock()

	// Calculate when the next pattern boundary occurs
	ticksSinceStart := atTick - patternStart
	ticksIntoPattern := ticksSinceStart % patternTicks
	ticksToNextBoundary := patternTicks - ticksIntoPattern
	boundaryTick := atTick + ticksToNextBoundary

	needsNotify := false

	// If we've already queued past the boundary, wipe those events
	if queuedUntil > boundaryTick {
		p.queueMu.Lock()
		newQueue := p.queue[:0]
		for _, e := range p.queue {
			if e.Tick < boundaryTick {
				newQueue = append(newQueue, e)
			}
		}
		p.queue = newQueue
		p.queuedUntilTick = boundaryTick
		p.nextPatternTick = boundaryTick
		p.queueMu.Unlock()
		needsNotify = true
	} else {
		p.queueMu.Lock()
		p.nextPatternTick = boundaryTick
		p.queueMu.Unlock()
	}

	// Wake manager outside the lock
	if needsNotify && p.onQueueChange != nil {
		p.onQueueChange()
	}
}

// CurrentPattern returns the currently playing pattern
func (p *PianoRollDevice) CurrentPattern() int {
	return p.state.Pattern
}

// NextPattern returns the queued pattern (-1 if none)
func (p *PianoRollDevice) NextPattern() int {
	if p.nextPatternTick >= 0 {
		return p.state.Next
	}
	return -1
}

func (p *PianoRollDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i := range p.state.Patterns {
		if len(p.state.Patterns[i].Notes) > 0 {
			mask[i] = true
		}
	}
	return mask
}

func (p *PianoRollDevice) HandleMIDI(event midi.Event) {
	// Only record while playing and recording is enabled
	if !S.Playing || !p.state.Recording {
		return
	}

	pattern := &p.state.Patterns[p.state.Editing]
	// Get current beat from global tick
	currentBeat := p.currentBeat()

	// Quantize to nearest 1/16th note
	quantized := float64(int(currentBeat*4+0.5)) / 4.0

	if event.Type == midi.NoteOn && event.Velocity > 0 {
		// Note on - start a pending note
		p.pendingNotes[event.Note] = &NoteEventState{
			Start:    quantized,
			Pitch:    event.Note,
			Velocity: event.Velocity,
		}
	} else if event.Type == midi.NoteOff || (event.Type == midi.NoteOn && event.Velocity == 0) {
		// Note off - complete the pending note
		if pending, ok := p.pendingNotes[event.Note]; ok {
			endBeat := float64(int(currentBeat*4+0.5)) / 4.0
			duration := endBeat - pending.Start
			if duration < 0.25 {
				duration = 0.25 // minimum 1/16th note
			}
			pending.Duration = duration
			pattern.Notes = append(pattern.Notes, *pending)
			delete(p.pendingNotes, event.Note)
		}
	}
}

func (p *PianoRollDevice) ToggleRecording() {
	p.state.Recording = !p.state.Recording
}

func (p *PianoRollDevice) TogglePreview() {
	p.state.Preview = !p.state.Preview
}

func (p *PianoRollDevice) IsRecording() bool {
	return p.state.Recording
}

func (p *PianoRollDevice) IsPreviewing() bool {
	return p.state.Preview
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
	s := p.state
	pat := &s.Patterns[s.Editing]

	playInfo := ""
	if s.Editing != s.Pattern {
		playInfo = fmt.Sprintf(" (playing:%d)", s.Pattern)
	}

	viewScale := ViewScales[s.ViewScale]
	editH := EditHorizSteps[s.EditHoriz]
	editV := EditVertSteps[s.EditVert]
	vertMode := "spread"
	if s.ViewRows == ViewSmushed {
		vertMode = "smushed"
	}

	beat := p.currentBeat()
	out := fmt.Sprintf("PIANO  Pattern %d%s  Beat %.1f/%g\n", s.Editing+1, playInfo, beat, pat.Length)
	out += fmt.Sprintf("View: %s/col %s  Edit: %s horiz, %d semi vert\n\n", formatStep(viewScale), vertMode, formatStep(editH), editV)

	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

	cols := 48
	rows := s.ViewRows

	beatsPerCol := viewScale
	totalBeats := float64(cols) * beatsPerCol
	startBeat := s.CenterBeat - totalBeats/2
	startPitch := int(s.CenterPitch) + rows/2

	playheadCol := -1
	if s.Editing == s.Pattern && beat >= startBeat {
		playheadCol = int((beat - startBeat) / beatsPerCol)
	}

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

			if colBeat < 0 || colBeat >= pat.Length {
				out += "-"
				continue
			}

			isPlayhead := col == playheadCol

			var notesHere []int
			var noteStartHere int = -1
			for i := range pat.Notes {
				n := &pat.Notes[i]
				if n.Pitch == pitch {
					noteEnd := n.Start + n.Duration
					if n.Start < colBeatEnd && noteEnd > colBeat {
						notesHere = append(notesHere, i)
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
					if noteStartHere == s.SelectedNote {
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

	if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
		n := &pat.Notes[s.SelectedNote]
		noteName := noteNames[n.Pitch%12]
		octNum := n.Pitch / 12
		out += fmt.Sprintf("\nSelected: %s%d  start:%.2f  dur:%.2f  vel:%d", noteName, octNum, n.Start, n.Duration, n.Velocity)
	}

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

	out += "\n\n"
	out += p.renderLaunchpadHelp()

	return out
}

func (p *PianoRollDevice) RenderLEDs() []LEDState {
	var leds []LEDState
	s := p.state
	pat := &s.Patterns[s.Editing]

	noteColor := [3]uint8{80, 200, 255}
	selectedColor := [3]uint8{255, 100, 200}
	dimColor := [3]uint8{20, 50, 70}
	playheadColor := [3]uint8{255, 255, 255}
	offColor := [3]uint8{0, 0, 0}

	basePitch := int(s.CenterPitch) - 4
	viewScale := ViewScales[s.ViewScale]
	startBeat := s.CenterBeat - 4*viewScale
	beat := p.currentBeat()

	playheadCol := -1
	if s.Editing == s.Pattern && beat >= startBeat {
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
							if i == s.SelectedNote {
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

func (p *PianoRollDevice) centerOnSelection() {
	s := p.state
	pat := &s.Patterns[s.Editing]
	if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
		n := &pat.Notes[s.SelectedNote]
		s.CenterBeat = n.Start
		s.CenterPitch = float64(n.Pitch)
	}
}

func (p *PianoRollDevice) selectNoteByTime(direction int) {
	s := p.state
	pat := &s.Patterns[s.Editing]
	if len(pat.Notes) == 0 {
		return
	}
	s.SelectedNote += direction
	if s.SelectedNote < 0 {
		s.SelectedNote = len(pat.Notes) - 1
	} else if s.SelectedNote >= len(pat.Notes) {
		s.SelectedNote = 0
	}
	p.centerOnSelection()
}

func (p *PianoRollDevice) selectNoteByPitch(direction int) {
	s := p.state
	pat := &s.Patterns[s.Editing]
	if len(pat.Notes) == 0 {
		return
	}

	if s.SelectedNote < 0 || s.SelectedNote >= len(pat.Notes) {
		s.SelectedNote = 0
		p.centerOnSelection()
		return
	}

	currentNote := pat.Notes[s.SelectedNote]
	targetPitch := int(currentNote.Pitch) + direction

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
			break
		}
	}

	if bestIdx >= 0 {
		s.SelectedNote = bestIdx
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
	s := p.state
	pat := &s.Patterns[s.Editing]
	editH := EditHorizSteps[s.EditHoriz]
	editV := EditVertSteps[s.EditVert]

	switch key {
	case "h", "left":
		p.selectNoteByTime(-1)
	case "l", "right":
		p.selectNoteByTime(1)
	case "j", "down":
		p.selectNoteByPitch(-1)
	case "k", "up":
		p.selectNoteByPitch(1)

	case "y":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if n.Start >= editH {
				n.Start -= editH
			} else {
				n.Start = 0
			}
			p.centerOnSelection()
		}
	case "o":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				n.Start += editH
			}
			p.centerOnSelection()
		}
	case "u":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if int(n.Pitch) >= editV {
				n.Pitch -= uint8(editV)
			}
			p.centerOnSelection()
		}
	case "i":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if int(n.Pitch)+editV <= 127 {
				n.Pitch += uint8(editV)
			}
			p.centerOnSelection()
		}

	case "n":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if n.Duration > editH {
				n.Duration -= editH
			}
		}
	case "m":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				n.Duration += editH
			}
		}

	case "q":
		if s.ViewScale < len(ViewScales)-1 {
			s.ViewScale++
		}
	case "w":
		if s.ViewScale > 0 {
			s.ViewScale--
		}
	case "a":
		s.ViewRows = ViewSmushed
	case "s":
		s.ViewRows = ViewSpread

	case "d":
		if s.EditHoriz < len(EditHorizSteps)-1 {
			s.EditHoriz++
		}
	case "f":
		if s.EditHoriz > 0 {
			s.EditHoriz--
		}
	case "e":
		if s.EditVert < len(EditVertSteps)-1 {
			s.EditVert++
		}
	case "r":
		if s.EditVert > 0 {
			s.EditVert--
		}

	case " ":
		newNote := NoteEventState{
			Start:    s.CenterBeat,
			Duration: EditHorizSteps[s.EditHoriz] * 4,
			Pitch:    uint8(s.CenterPitch),
			Velocity: 100,
		}
		if newNote.Duration < 0.25 {
			newNote.Duration = 0.25
		}
		pat.Notes = append(pat.Notes, newNote)
		s.SelectedNote = len(pat.Notes) - 1
		p.centerOnSelection()

	case "x":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			pat.Notes = append(pat.Notes[:s.SelectedNote], pat.Notes[s.SelectedNote+1:]...)
			if s.SelectedNote >= len(pat.Notes) {
				s.SelectedNote = len(pat.Notes) - 1
			}
			if s.SelectedNote >= 0 {
				p.centerOnSelection()
			}
		}

	case "[":
		if pat.Length > 1.0 {
			pat.Length -= 1.0
		}
	case "]":
		if pat.Length < 64.0 {
			pat.Length += 1.0
		}

	case "c":
		pat.Notes = []NoteEventState{}
		s.SelectedNote = -1

	case "<":
		if s.Editing > 0 {
			s.Editing--
			s.SelectedNote = -1
		}
	case ">", ".":
		if s.Editing < NumPatterns-1 {
			s.Editing++
			s.SelectedNote = -1
		}
	}

	// Keep notes sorted by start time, preserving selection
	var selectedNote *NoteEventState
	if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
		n := pat.Notes[s.SelectedNote]
		selectedNote = &n
	}

	sort.Slice(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})

	if selectedNote != nil {
		for i, n := range pat.Notes {
			if n.Start == selectedNote.Start && n.Pitch == selectedNote.Pitch {
				s.SelectedNote = i
				break
			}
		}
	}
}

func (p *PianoRollDevice) HandlePad(row, col int) {
	s := p.state
	pat := &s.Patterns[s.Editing]

	basePitch := int(s.CenterPitch) - 4
	viewScale := ViewScales[s.ViewScale]
	startBeat := s.CenterBeat - 4*viewScale

	pitch := uint8(basePitch + row)
	beat := startBeat + float64(col)*viewScale
	beatEnd := beat + viewScale

	if beat < 0 || beat >= pat.Length || pitch > 127 {
		return
	}

	for i, n := range pat.Notes {
		if n.Pitch == pitch {
			noteEnd := n.Start + n.Duration
			if n.Start < beatEnd && noteEnd > beat {
				s.SelectedNote = i
				p.centerOnSelection()
				return
			}
		}
	}

	newNote := NoteEventState{
		Start:    beat,
		Duration: viewScale,
		Pitch:    pitch,
		Velocity: 100,
	}
	if newNote.Duration < 0.25 {
		newNote.Duration = 0.25
	}
	pat.Notes = append(pat.Notes, newNote)
	s.SelectedNote = len(pat.Notes) - 1
	p.centerOnSelection()
}

func (p *PianoRollDevice) renderLaunchpadHelp() string {
	topRowColor := [3]uint8{111, 10, 126}
	gridColor := [3]uint8{80, 200, 255}
	sceneColor := [3]uint8{148, 18, 126}

	var grid [8][8][3]uint8
	var rightCol [8][3]uint8
	topRow := make([][3]uint8, 8)

	for i := 0; i < 8; i++ {
		topRow[i] = topRowColor
		rightCol[i] = sceneColor
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = gridColor
		}
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, &rightCol) + "\n\n"
	out += widgets.RenderLegendItem(gridColor, "Notes", "tap to add/select notes") + "\n"
	out += widgets.RenderLegendItem(sceneColor, "Scene", "launch scenes")

	return out
}
