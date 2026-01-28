package sequencer

import (
	"fmt"
	"sync"

	"go-sequence/midi"
	"go-sequence/widgets"
)

// DrumSchedule tracks what patterns play at what ticks (source of truth for playback)
type DrumSchedule struct {
	StartTick int64 // when patterns[0] starts
	Patterns  []int // pattern indices in order
}

// DrumDevice reads/writes from central DrumState
type DrumDevice struct {
	state       *DrumState
	previewChan chan int // sends slot index when preview sound should play

	// Schedule - source of truth for what plays when
	schedule     DrumSchedule
	patternDirty [NumPatterns]bool // tracks which patterns have been modified

	// Queue - derived from schedule + pattern data
	queueMu       sync.RWMutex
	queue         []midi.Event // events sorted by tick
	onQueueChange func()       // callback to wake manager when queue needs recalc

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
		schedule: DrumSchedule{
			StartTick: 0,
			Patterns:  []int{0}, // start with pattern 0
		},
	}
}

// SetOnQueueChange sets the callback for when the queue needs recalculation
func (d *DrumDevice) SetOnQueueChange(fn func()) {
	d.onQueueChange = fn
}

// PreviewChan returns the channel for preview events (slot indices)
func (d *DrumDevice) PreviewChan() <-chan int {
	return d.previewChan
}

// currentStep returns the current playback step derived from global tick
func (d *DrumDevice) currentStep() int {
	ticksPerStep := int64(PPQ / 4)
	ticksSinceStart := S.Tick - d.schedule.StartTick
	if ticksSinceStart < 0 {
		ticksSinceStart = 0
	}
	return int(ticksSinceStart / ticksPerStep)
}

// GeneratePattern generates all MIDI events for a pattern starting at startTick.
// This is the ONLY place pattern data → events conversion happens.
func (d *DrumDevice) GeneratePattern(patternNum int, startTick int64) []midi.Event {
	pat := &d.state.Patterns[patternNum]
	masterLen := pat.MasterLength()
	ticksPerStep := int64(PPQ / 4)

	var events []midi.Event

	// Generate events for each step in the pattern
	for step := 0; step < masterLen; step++ {
		stepTick := startTick + int64(step)*ticksPerStep

		// Check all 16 notes at this step
		for noteIdx := 0; noteIdx < 16; noteIdx++ {
			note := &pat.Notes[noteIdx]
			// Each note loops at its own length (polymeters)
			noteStep := step % note.Length
			s := &note.Steps[noteStep]
			if s.Active {
				events = append(events, midi.Event{
					Tick:     stepTick,
					Type:     midi.Trigger,
					Note:     uint8(noteIdx), // Manager translates via kit
					Velocity: s.Velocity,
				})
			}
		}
	}

	return events
}

// patternLengthTicks returns the length of a pattern in ticks
func (d *DrumDevice) patternLengthTicks(patternNum int) int64 {
	pat := &d.state.Patterns[patternNum]
	return int64(pat.MasterLength()) * (PPQ / 4)
}

// --- Schedule helpers ---

// scheduleEndTick returns the tick where the current schedule ends
func (d *DrumDevice) scheduleEndTick() int64 {
	tick := d.schedule.StartTick
	for _, patIdx := range d.schedule.Patterns {
		tick += d.patternLengthTicks(patIdx)
	}
	return tick
}

// extendSchedule ensures the schedule covers at least until targetTick
func (d *DrumDevice) extendSchedule(targetTick int64) {
	for d.scheduleEndTick() < targetTick {
		// Append the last pattern (or pattern 0 if empty)
		lastPat := 0
		if len(d.schedule.Patterns) > 0 {
			lastPat = d.schedule.Patterns[len(d.schedule.Patterns)-1]
		}
		d.schedule.Patterns = append(d.schedule.Patterns, lastPat)
	}
}

// trimSchedule drops patterns that are entirely behind the playhead
func (d *DrumDevice) trimSchedule(currentTick int64) {
	for len(d.schedule.Patterns) > 1 {
		firstPatLen := d.patternLengthTicks(d.schedule.Patterns[0])
		if d.schedule.StartTick+firstPatLen <= currentTick {
			// First pattern is entirely in the past - drop it
			d.schedule.StartTick += firstPatLen
			d.schedule.Patterns = d.schedule.Patterns[1:]
		} else {
			break
		}
	}
}

// scheduleContainsDirty checks if any dirty pattern is in the schedule
func (d *DrumDevice) scheduleContainsDirty() bool {
	for _, patIdx := range d.schedule.Patterns {
		if d.patternDirty[patIdx] {
			return true
		}
	}
	return false
}

// clearDirtyFlags clears all pattern dirty flags
func (d *DrumDevice) clearDirtyFlags() {
	for i := range d.patternDirty {
		d.patternDirty[i] = false
	}
}

// syncQueueToSchedule regenerates the queue from the schedule
// This is the single function that reconciles queue with schedule
func (d *DrumDevice) syncQueueToSchedule() {
	// Check if we have work to do
	if !d.scheduleContainsDirty() {
		return
	}

	// Generate all events from schedule
	var newQueue []midi.Event
	tick := d.schedule.StartTick
	for _, patIdx := range d.schedule.Patterns {
		events := d.GeneratePattern(patIdx, tick)
		newQueue = append(newQueue, events...)
		tick += d.patternLengthTicks(patIdx)
	}

	// Update playing pattern index to match schedule
	if len(d.schedule.Patterns) > 0 {
		d.state.PlayingPatternIdx = d.schedule.Patterns[0]
	}

	// Swap in new queue
	d.queueMu.Lock()
	d.queue = newQueue
	d.queueMu.Unlock()

	// Clear dirty flags
	d.clearDirtyFlags()

	// Wake manager
	if d.onQueueChange != nil {
		d.onQueueChange()
	}
}

// Device interface implementation - queue-based

// FillUntil fills the event queue with events up to the given tick
func (d *DrumDevice) FillUntil(tick int64) {
	// Trim old patterns behind playhead
	d.trimSchedule(S.Tick)

	// Extend schedule to cover requested tick
	d.extendSchedule(tick)

	// Mark all scheduled patterns as dirty to force regeneration
	// (This ensures the queue gets built if it was empty)
	for _, patIdx := range d.schedule.Patterns {
		d.patternDirty[patIdx] = true
	}

	// Sync queue to schedule
	d.syncQueueToSchedule()
}

// PeekNextEvent returns the next event without removing it
func (d *DrumDevice) PeekNextEvent() *midi.Event {
	d.queueMu.RLock()
	defer d.queueMu.RUnlock()

	if len(d.queue) == 0 {
		return nil
	}
	return &d.queue[0]
}

// PopNextEvent removes and returns the next event
func (d *DrumDevice) PopNextEvent() *midi.Event {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()

	if len(d.queue) == 0 {
		return nil
	}
	event := d.queue[0]
	d.queue = d.queue[1:]
	return &event
}

// ClearQueue clears all queued events (for stop/restart)
func (d *DrumDevice) ClearQueue() {
	d.queueMu.Lock()
	d.queue = nil
	d.queueMu.Unlock()

	// Reset schedule to start fresh
	d.schedule.StartTick = 0
	d.schedule.Patterns = []int{d.state.PlayingPatternIdx}
	d.clearDirtyFlags()
}

// QueuePattern queues a pattern change at the next boundary after atTick
func (d *DrumDevice) QueuePattern(p int, atTick int64) {
	if p < 0 || p >= NumPatterns {
		return
	}
	d.state.Next = p

	// First, extend schedule to cover atTick if needed
	d.extendSchedule(atTick)

	// Find which schedule slot contains atTick, then replace everything after with new pattern
	tick := d.schedule.StartTick
	foundSlot := false
	for i, patIdx := range d.schedule.Patterns {
		patLen := d.patternLengthTicks(patIdx)
		if tick+patLen > atTick {
			// atTick is within this pattern - replace from next slot onward
			nextSlot := i + 1
			if nextSlot < len(d.schedule.Patterns) {
				// Replace remaining slots with new pattern
				for j := nextSlot; j < len(d.schedule.Patterns); j++ {
					d.schedule.Patterns[j] = p
				}
			} else {
				// No more slots - append the new pattern
				d.schedule.Patterns = append(d.schedule.Patterns, p)
			}
			foundSlot = true
			break
		}
		tick += patLen
	}

	// If atTick was beyond all patterns (shouldn't happen after extendSchedule, but be safe)
	if !foundSlot {
		d.schedule.Patterns = append(d.schedule.Patterns, p)
	}

	// Mark new pattern as dirty and sync
	d.patternDirty[p] = true
	d.syncQueueToSchedule()
}

// CurrentPattern returns the currently playing pattern
func (d *DrumDevice) CurrentPattern() int {
	return d.state.PlayingPatternIdx
}

// NextPattern returns the queued pattern (-1 if none)
func (d *DrumDevice) NextPattern() int {
	// Check if there's a different pattern scheduled after the current one
	if len(d.schedule.Patterns) > 1 && d.schedule.Patterns[1] != d.schedule.Patterns[0] {
		return d.schedule.Patterns[1]
	}
	return -1
}

func (d *DrumDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i := range d.state.Patterns {
		pat := &d.state.Patterns[i]
		for t := 0; t < 16; t++ {
			for s := 0; s < pat.Notes[t].Length; s++ {
				if pat.Notes[t].Steps[s].Active {
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

// HandleMIDI handles incoming MIDI for recording
func (d *DrumDevice) HandleMIDI(event midi.Event) {
	if !d.state.Recording || !S.Playing {
		return
	}

	// Only handle note-on with velocity
	if event.Type != midi.NoteOn || event.Velocity == 0 {
		return
	}

	// Find which note this MIDI note maps to (via kit reverse lookup would be ideal,
	// for now just use note as index if in range)
	noteIdx := int(event.Note)
	if noteIdx >= 16 {
		return
	}

	// Calculate step from the tick passed in the event
	ticksSinceStart := event.Tick - d.schedule.StartTick
	ticksPerStep := int64(PPQ / 4)
	pat := &d.state.Patterns[d.state.EditingPatternIdx]
	step := int((ticksSinceStart / ticksPerStep) % int64(pat.Notes[noteIdx].Length))

	// Use SetStep to write to the editing pattern
	d.SetStep(noteIdx, step, event.Velocity)
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

// --- Core Edit Functions ---
// All operate on EditingPatternIdx

// ToggleStep toggles a step on/off for a given note
func (d *DrumDevice) ToggleStep(note, step int) {
	pat := &d.state.Patterns[d.state.EditingPatternIdx]
	if note < 0 || note >= 16 || step < 0 || step >= pat.Notes[note].Length {
		return
	}
	pat.Notes[note].Steps[step].Active = !pat.Notes[note].Steps[step].Active
	d.patternDirty[d.state.EditingPatternIdx] = true
	d.syncQueueToSchedule()
}

// SetStep sets a step to active with a given velocity (for MIDI recording)
func (d *DrumDevice) SetStep(note, step int, velocity uint8) {
	pat := &d.state.Patterns[d.state.EditingPatternIdx]
	if note < 0 || note >= 16 || step < 0 || step >= pat.Notes[note].Length {
		return
	}
	pat.Notes[note].Steps[step].Active = true
	pat.Notes[note].Steps[step].Velocity = velocity
	d.patternDirty[d.state.EditingPatternIdx] = true
	d.syncQueueToSchedule()
}

// SetNoteLaneLength sets the length of a note lane
func (d *DrumDevice) SetNoteLaneLength(note, length int) {
	pat := &d.state.Patterns[d.state.EditingPatternIdx]
	if note < 0 || note >= 16 || length < 1 || length > 32 {
		return
	}
	pat.Notes[note].Length = length
	d.patternDirty[d.state.EditingPatternIdx] = true
	d.syncQueueToSchedule()
}

// ClearNote clears all steps in a note lane
func (d *DrumDevice) ClearNote(note int) {
	pat := &d.state.Patterns[d.state.EditingPatternIdx]
	if note < 0 || note >= 16 {
		return
	}
	for step := 0; step < 32; step++ {
		pat.Notes[note].Steps[step].Active = false
	}
	d.patternDirty[d.state.EditingPatternIdx] = true
	d.syncQueueToSchedule()
}

// ClearEditingPattern clears all notes in the editing pattern
func (d *DrumDevice) ClearEditingPattern() {
	pat := &d.state.Patterns[d.state.EditingPatternIdx]
	for n := 0; n < 16; n++ {
		for step := 0; step < 32; step++ {
			pat.Notes[n].Steps[step].Active = false
		}
	}
	d.patternDirty[d.state.EditingPatternIdx] = true
	d.syncQueueToSchedule()
}

func (d *DrumDevice) View() string {
	s := d.state
	pat := &s.Patterns[s.EditingPatternIdx]

	// Header - show editing vs playing
	playInfo := ""
	if s.EditingPatternIdx != s.PlayingPatternIdx {
		playInfo = fmt.Sprintf(" (playing:%d)", s.PlayingPatternIdx)
	}
	selectedNote := &pat.Notes[s.SelectedNoteIdx]
	currentStep := d.currentStep()
	selectedStep := currentStep % selectedNote.Length
	out := fmt.Sprintf("DRUM  Pattern %d%s  Step %d/%d  Note %d\n\n", s.EditingPatternIdx+1, playInfo, selectedStep+1, selectedNote.Length, s.SelectedNoteIdx+1)

	// Confirmation dialog takes over
	if d.confirmMode {
		out += "─────────────────────────────────────────────────\n"
		out += fmt.Sprintf("\n%s\n\n", d.confirmMsg)
		out += "  [y] Yes    [n] No\n"
		out += "\n─────────────────────────────────────────────────\n"
		return out
	}

	// 16x32 grid - single char per cell
	for n := 0; n < 16; n++ {
		note := &pat.Notes[n]
		noteStep := currentStep % note.Length
		out += fmt.Sprintf("%2d ", n+1)

		for step := 0; step < 32; step++ {
			isCursor := n == s.SelectedNoteIdx && step == s.Cursor

			var char string
			if step >= note.Length {
				if isCursor {
					char = "□"
				} else {
					char = "-"
				}
			} else if step == noteStep {
				if isCursor {
					char = "▷"
				} else {
					char = "▶"
				}
			} else if note.Steps[step].Active {
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
			{Key: "j / k", Desc: "select note up/down"},
			{Key: "space", Desc: "toggle step on/off"},
			{Key: "[ / ]", Desc: "shorten/lengthen note lane"},
			{Key: "c", Desc: "clear current note"},
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
	pat := &s.Patterns[s.EditingPatternIdx]
	selectedNote := &pat.Notes[s.SelectedNoteIdx]

	// Colors
	stepsColor := [3]uint8{234, 73, 116}
	stepsEmpty := [3]uint8{80, 30, 50}
	noteHasContent := [3]uint8{148, 18, 126}
	noteEmpty := [3]uint8{40, 10, 30}
	noteSelected := [3]uint8{255, 255, 255}
	commandsColor := [3]uint8{253, 157, 110}
	playheadColor := [3]uint8{255, 255, 255}
	offColor := [3]uint8{0, 0, 0}

	// Top 4 rows (rows 4-7): steps for selected note
	currentStep := d.currentStep()
	noteStep := currentStep % selectedNote.Length
	for stepIdx := 0; stepIdx < 32; stepIdx++ {
		row := 7 - (stepIdx / 8)
		col := stepIdx % 8

		var color [3]uint8 = offColor
		var channel uint8 = midi.ChannelStatic

		if stepIdx >= selectedNote.Length {
			color = offColor
		} else if stepIdx == noteStep {
			color = playheadColor
			channel = midi.ChannelPulse
		} else if selectedNote.Steps[stepIdx].Active {
			color = stepsColor
		} else {
			color = stepsEmpty
		}

		leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: channel})
	}

	// Bottom-left 4x4: note select
	for n := 0; n < 16; n++ {
		row := n / 4
		col := n % 4

		hasContent := false
		for step := 0; step < pat.Notes[n].Length; step++ {
			if pat.Notes[n].Steps[step].Active {
				hasContent = true
				break
			}
		}

		var color [3]uint8
		if n == s.SelectedNoteIdx {
			color = noteSelected
		} else if hasContent {
			color = noteHasContent
		} else {
			color = noteEmpty
		}

		leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
	}

	// Bottom-right 4x4: commands
	previewActive := [3]uint8{0, 255, 0} // green when on
	recordActive := [3]uint8{255, 0, 0}  // red when on
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
	pat := &s.Patterns[s.EditingPatternIdx]
	note := &pat.Notes[s.SelectedNoteIdx]

	switch key {
	case "h", "left":
		if s.Cursor > 0 {
			s.Cursor--
		}
	case "l", "right":
		if s.Cursor < note.Length-1 {
			s.Cursor++
		}
	case " ":
		d.ToggleStep(s.SelectedNoteIdx, s.Cursor)
	case "j", "down":
		if s.SelectedNoteIdx < 15 {
			s.SelectedNoteIdx++
			if s.Cursor >= s.Patterns[s.EditingPatternIdx].Notes[s.SelectedNoteIdx].Length {
				s.Cursor = s.Patterns[s.EditingPatternIdx].Notes[s.SelectedNoteIdx].Length - 1
			}
		}
	case "k", "up":
		if s.SelectedNoteIdx > 0 {
			s.SelectedNoteIdx--
			if s.Cursor >= s.Patterns[s.EditingPatternIdx].Notes[s.SelectedNoteIdx].Length {
				s.Cursor = s.Patterns[s.EditingPatternIdx].Notes[s.SelectedNoteIdx].Length - 1
			}
		}
	case "[":
		if note.Length > 1 {
			newLen := note.Length - 1
			d.SetNoteLaneLength(s.SelectedNoteIdx, newLen)
			if s.Cursor >= newLen {
				s.Cursor = newLen - 1
			}
		}
	case "]":
		if note.Length < 32 {
			d.SetNoteLaneLength(s.SelectedNoteIdx, note.Length+1)
		}
	case "c":
		d.confirmClearNote()
	case "C":
		d.confirmClearPattern()
	case "<", ",":
		if s.EditingPatternIdx > 0 {
			s.EditingPatternIdx--
		}
	case ">", ".":
		if s.EditingPatternIdx < NumPatterns-1 {
			s.EditingPatternIdx++
		}
	}
}

func (d *DrumDevice) confirmClearNote() {
	s := d.state
	pat := &s.Patterns[s.EditingPatternIdx]
	note := &pat.Notes[s.SelectedNoteIdx]
	noteIdx := s.SelectedNoteIdx // capture for closure

	// Check if note has any content
	hasContent := false
	for i := 0; i < note.Length; i++ {
		if note.Steps[i].Active {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return // nothing to clear
	}

	d.confirmMsg = fmt.Sprintf("Clear note %d?", noteIdx+1)
	d.confirmAction = func() {
		d.ClearNote(noteIdx)
	}
	d.confirmMode = true
}

func (d *DrumDevice) confirmClearPattern() {
	s := d.state
	pat := &s.Patterns[s.EditingPatternIdx]

	// Check if pattern has any content
	hasContent := false
	for n := 0; n < 16 && !hasContent; n++ {
		for step := 0; step < pat.Notes[n].Length; step++ {
			if pat.Notes[n].Steps[step].Active {
				hasContent = true
				break
			}
		}
	}
	if !hasContent {
		return // nothing to clear
	}

	d.confirmMsg = fmt.Sprintf("Clear pattern %d?", s.EditingPatternIdx+1)
	d.confirmAction = func() {
		d.ClearEditingPattern()
	}
	d.confirmMode = true
}

func (d *DrumDevice) HandlePad(row, col int) {
	s := d.state
	pat := &s.Patterns[s.EditingPatternIdx]

	// Top 4 rows: step toggle
	if row >= 4 && row <= 7 {
		stepIdx := (7-row)*8 + col
		note := &pat.Notes[s.SelectedNoteIdx]
		if stepIdx < note.Length {
			d.ToggleStep(s.SelectedNoteIdx, stepIdx)
			s.Cursor = stepIdx
		}
		return
	}

	// Bottom-left 4x4: note select (and record/preview)
	if row < 4 && col < 4 {
		noteIdx := row*4 + col
		if noteIdx < 16 {
			// Always select the note
			s.SelectedNoteIdx = noteIdx
			if s.Cursor >= pat.Notes[s.SelectedNoteIdx].Length {
				s.Cursor = pat.Notes[s.SelectedNoteIdx].Length - 1
			}

			// If preview mode, play the sound
			if s.Preview {
				select {
				case d.previewChan <- noteIdx:
				default:
				}
			}

			// If recording while playing, toggle step at current position
			if s.Recording && S.Playing {
				note := &pat.Notes[noteIdx]
				stepIdx := d.currentStep() % note.Length
				d.ToggleStep(noteIdx, stepIdx)
			}
		}
		return
	}

	// Bottom-right 4x4: command pads
	if row < 4 && col >= 4 {
		note := &pat.Notes[s.SelectedNoteIdx]
		switch {
		// Row 0: Clear Note, Clear Pattern, Copy, Paste
		case row == 0 && col == 4: // Clear Note
			d.confirmClearNote()
		case row == 0 && col == 5: // Clear Pattern
			d.confirmClearPattern()
		// Row 1: Nudge Left, Nudge Right, Length -, Length +
		case row == 1 && col == 6: // Length -
			if note.Length > 1 {
				newLen := note.Length - 1
				d.SetNoteLaneLength(s.SelectedNoteIdx, newLen)
				if s.Cursor >= newLen {
					s.Cursor = newLen - 1
				}
			}
		case row == 1 && col == 7: // Length +
			if note.Length < 32 {
				d.SetNoteLaneLength(s.SelectedNoteIdx, note.Length+1)
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
	noteColor := [3]uint8{148, 18, 126}
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

	// Bottom-left 4x4: note select
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			grid[row][col] = noteColor
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
	out += widgets.RenderLegendItem(noteColor, "Note", "select note 1-16 (plays sound in preview mode)") + "\n"
	out += widgets.RenderLegendItem(commandsColor, "Commands", "") + "\n"
	out += `    Row 3: [Preview] [Record]  (Mute)   (Solo)
    Row 2: (Vel -)   (Vel +)   (-)      (-)
    Row 1: (Nudge<)  (Nudge>)  [Len -]  [Len +]
    Row 0: [ClrNote] [ClrPat]  (Copy)   (Paste)
    [ ] = implemented, ( ) = not yet` + "\n"
	out += widgets.RenderLegendItem(sceneColor, "Scene", "launch scenes")

	return out
}
