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

// PianoSchedule tracks what patterns play at what ticks (source of truth for playback)
type PianoSchedule struct {
	StartTick int64 // when patterns[0] starts
	Patterns  []int // pattern indices in order
}

// PianoRollDevice reads/writes from central PianoState
type PianoRollDevice struct {
	state        *PianoState
	heldNotes    map[uint8]bool            // runtime only - for note-off tracking during playback
	pendingNotes map[uint8]*NoteEventState // runtime only - for recording note-on/note-off pairs

	// Schedule - source of truth for what plays when
	schedule     PianoSchedule
	patternDirty [NumPatterns]bool

	// Queue - derived from schedule + pattern data
	queueMu          sync.RWMutex
	queue            []midi.Event // events sorted by tick
	patternStartTick int64        // tick when current pattern started (updated by sync)
	onQueueChange    func()       // callback to wake manager when queue needs recalc
}

// NewPianoRollDevice creates a device that operates on the given state
func NewPianoRollDevice(state *PianoState) *PianoRollDevice {
	return &PianoRollDevice{
		state:        state,
		heldNotes:    make(map[uint8]bool),
		pendingNotes: make(map[uint8]*NoteEventState),
		schedule: PianoSchedule{
			StartTick: 0,
			Patterns:  []int{0},
		},
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

// --- Schedule helpers ---

func (p *PianoRollDevice) scheduleEndTick() int64 {
	tick := p.schedule.StartTick
	for _, patIdx := range p.schedule.Patterns {
		tick += p.patternLengthTicks(patIdx)
	}
	return tick
}

func (p *PianoRollDevice) extendSchedule(targetTick int64) {
	for p.scheduleEndTick() < targetTick {
		lastPat := 0
		if len(p.schedule.Patterns) > 0 {
			lastPat = p.schedule.Patterns[len(p.schedule.Patterns)-1]
		}
		p.schedule.Patterns = append(p.schedule.Patterns, lastPat)
	}
}

func (p *PianoRollDevice) trimSchedule(currentTick int64) {
	for len(p.schedule.Patterns) > 1 {
		firstPatLen := p.patternLengthTicks(p.schedule.Patterns[0])
		if p.schedule.StartTick+firstPatLen <= currentTick {
			p.schedule.StartTick += firstPatLen
			p.schedule.Patterns = p.schedule.Patterns[1:]
		} else {
			break
		}
	}
}

func (p *PianoRollDevice) scheduleContainsDirty() bool {
	for _, patIdx := range p.schedule.Patterns {
		if p.patternDirty[patIdx] {
			return true
		}
	}
	return false
}

func (p *PianoRollDevice) clearDirtyFlags() {
	for i := range p.patternDirty {
		p.patternDirty[i] = false
	}
}

func (p *PianoRollDevice) queueEndTick() int64 {
	p.queueMu.RLock()
	defer p.queueMu.RUnlock()
	if len(p.queue) == 0 {
		return p.schedule.StartTick
	}
	return p.queue[len(p.queue)-1].Tick
}

// syncQueueToSchedule regenerates the queue from the schedule
func (p *PianoRollDevice) syncQueueToSchedule() {
	if !p.scheduleContainsDirty() {
		return
	}

	var newQueue []midi.Event
	tick := p.schedule.StartTick
	for _, patIdx := range p.schedule.Patterns {
		events := p.GeneratePattern(patIdx, tick)
		newQueue = append(newQueue, events...)
		tick += p.patternLengthTicks(patIdx)
	}

	if len(p.schedule.Patterns) > 0 {
		p.state.Pattern = p.schedule.Patterns[0]
	}
	p.patternStartTick = p.schedule.StartTick

	p.queueMu.Lock()
	p.queue = newQueue
	p.queueMu.Unlock()

	p.clearDirtyFlags()
	if p.onQueueChange != nil {
		p.onQueueChange()
	}
}

// markDirtyAndSync marks the editing pattern dirty and wakes queue manager
func (p *PianoRollDevice) markDirtyAndSync() {
	p.patternDirty[p.state.Editing] = true
	if p.onQueueChange != nil {
		p.onQueueChange()
	}
}

// Device interface implementation - queue-based

// FillUntil fills the event queue with events up to the given tick
func (p *PianoRollDevice) FillUntil(tick int64) {
	p.trimSchedule(S.Tick)
	p.extendSchedule(tick)

	if p.scheduleContainsDirty() {
		p.syncQueueToSchedule()
	}
	if len(p.queue) == 0 || p.queueEndTick() < tick {
		for _, patIdx := range p.schedule.Patterns {
			p.patternDirty[patIdx] = true
		}
		p.syncQueueToSchedule()
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
	p.queue = nil
	p.queueMu.Unlock()

	p.schedule.StartTick = 0
	p.schedule.Patterns = []int{p.state.Pattern}
	p.patternStartTick = 0
	p.clearDirtyFlags()
	p.heldNotes = make(map[uint8]bool)
}

// QueuePattern queues a pattern change at the next boundary after atTick
func (p *PianoRollDevice) QueuePattern(patIdx int, atTick int64) {
	if patIdx < 0 || patIdx >= NumPatterns {
		return
	}
	p.state.Next = patIdx

	p.extendSchedule(atTick)

	tick := p.schedule.StartTick
	foundSlot := false
	for i, idx := range p.schedule.Patterns {
		patLen := p.patternLengthTicks(idx)
		if tick+patLen > atTick {
			nextSlot := i + 1
			if nextSlot < len(p.schedule.Patterns) {
				for j := nextSlot; j < len(p.schedule.Patterns); j++ {
					p.schedule.Patterns[j] = patIdx
				}
			} else {
				p.schedule.Patterns = append(p.schedule.Patterns, patIdx)
			}
			foundSlot = true
			break
		}
		tick += patLen
	}
	if !foundSlot {
		p.schedule.Patterns = append(p.schedule.Patterns, patIdx)
	}

	p.patternDirty[patIdx] = true
	p.syncQueueToSchedule()
}

// CurrentPattern returns the currently playing pattern
func (p *PianoRollDevice) CurrentPattern() int {
	return p.state.Pattern
}

// NextPattern returns the queued pattern (-1 if none)
func (p *PianoRollDevice) NextPattern() int {
	if len(p.schedule.Patterns) > 1 && p.schedule.Patterns[1] != p.schedule.Patterns[0] {
		return p.schedule.Patterns[1]
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

// --- Core edit functions (all operate on Editing pattern, call markDirtyAndSync) ---

func (p *PianoRollDevice) AddNote(start, duration float64, pitch uint8, velocity uint8) {
	pat := &p.state.Patterns[p.state.Editing]
	if duration < 0.25 {
		duration = 0.25
	}
	if start < 0 {
		start = 0
	}
	if start+duration > pat.Length {
		return
	}
	pat.Notes = append(pat.Notes, NoteEventState{
		Start: start, Duration: duration, Pitch: pitch, Velocity: velocity,
	})
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) AddNoteFromRecord(note NoteEventState) {
	pat := &p.state.Patterns[p.state.Editing]
	pat.Notes = append(pat.Notes, note)
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) DeleteNote(idx int) {
	pat := &p.state.Patterns[p.state.Editing]
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	pat.Notes = append(pat.Notes[:idx], pat.Notes[idx+1:]...)
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) MoveNoteTime(idx int, deltaBeat float64) {
	pat := &p.state.Patterns[p.state.Editing]
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	n.Start += deltaBeat
	if n.Start < 0 {
		n.Start = 0
	}
	if n.Start+n.Duration > pat.Length {
		n.Start = pat.Length - n.Duration
	}
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) MoveNotePitch(idx int, deltaSemitones int) {
	pat := &p.state.Patterns[p.state.Editing]
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	if int(n.Pitch)+deltaSemitones < 0 {
		n.Pitch = 0
	} else if int(n.Pitch)+deltaSemitones > 127 {
		n.Pitch = 127
	} else {
		n.Pitch = uint8(int(n.Pitch) + deltaSemitones)
	}
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) SetNoteDuration(idx int, duration float64) {
	pat := &p.state.Patterns[p.state.Editing]
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	if duration < 0.25 {
		duration = 0.25
	}
	if n.Start+duration > pat.Length {
		duration = pat.Length - n.Start
	}
	n.Duration = duration
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) SetPatternLength(beats float64) {
	if beats < 1.0 || beats > 64.0 {
		return
	}
	pat := &p.state.Patterns[p.state.Editing]
	pat.Length = beats
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) ClearEditingPattern() {
	pat := &p.state.Patterns[p.state.Editing]
	pat.Notes = []NoteEventState{}
	p.markDirtyAndSync()
}

func (p *PianoRollDevice) HandleMIDI(event midi.Event) {
	if !S.Playing || !p.state.Recording {
		return
	}

	currentBeat := p.currentBeat()
	quantized := float64(int(currentBeat*4+0.5)) / 4.0

	if event.Type == midi.NoteOn && event.Velocity > 0 {
		p.pendingNotes[event.Note] = &NoteEventState{
			Start: quantized, Pitch: event.Note, Velocity: event.Velocity,
		}
	} else if event.Type == midi.NoteOff || (event.Type == midi.NoteOn && event.Velocity == 0) {
		if pending, ok := p.pendingNotes[event.Note]; ok {
			endBeat := float64(int(currentBeat*4+0.5)) / 4.0
			duration := endBeat - pending.Start
			if duration < 0.25 {
				duration = 0.25
			}
			pending.Duration = duration
			p.AddNoteFromRecord(*pending)
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
			delta := -editH
			if n.Start+delta < 0 {
				delta = -n.Start
			}
			p.MoveNoteTime(s.SelectedNote, delta)
			p.centerOnSelection()
		}
	case "o":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			delta := editH
			if n.Start+n.Duration+delta > pat.Length {
				delta = pat.Length - n.Start - n.Duration
			}
			p.MoveNoteTime(s.SelectedNote, delta)
			p.centerOnSelection()
		}
	case "u":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			p.MoveNotePitch(s.SelectedNote, -editV)
			p.centerOnSelection()
		}
	case "i":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			p.MoveNotePitch(s.SelectedNote, editV)
			p.centerOnSelection()
		}

	case "n":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if n.Duration > editH {
				p.SetNoteDuration(s.SelectedNote, n.Duration-editH)
			}
		}
	case "m":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[s.SelectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				p.SetNoteDuration(s.SelectedNote, n.Duration+editH)
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
		dur := EditHorizSteps[s.EditHoriz] * 4
		p.AddNote(s.CenterBeat, dur, uint8(s.CenterPitch), 100)
		s.SelectedNote = len(pat.Notes) - 1
		p.centerOnSelection()

	case "x":
		if s.SelectedNote >= 0 && s.SelectedNote < len(pat.Notes) {
			p.DeleteNote(s.SelectedNote)
			if s.SelectedNote >= len(pat.Notes) {
				s.SelectedNote = len(pat.Notes) - 1
			}
			if s.SelectedNote >= 0 {
				p.centerOnSelection()
			}
		}

	case "[":
		if pat.Length > 1.0 {
			p.SetPatternLength(pat.Length - 1.0)
		}
	case "]":
		if pat.Length < 64.0 {
			p.SetPatternLength(pat.Length + 1.0)
		}

	case "c":
		p.ClearEditingPattern()
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

	dur := viewScale
	if dur < 0.25 {
		dur = 0.25
	}
	p.AddNote(beat, dur, pitch, 100)
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
