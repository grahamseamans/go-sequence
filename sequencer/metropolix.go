package sequencer

import (
	"fmt"
	"math/rand"
	"sync"

	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/widgets"
)

// Scale definitions - intervals from root (semitones)
var scales = map[ScaleType][]int{
	ScaleChromatic:        {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
	ScaleMajor:            {0, 2, 4, 5, 7, 9, 11, 12},
	ScaleMinor:            {0, 2, 3, 5, 7, 8, 10, 12},
	ScalePentatonic:       {0, 2, 4, 7, 9, 12, 14, 16},
	ScaleDorian:           {0, 2, 3, 5, 7, 9, 10, 12},
	ScalePhrygian:         {0, 1, 3, 5, 7, 8, 10, 12},
	ScaleLydian:           {0, 2, 4, 6, 7, 9, 11, 12},
	ScaleMixolydian:       {0, 2, 4, 5, 7, 9, 10, 12},
	ScaleLocrian:          {0, 1, 3, 5, 6, 8, 10, 12},
	ScaleHarmonicMinor:    {0, 2, 3, 5, 7, 8, 11, 12},
	ScaleMelodicMinor:     {0, 2, 3, 5, 7, 9, 11, 12},
	ScaleBlues:            {0, 3, 5, 6, 7, 10, 12, 15},
	ScaleWholeTone:        {0, 2, 4, 6, 8, 10, 12},
	ScaleDimHalfWhole:     {0, 1, 3, 4, 6, 7, 9, 10},
	ScaleDimWholeHalf:     {0, 2, 3, 5, 6, 8, 9, 11},
	ScaleHungarianMinor:   {0, 2, 3, 6, 7, 8, 11, 12},
	ScaleDoubleHarmonic:   {0, 1, 4, 5, 7, 8, 11, 12},
	ScalePhrygianDominant: {0, 1, 4, 5, 7, 8, 10, 12},
	ScaleHirajoshi:        {0, 2, 3, 7, 8, 12, 14, 15},
	ScaleInSen:            {0, 1, 5, 7, 10, 12, 13, 17},
	ScaleYo:               {0, 2, 4, 7, 9, 12, 14, 16},
	ScaleBhairavi:         {0, 1, 3, 5, 7, 8, 10, 12},
}

var scaleNames = []string{
	"Chromatic", "Major", "Minor", "Pentatonic",
	"Dorian", "Phrygian", "Lydian", "Mixolydian", "Locrian",
	"Harm Min", "Mel Min", "Blues", "Whole Tone",
	"Dim H-W", "Dim W-H", "Hungarian", "Dbl Harm",
	"Phryg Dom", "Hirajoshi", "In Sen", "Yo", "Bhairavi",
}
var modeNames = []string{"Forward", "Reverse", "Pendulum", "Random"}

// Launchpad pages
const (
	PageAccumulator  = 0 // Has sub-pages: value, reset, mode
	PageProbability  = 1
	PageGate         = 2 // Gate length (rows 7-2), gate on/off (row 1), slide (row 0)
	PageRatchets     = 3
	PagePulseCount   = 4
	PageNotes        = 5
	PageOctave       = 6
	PageSettings     = 7
)

// Accumulator sub-pages (navigated with up/down arrows)
const (
	AccumSubValue = 0
	AccumSubReset = 1
	AccumSubMode  = 2
)

// Gate length values (as fraction of pulse)
var gateLengthValues = []float64{
	0.0,   // Trigger (immediate off)
	0.0625, // 1/16
	0.125,  // 1/8
	0.25,   // 1/4
	0.5,    // 1/2
	1.0,    // Full (legato)
}

var pageNames = []string{"Accum", "Prob", "Gate", "Ratchets", "Pulse", "Notes", "Octave", "Settings"}

// MetropolixDevice is a Metropolix-style melodic sequencer
type MetropolixDevice struct {
	state *MetropolixState

	// Queue-based playback - protected by queueMu (held ONLY during swap, not generation)
	queueMu          sync.RWMutex
	queue            []midi.Event // events sorted by tick
	queuedUntilTick  int64        // how far we've filled the queue
	patternStartTick int64        // tick when current pattern started
	onQueueChange    func()       // callback to wake manager when queue needs recalc

	// Pattern switching
	nextPatternTick int64 // tick when next pattern should start (-1 if none)

	// Confirmation dialog
	confirmMode   bool
	confirmMsg    string
	confirmAction func()
}

// NewMetropolixDevice creates a device that operates on the given state
func NewMetropolixDevice(state *MetropolixState) *MetropolixDevice {
	return &MetropolixDevice{
		state:           state,
		nextPatternTick: -1,
	}
}

// SetOnQueueChange sets the callback for when the queue needs recalculation
func (d *MetropolixDevice) SetOnQueueChange(fn func()) {
	d.onQueueChange = fn
}

// fauxPatternLength returns the length of one pass through all stages (in steps)
func (d *MetropolixDevice) fauxPatternLength(patternNum int) int {
	pat := &d.state.Patterns[patternNum]
	total := 0
	for i := 0; i < pat.Length; i++ {
		total += pat.Stages[i].PulseCount
	}
	return total
}

// fauxPatternTicks returns the faux pattern length in ticks
func (d *MetropolixDevice) fauxPatternTicks(patternNum int) int64 {
	return int64(d.fauxPatternLength(patternNum)) * (PPQ / 4)
}

// GeneratePattern generates all MIDI events for one faux cycle starting at startTick.
// Uses and updates device state (accumulators, stage position, etc.)
func (d *MetropolixDevice) GeneratePattern(patternNum int, startTick int64) []midi.Event {
	s := d.state
	pat := &s.Patterns[patternNum]
	ticksPerStep := int64(PPQ / 4)

	var events []midi.Event

	// Reset stage position for fresh faux cycle
	s.Stage = 0

	// Track current tick position
	currentTick := startTick

	// Process each stage
	for stageIdx := 0; stageIdx < pat.Length; stageIdx++ {
		stage := &pat.Stages[s.Stage]
		stageTicks := int64(stage.PulseCount) * ticksPerStep

		// Generate ratchets within this stage's time span
		if stage.Gate && stage.Ratchets > 0 {
			ratchetInterval := stageTicks / int64(stage.Ratchets)
			if ratchetInterval < 1 {
				ratchetInterval = 1
			}

			for r := 0; r < stage.Ratchets; r++ {
				// Probability check per ratchet
				if rand.Intn(100) >= stage.Probability {
					continue
				}

				ratchetTick := currentTick + int64(r)*ratchetInterval

				pitch := d.calculatePitch(s.Stage)
				events = append(events, midi.Event{
					Tick:     ratchetTick,
					Type:     midi.NoteOn,
					Note:     uint8(pitch),
					Velocity: 100,
				})

				// Note-off based on gate length
				gateLengths := []int64{0, 1, 2, 4, 8, 16}
				gt := gateLengths[stage.GateLength] * ticksPerStep
				if gt == 0 {
					// Trigger mode - immediate note-off
					events = append(events, midi.Event{
						Tick: ratchetTick,
						Type: midi.NoteOff,
						Note: uint8(pitch),
					})
				} else {
					// Clamp gate to not exceed next ratchet or stage end
					maxGate := ratchetInterval
					if r == stage.Ratchets-1 {
						maxGate = stageTicks - int64(r)*ratchetInterval
					}
					if gt > maxGate {
						gt = maxGate
					}
					events = append(events, midi.Event{
						Tick: ratchetTick + gt,
						Type: midi.NoteOff,
						Note: uint8(pitch),
					})
				}
			}
		}

		// Apply accumulator at end of stage
		d.applyAccumulator(s.Stage)

		// Slide pitch bend at stage transitions
		nextStage := d.nextStage()
		if stage.Slide && nextStage != s.Stage && pat.SlideTime > 0 {
			slideStartTick := currentTick + stageTicks
			startPitch := d.calculatePitch(s.Stage)
			endPitch := d.calculatePitch(nextStage)
			for i := 0; i < pat.SlideTime; i++ {
				progress := float64(i) / float64(pat.SlideTime)
				bendSemitones := float64(endPitch-startPitch) * progress
				events = append(events, midi.Event{
					Tick:      slideStartTick + int64(i),
					Type:      midi.PitchBend,
					BendValue: int16(bendSemitones * 4096),
				})
			}
			// Reset pitch bend at end
			events = append(events, midi.Event{
				Tick:      slideStartTick + int64(pat.SlideTime),
				Type:      midi.PitchBend,
				BendValue: 0,
			})
		}

		// Advance to next stage
		currentTick += stageTicks
		s.Stage = nextStage
	}

	return events
}

// Device interface implementation - queue-based

// FillUntil fills the event queue with events up to the given tick
func (d *MetropolixDevice) FillUntil(tick int64) {
	// Read current state
	d.queueMu.RLock()
	queuedUntil := d.queuedUntilTick
	patternStart := d.patternStartTick
	nextPatTick := d.nextPatternTick
	d.queueMu.RUnlock()

	if queuedUntil >= tick {
		return // already filled
	}

	// Generate events OUTSIDE the lock
	var newEvents []midi.Event
	currentPattern := d.state.Pattern
	for queuedUntil < tick {
		// Check for pattern switch at boundary
		if nextPatTick >= 0 && queuedUntil >= nextPatTick {
			d.state.Pattern = d.state.Next
			d.state.Next = -1
			currentPattern = d.state.Pattern
			patternStart = nextPatTick
			nextPatTick = -1
			d.state.ResetAccumulators()
		}

		events := d.GeneratePattern(currentPattern, queuedUntil)
		newEvents = append(newEvents, events...)
		queuedUntil += d.fauxPatternTicks(currentPattern)
	}

	// Swap in new events (brief lock)
	d.queueMu.Lock()
	d.queue = append(d.queue, newEvents...)
	d.queuedUntilTick = queuedUntil
	d.patternStartTick = patternStart
	d.nextPatternTick = nextPatTick
	d.queueMu.Unlock()
}

// PeekNextEvent returns the next event without removing it
func (d *MetropolixDevice) PeekNextEvent() *midi.Event {
	d.queueMu.RLock()
	defer d.queueMu.RUnlock()

	if len(d.queue) == 0 {
		return nil
	}
	return &d.queue[0]
}

// PopNextEvent removes and returns the next event
func (d *MetropolixDevice) PopNextEvent() *midi.Event {
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
func (d *MetropolixDevice) ClearQueue() {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()

	d.queue = nil
	d.queuedUntilTick = 0
	d.patternStartTick = 0
	d.nextPatternTick = -1
	d.state.ResetPlayback()
}

func (d *MetropolixDevice) calculatePitch(stageIdx int) int {
	s := d.state
	pat := &s.Patterns[s.Pattern]
	stage := &pat.Stages[stageIdx]

	scale := scales[pat.Scale]
	scaleLen := len(scale)

	// Base pitch from scale degree
	noteIdx := stage.Note % scaleLen
	octaveShift := stage.Note / scaleLen
	basePitch := int(pat.RootNote) + scale[noteIdx] + (octaveShift * 12)

	// Add octave offset (4 = middle, so offset from 4)
	basePitch += (stage.Octave - 4) * 12

	// Add accumulator
	basePitch += s.Accum[stageIdx]

	// Clamp to valid MIDI range
	if basePitch < 0 {
		basePitch = 0
	}
	if basePitch > 127 {
		basePitch = 127
	}

	return basePitch
}

func (d *MetropolixDevice) nextStage() int {
	s := d.state
	pat := &s.Patterns[s.Pattern]

	switch pat.Mode {
	case ModeForward:
		return (s.Stage + 1) % pat.Length
	case ModeReverse:
		next := s.Stage - 1
		if next < 0 {
			next = pat.Length - 1
		}
		return next
	case ModePendulum:
		next := s.Stage + s.Direction
		if next >= pat.Length {
			s.Direction = -1
			next = s.Stage - 1
			if next < 0 {
				next = 0
			}
		} else if next < 0 {
			s.Direction = 1
			next = 1
			if next >= pat.Length {
				next = 0
			}
		}
		return next
	case ModeRandom:
		return rand.Intn(pat.Length)
	default:
		return (s.Stage + 1) % pat.Length
	}
}

// applyAccumulator handles the accumulator logic with reset and mode
func (d *MetropolixDevice) applyAccumulator(stageIdx int) {
	s := d.state
	pat := &s.Patterns[s.Pattern]
	stage := &pat.Stages[stageIdx]

	// Skip if no accumulator set
	if stage.Accumulator == 0 {
		return
	}

	// Initialize direction if not set
	if s.AccumDir[stageIdx] == 0 {
		s.AccumDir[stageIdx] = 1
	}

	// Increment trigger count for this stage
	s.AccumCount[stageIdx]++

	// Check for reset
	if stage.AccumReset > 0 && s.AccumCount[stageIdx] >= stage.AccumReset {
		switch stage.AccumMode {
		case 0: // Reset - reset to zero
			s.Accum[stageIdx] = 0
			s.AccumCount[stageIdx] = 0
			s.AccumDir[stageIdx] = 1 // Reset direction too
		case 1: // Ping-pong - reverse direction
			s.AccumDir[stageIdx] = -s.AccumDir[stageIdx]
			s.AccumCount[stageIdx] = 0
		case 2: // Hold - stop accumulating, keep value
			s.AccumCount[stageIdx] = stage.AccumReset // Keep at limit so it stays held
			return                                    // Don't apply accumulator
		}
	}

	// Apply the accumulator with direction
	s.Accum[stageIdx] += stage.Accumulator * s.AccumDir[stageIdx]
}

// QueuePattern queues a pattern change at the next faux boundary after atTick
func (d *MetropolixDevice) QueuePattern(p int, atTick int64) {
	if p < 0 || p >= NumPatterns {
		return
	}
	d.state.Next = p

	// Use faux pattern length for musical switching
	patternTicks := d.fauxPatternTicks(d.state.Pattern)

	// Read state under lock
	d.queueMu.RLock()
	patternStart := d.patternStartTick
	queuedUntil := d.queuedUntilTick
	d.queueMu.RUnlock()

	// Calculate when the next pattern boundary occurs
	ticksSinceStart := atTick - patternStart
	ticksIntoPattern := ticksSinceStart % patternTicks
	ticksToNextBoundary := patternTicks - ticksIntoPattern
	boundaryTick := atTick + ticksToNextBoundary

	needsNotify := false

	// If we've already queued past the boundary, wipe those events
	if queuedUntil > boundaryTick {
		d.queueMu.Lock()
		newQueue := d.queue[:0]
		for _, e := range d.queue {
			if e.Tick < boundaryTick {
				newQueue = append(newQueue, e)
			}
		}
		d.queue = newQueue
		d.queuedUntilTick = boundaryTick
		d.nextPatternTick = boundaryTick
		d.queueMu.Unlock()
		needsNotify = true
	} else {
		d.queueMu.Lock()
		d.nextPatternTick = boundaryTick
		d.queueMu.Unlock()
	}

	// Wake manager outside the lock
	if needsNotify && d.onQueueChange != nil {
		d.onQueueChange()
	}
}

// regeneratePatternInQueue replaces events for the current pattern in queue.
// Called from UI thread - generates events WITHOUT holding lock, then swaps.
func (d *MetropolixDevice) regeneratePatternInQueue(patternNum int) {
	if patternNum != d.state.Pattern {
		return
	}

	patternTicks := d.fauxPatternTicks(patternNum)

	// --- Read current state (brief lock) ---
	d.queueMu.RLock()
	oldQueue := d.queue
	oldQueuedUntil := d.queuedUntilTick
	patternStart := d.patternStartTick
	d.queueMu.RUnlock()

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
		events := d.GeneratePattern(d.state.Pattern, newQueuedUntil)
		newQueue = append(newQueue, events...)
		newQueuedUntil += patternTicks
	}

	// --- Swap in new queue (brief lock) ---
	d.queueMu.Lock()
	d.queue = newQueue
	d.queuedUntilTick = newQueuedUntil
	d.queueMu.Unlock()

	// --- Wake dispatch loop to recalculate next event ---
	if d.onQueueChange != nil {
		d.onQueueChange()
	}
}

// CurrentPattern returns the currently playing pattern
func (d *MetropolixDevice) CurrentPattern() int {
	return d.state.Pattern
}

// NextPattern returns the queued pattern (-1 if none)
func (d *MetropolixDevice) NextPattern() int {
	if d.nextPatternTick >= 0 {
		return d.state.Next
	}
	return -1
}

func (d *MetropolixDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i := range d.state.Patterns {
		pat := &d.state.Patterns[i]
		// Consider pattern has content if any stage differs from default
		for s := 0; s < pat.Length; s++ {
			stage := &pat.Stages[s]
			if !stage.Gate || stage.Ratchets != 1 || stage.PulseCount != 1 ||
				stage.Slide || stage.Accumulator != 0 || stage.Probability != 100 {
				mask[i] = true
				break
			}
		}
	}
	return mask
}

func (d *MetropolixDevice) HandleMIDI(event midi.Event) {
	// Could record incoming notes to stages
}

func (d *MetropolixDevice) ToggleRecording() {}
func (d *MetropolixDevice) TogglePreview()   {}
func (d *MetropolixDevice) IsRecording() bool { return false }
func (d *MetropolixDevice) IsPreviewing() bool { return false }

func (d *MetropolixDevice) View() string {
	s := d.state
	pat := &s.Patterns[s.Editing]

	// Header
	playInfo := ""
	if s.Editing != s.Pattern {
		playInfo = fmt.Sprintf(" (playing:%d)", s.Pattern+1)
	}
	out := fmt.Sprintf("METROPOLIX  Pattern %d%s  Stage %d/%d  Mode: %s\n\n",
		s.Editing+1, playInfo, s.Stage+1, pat.Length, modeNames[pat.Mode])

	// Confirmation dialog
	if d.confirmMode {
		out += "─────────────────────────────────────────────────────────────────────\n"
		out += fmt.Sprintf("\n%s\n\n", d.confirmMsg)
		out += "  [y] Yes    [n] No\n"
		out += "\n─────────────────────────────────────────────────────────────────────\n"
		return out
	}

	// Stage grid
	out += "     "
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			out += fmt.Sprintf("  %d   ", i+1)
		} else {
			out += "      "
		}
	}
	out += "\n"

	out += "   ┌"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			out += "─────"
		} else {
			out += "     "
		}
		if i < 7 && i < pat.Length-1 {
			out += "┬"
		}
	}
	out += "┐\n"

	// Pitch row
	out += "   │"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			pitch := d.calculatePitch(i)
			noteName := d.pitchToName(pitch)
			if i == s.Stage {
				out += fmt.Sprintf(">%3s<│", noteName)
			} else if i == s.Selected {
				out += fmt.Sprintf("[%3s]│", noteName)
			} else {
				out += fmt.Sprintf(" %3s │", noteName)
			}
		} else {
			out += "     "
			_ = i // Mark as intentionally unused
		}
	}
	out += " Pitch\n"

	// Gate row
	out += "   │"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			stage := &pat.Stages[i]
			gateChar := "○"
			if stage.Gate {
				gateChar = "●"
			}
			out += fmt.Sprintf("  %s  │", gateChar)
		}
	}
	out += " Gate\n"

	// Ratchets row
	out += "   │"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			stage := &pat.Stages[i]
			out += fmt.Sprintf("  %d  │", stage.Ratchets)
		}
	}
	out += " Ratchets\n"

	// Slide row
	out += "   │"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			stage := &pat.Stages[i]
			slideChar := " "
			if stage.Slide {
				slideChar = "~"
			}
			out += fmt.Sprintf("  %s  │", slideChar)
		}
	}
	out += " Slide\n"

	// Accumulator row
	out += "   │"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			stage := &pat.Stages[i]
			if stage.Accumulator == 0 {
				out += "     │"
			} else if stage.Accumulator > 0 {
				out += fmt.Sprintf(" +%d  │", stage.Accumulator)
			} else {
				out += fmt.Sprintf(" %d  │", stage.Accumulator)
			}
		}
	}
	out += " Accum\n"

	out += "   └"
	for i := 0; i < 8; i++ {
		if i < pat.Length {
			out += "─────"
		} else {
			out += "     "
		}
		if i < 7 && i < pat.Length-1 {
			out += "┴"
		}
	}
	out += "┘\n"

	// Global settings
	out += fmt.Sprintf("\nLength: %d  Scale: %s  Root: %s  SlideTime: %d\n",
		pat.Length, scaleNames[pat.Scale], d.pitchToName(int(pat.RootNote)), pat.SlideTime)

	// Key help
	out += "\n"
	out += widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: "h / l", Desc: "select stage"},
			{Key: "j / k", Desc: "adjust pitch"},
			{Key: "space", Desc: "toggle gate"},
			{Key: "r / R", Desc: "ratchets -/+"},
			{Key: "s", Desc: "toggle slide"},
			{Key: "a / A", Desc: "accumulator -/+"},
			{Key: "p / P", Desc: "probability -/+"},
			{Key: "m", Desc: "cycle mode"},
			{Key: "q", Desc: "cycle scale"},
			{Key: "z / x", Desc: "root note -/+"},
			{Key: "[ / ]", Desc: "length -/+"},
			{Key: "< / >", Desc: "prev/next pattern"},
		}},
	})

	// Launchpad help
	out += "\n\n"
	out += d.renderLaunchpadHelp()

	return out
}

func (d *MetropolixDevice) pitchToName(pitch int) string {
	notes := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	octave := pitch/12 - 1
	note := pitch % 12
	return fmt.Sprintf("%s%d", notes[note], octave)
}

func (d *MetropolixDevice) RenderLEDs() []LEDState {
	var leds []LEDState
	s := d.state
	pat := &s.Patterns[s.Editing]

	// Colors
	playheadColor := [3]uint8{255, 255, 255}   // White for playhead
	pageActiveColor := [3]uint8{200, 100, 255} // Purple for active page
	pageDimColor := [3]uint8{50, 25, 60}       // Dim purple

	// Scene buttons (right column) - page selection
	for row := 0; row < 8; row++ {
		color := pageDimColor
		if row == s.Page {
			color = pageActiveColor
		}
		leds = append(leds, LEDState{Row: row, Col: 8, Color: color, Channel: midi.ChannelStatic})
	}

	// Main grid depends on current page
	switch s.Page {
	case PageSettings:
		leds = append(leds, d.renderSettingsPage()...)
	case PageOctave:
		leds = append(leds, d.renderValuePage(func(stage *MetropolixStageState) int { return stage.Octave }, 8)...)
	case PageNotes:
		leds = append(leds, d.renderValuePage(func(stage *MetropolixStageState) int { return stage.Note }, 8)...)
	case PagePulseCount:
		leds = append(leds, d.renderValuePage(func(stage *MetropolixStageState) int { return stage.PulseCount - 1 }, 8)...)
	case PageRatchets:
		leds = append(leds, d.renderValuePage(func(stage *MetropolixStageState) int { return stage.Ratchets - 1 }, 8)...)
	case PageProbability:
		leds = append(leds, d.renderValuePage(func(stage *MetropolixStageState) int { return stage.Probability / 13 }, 8)...)
	case PageGate:
		leds = append(leds, d.renderGatePage()...)
	case PageAccumulator:
		leds = append(leds, d.renderAccumulatorPage()...)
	}

	// Playhead indicator - pulse the current stage column
	for row := 0; row < 8; row++ {
		if s.Stage < pat.Length {
			key := [2]int{row, s.Stage}
			// Find existing LED at this position and make it pulse
			found := false
			for i, led := range leds {
				if led.Row == key[0] && led.Col == key[1] {
					leds[i].Channel = midi.ChannelPulse
					found = true
					break
				}
			}
			if !found {
				leds = append(leds, LEDState{Row: row, Col: s.Stage, Color: playheadColor, Channel: midi.ChannelPulse})
			}
		}
	}

	return leds
}

func (d *MetropolixDevice) renderValuePage(getValue func(*MetropolixStageState) int, maxVal int) []LEDState {
	var leds []LEDState
	s := d.state
	pat := &s.Patterns[s.Editing]

	activeColor := [3]uint8{255, 100, 50}
	dimColor := [3]uint8{50, 30, 20}
	offColor := [3]uint8{0, 0, 0}

	for col := 0; col < 8; col++ {
		if col >= pat.Length {
			// Inactive stages
			for row := 0; row < 8; row++ {
				leds = append(leds, LEDState{Row: row, Col: col, Color: offColor, Channel: midi.ChannelStatic})
			}
			continue
		}

		stage := &pat.Stages[col]
		value := getValue(stage)

		for row := 0; row < 8; row++ {
			// Row 7 = value 7 (high), row 0 = value 0 (low)
			rowValue := row
			var color [3]uint8
			if rowValue == value {
				color = activeColor
			} else {
				color = dimColor
			}
			leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
		}
	}

	return leds
}

func (d *MetropolixDevice) renderGatePage() []LEDState {
	var leds []LEDState
	s := d.state
	pat := &s.Patterns[s.Editing]

	lengthActiveColor := [3]uint8{255, 100, 50}
	lengthDimColor := [3]uint8{50, 30, 20}
	gateOnColor := [3]uint8{0, 255, 0}
	gateOffColor := [3]uint8{30, 50, 30}
	slideOnColor := [3]uint8{0, 150, 255}
	slideOffColor := [3]uint8{20, 40, 50}
	offColor := [3]uint8{0, 0, 0}

	for col := 0; col < 8; col++ {
		if col >= pat.Length {
			for row := 0; row < 8; row++ {
				leds = append(leds, LEDState{Row: row, Col: col, Color: offColor, Channel: midi.ChannelStatic})
			}
			continue
		}

		stage := &pat.Stages[col]

		// Rows 7-2: Gate length (6 values)
		// Row 7 = index 5 (full), Row 2 = index 0 (trigger)
		for row := 7; row >= 2; row-- {
			lengthIdx := row - 2 // row 7 -> 5, row 2 -> 0
			var color [3]uint8
			if stage.GateLength == lengthIdx {
				color = lengthActiveColor
			} else {
				color = lengthDimColor
			}
			leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
		}

		// Row 1 = Gate on/off
		gateColor := gateOffColor
		if stage.Gate {
			gateColor = gateOnColor
		}
		leds = append(leds, LEDState{Row: 1, Col: col, Color: gateColor, Channel: midi.ChannelStatic})

		// Row 0 = Slide on/off
		slideColor := slideOffColor
		if stage.Slide {
			slideColor = slideOnColor
		}
		leds = append(leds, LEDState{Row: 0, Col: col, Color: slideColor, Channel: midi.ChannelStatic})
	}

	return leds
}

func (d *MetropolixDevice) renderAccumulatorPage() []LEDState {
	var leds []LEDState
	s := d.state
	pat := &s.Patterns[s.Editing]

	activeColor := [3]uint8{255, 100, 50}
	dimColor := [3]uint8{50, 30, 20}
	offColor := [3]uint8{0, 0, 0}
	centerColor := [3]uint8{100, 100, 100}

	// Top row: show up/down arrows on cols 1 and 2
	upColor := dimColor
	downColor := dimColor
	if s.AccumSubPage > 0 {
		upColor = activeColor // Can go up (to previous sub-page)
	}
	if s.AccumSubPage < 2 {
		downColor = activeColor // Can go down (to next sub-page)
	}
	leds = append(leds, LEDState{Row: 8, Col: 0, Color: offColor, Channel: midi.ChannelStatic})
	leds = append(leds, LEDState{Row: 8, Col: 1, Color: upColor, Channel: midi.ChannelStatic})
	leds = append(leds, LEDState{Row: 8, Col: 2, Color: downColor, Channel: midi.ChannelStatic})
	for col := 3; col < 8; col++ {
		leds = append(leds, LEDState{Row: 8, Col: col, Color: offColor, Channel: midi.ChannelStatic})
	}

	// Render based on sub-page
	switch s.AccumSubPage {
	case AccumSubValue:
		// Accumulator value: -4 to +3 (rows 0-7)
		for col := 0; col < 8; col++ {
			if col >= pat.Length {
				for row := 0; row < 8; row++ {
					leds = append(leds, LEDState{Row: row, Col: col, Color: offColor, Channel: midi.ChannelStatic})
				}
				continue
			}

			stage := &pat.Stages[col]
			// Accumulator range: -4 to +3
			// Map to rows: row 0 = -4, row 4 = 0, row 7 = +3
			value := stage.Accumulator + 4 // Now 0-7

			for row := 0; row < 8; row++ {
				var color [3]uint8
				if row == value {
					color = activeColor
				} else if row == 4 { // Center (0)
					color = centerColor
				} else {
					color = dimColor
				}
				leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
			}
		}

	case AccumSubReset:
		// Reset count: 0-8 (0 = never, 1-8 = reset after N triggers)
		// Use rows 0-7 for values 0-7, row 7 = 8
		for col := 0; col < 8; col++ {
			if col >= pat.Length {
				for row := 0; row < 8; row++ {
					leds = append(leds, LEDState{Row: row, Col: col, Color: offColor, Channel: midi.ChannelStatic})
				}
				continue
			}

			stage := &pat.Stages[col]
			// Row 0 = never (0), rows 1-7 = reset after 1-7, row 7 can also be 8
			value := stage.AccumReset
			if value > 7 {
				value = 7
			}

			for row := 0; row < 8; row++ {
				var color [3]uint8
				if row == value {
					color = activeColor
				} else if row == 0 { // "Never" position
					color = centerColor
				} else {
					color = dimColor
				}
				leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
			}
		}

	case AccumSubMode:
		// Mode: 0=reset, 1=ping-pong, 2=hold (only 3 values, use bottom 3 rows)
		modeColors := [][3]uint8{
			{0, 255, 0},   // Reset = green
			{255, 200, 0}, // Ping-pong = yellow
			{255, 0, 0},   // Hold = red
		}
		for col := 0; col < 8; col++ {
			if col >= pat.Length {
				for row := 0; row < 8; row++ {
					leds = append(leds, LEDState{Row: row, Col: col, Color: offColor, Channel: midi.ChannelStatic})
				}
				continue
			}

			stage := &pat.Stages[col]

			for row := 0; row < 8; row++ {
				var color [3]uint8
				if row < 3 {
					// Rows 0-2 are the mode options
					if row == stage.AccumMode {
						color = modeColors[row]
					} else {
						// Dim version of mode color
						color = [3]uint8{modeColors[row][0] / 5, modeColors[row][1] / 5, modeColors[row][2] / 5}
					}
				} else {
					color = offColor
				}
				leds = append(leds, LEDState{Row: row, Col: col, Color: color, Channel: midi.ChannelStatic})
			}
		}
	}

	return leds
}

func (d *MetropolixDevice) renderSettingsPage() []LEDState {
	var leds []LEDState
	s := d.state
	pat := &s.Patterns[s.Editing]

	activeColor := [3]uint8{255, 100, 50}
	dimColor := [3]uint8{50, 30, 20}
	offColor := [3]uint8{0, 0, 0}

	// Row 7: Mode (columns 0-3)
	for col := 0; col < 4; col++ {
		color := dimColor
		if col == int(pat.Mode) {
			color = activeColor
		}
		leds = append(leds, LEDState{Row: 7, Col: col, Color: color, Channel: midi.ChannelStatic})
	}
	for col := 4; col < 8; col++ {
		leds = append(leds, LEDState{Row: 7, Col: col, Color: offColor, Channel: midi.ChannelStatic})
	}

	// Row 6: Scale (columns 0-3)
	for col := 0; col < 4; col++ {
		color := dimColor
		if col == int(pat.Scale) {
			color = activeColor
		}
		leds = append(leds, LEDState{Row: 6, Col: col, Color: color, Channel: midi.ChannelStatic})
	}
	for col := 4; col < 8; col++ {
		leds = append(leds, LEDState{Row: 6, Col: col, Color: offColor, Channel: midi.ChannelStatic})
	}

	// Row 5: Length (1-8)
	for col := 0; col < 8; col++ {
		color := dimColor
		if col < pat.Length {
			color = activeColor
		}
		leds = append(leds, LEDState{Row: 5, Col: col, Color: color, Channel: midi.ChannelStatic})
	}

	// Row 4: Root note (C through B, 12 notes but only 8 columns)
	rootNote := int(pat.RootNote) % 12
	for col := 0; col < 8; col++ {
		color := dimColor
		if col == rootNote%8 {
			color = activeColor
		}
		leds = append(leds, LEDState{Row: 4, Col: col, Color: color, Channel: midi.ChannelStatic})
	}

	// Row 3: Slide time (1-8)
	for col := 0; col < 8; col++ {
		color := dimColor
		if col < pat.SlideTime {
			color = activeColor
		}
		leds = append(leds, LEDState{Row: 3, Col: col, Color: color, Channel: midi.ChannelStatic})
	}

	// Rows 0-2: Reserved
	for row := 0; row < 3; row++ {
		for col := 0; col < 8; col++ {
			leds = append(leds, LEDState{Row: row, Col: col, Color: offColor, Channel: midi.ChannelStatic})
		}
	}

	return leds
}

func (d *MetropolixDevice) HandleKey(key string) {
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
	stage := &pat.Stages[s.Selected]

	switch key {
	case "h", "left":
		if s.Selected > 0 {
			s.Selected--
		}
	case "l", "right":
		if s.Selected < pat.Length-1 {
			s.Selected++
		}
	case "j", "down":
		// Lower pitch
		if stage.Note > 0 {
			stage.Note--
		} else if stage.Octave > 0 {
			stage.Octave--
			stage.Note = 7
		}
	case "k", "up":
		// Higher pitch
		if stage.Note < 7 {
			stage.Note++
		} else if stage.Octave < 7 {
			stage.Octave++
			stage.Note = 0
		}
	case " ":
		stage.Gate = !stage.Gate
	case "r":
		if stage.Ratchets > 1 {
			stage.Ratchets--
		}
	case "R":
		if stage.Ratchets < 8 {
			stage.Ratchets++
		}
	case "s":
		stage.Slide = !stage.Slide
	case "a":
		if stage.Accumulator > -4 {
			stage.Accumulator--
		}
	case "A":
		if stage.Accumulator < 3 {
			stage.Accumulator++
		}
	case "p":
		if stage.Probability > 0 {
			stage.Probability -= 10
			if stage.Probability < 0 {
				stage.Probability = 0
			}
		}
	case "P":
		if stage.Probability < 100 {
			stage.Probability += 10
			if stage.Probability > 100 {
				stage.Probability = 100
			}
		}
	case "m":
		pat.Mode = (pat.Mode + 1) % 4
	case "[":
		if pat.Length > 1 {
			pat.Length--
			if s.Selected >= pat.Length {
				s.Selected = pat.Length - 1
			}
		}
	case "]":
		if pat.Length < 8 {
			pat.Length++
		}
	case "<", ",":
		if s.Editing > 0 {
			s.Editing--
		}
	case ">", ".":
		if s.Editing < NumPatterns-1 {
			s.Editing++
		}
	case "c":
		d.confirmClearPattern()
	case "q":
		// Cycle scale forward (wraps)
		pat.Scale = (pat.Scale + 1) % ScaleCount
		d.regeneratePatternInQueue(s.Editing)
	case "z":
		// Root note down
		if pat.RootNote > 0 {
			pat.RootNote--
		}
		d.regeneratePatternInQueue(s.Editing)
	case "x":
		// Root note up
		if pat.RootNote < 127 {
			pat.RootNote++
		}
		d.regeneratePatternInQueue(s.Editing)
	}
}

func (d *MetropolixDevice) confirmClearPattern() {
	s := d.state

	d.confirmMsg = fmt.Sprintf("Clear pattern %d?", s.Editing+1)
	d.confirmAction = func() {
		pat := &s.Patterns[s.Editing]
		pat.Length = 8
		pat.Mode = ModeForward
		pat.Scale = ScaleMajor
		pat.RootNote = 60
		pat.SlideTime = 3
		for i := 0; i < 8; i++ {
			pat.Stages[i] = MetropolixStageState{
				Octave:      4,
				Note:        i % 8,
				Gate:        true,
				PulseCount:  1,
				Ratchets:    1,
				Probability: 100,
				Slide:       false,
				Accumulator: 0,
			}
		}
	}
	d.confirmMode = true
}

func (d *MetropolixDevice) HandlePad(row, col int) {
	s := d.state
	pat := &s.Patterns[s.Editing]

	debug.Log("metro", "HandlePad row=%d col=%d page=%d", row, col, s.Page)

	// Top row (row 8) - handle up/down arrows for accumulator sub-pages
	if row == 8 {
		if s.Page == PageAccumulator {
			if col == 1 && s.AccumSubPage > 0 {
				// Up arrow - go to previous sub-page
				s.AccumSubPage--
				debug.Log("metro", "Accum sub-page up to %d", s.AccumSubPage)
			} else if col == 2 && s.AccumSubPage < 2 {
				// Down arrow - go to next sub-page
				s.AccumSubPage++
				debug.Log("metro", "Accum sub-page down to %d", s.AccumSubPage)
			}
		}
		return
	}

	// Scene buttons (col 8) - page selection
	if col == 8 {
		debug.Log("metro", "Scene button pressed, setting page to %d", row)
		s.Page = row
		return
	}

	// Handle based on current page
	switch s.Page {
	case PageSettings:
		d.handleSettingsPad(row, col)
	case PageOctave:
		if col < pat.Length {
			pat.Stages[col].Octave = row
		}
	case PageNotes:
		if col < pat.Length {
			pat.Stages[col].Note = row
		}
	case PagePulseCount:
		if col < pat.Length {
			pat.Stages[col].PulseCount = row + 1
		}
	case PageRatchets:
		if col < pat.Length {
			pat.Stages[col].Ratchets = row + 1
		}
	case PageProbability:
		if col < pat.Length {
			// 8 levels: 0, 14, 28, 42, 57, 71, 85, 100
			pat.Stages[col].Probability = row * 100 / 7
		}
	case PageGate:
		if col < pat.Length {
			if row >= 2 && row <= 7 {
				// Gate length: row 7 = index 5 (full), row 2 = index 0 (trigger)
				pat.Stages[col].GateLength = row - 2
			} else if row == 1 {
				pat.Stages[col].Gate = !pat.Stages[col].Gate
			} else if row == 0 {
				pat.Stages[col].Slide = !pat.Stages[col].Slide
			}
		}
	case PageAccumulator:
		d.handleAccumulatorPad(row, col)
	}
}

func (d *MetropolixDevice) handleSettingsPad(row, col int) {
	s := d.state
	pat := &s.Patterns[s.Editing]

	switch row {
	case 7: // Mode
		if col < 4 {
			pat.Mode = PlaybackMode(col)
		}
	case 6: // Scale
		if col < 4 {
			pat.Scale = ScaleType(col)
		}
	case 5: // Length
		pat.Length = col + 1
		if s.Selected >= pat.Length {
			s.Selected = pat.Length - 1
		}
	case 4: // Root note
		// Adjust within current octave
		currentOctave := int(pat.RootNote) / 12
		pat.RootNote = uint8(currentOctave*12 + col)
	case 3: // Slide time
		pat.SlideTime = col + 1
	}
}

func (d *MetropolixDevice) handleAccumulatorPad(row, col int) {
	s := d.state
	pat := &s.Patterns[s.Editing]

	if col >= pat.Length {
		return
	}

	stage := &pat.Stages[col]

	switch s.AccumSubPage {
	case AccumSubValue:
		// Accumulator value: row 0 = -4, row 4 = 0, row 7 = +3
		stage.Accumulator = row - 4
	case AccumSubReset:
		// Reset count: row 0 = never, rows 1-7 = reset after 1-7 triggers
		stage.AccumReset = row
	case AccumSubMode:
		// Mode: row 0 = reset, row 1 = ping-pong, row 2 = hold
		if row < 3 {
			stage.AccumMode = row
		}
	}
}

func (d *MetropolixDevice) renderLaunchpadHelp() string {
	pageColor := [3]uint8{200, 100, 255}
	gridColor := [3]uint8{255, 100, 50}
	offColor := [3]uint8{30, 30, 30}

	var grid [8][8][3]uint8
	var rightCol [8][3]uint8

	// Fill grid with active color
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = gridColor
		}
	}

	// Right column - pages
	for row := 0; row < 8; row++ {
		rightCol[row] = pageColor
	}

	topRow := make([][3]uint8, 8)
	for i := range topRow {
		topRow[i] = offColor
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, &rightCol) + "\n\n"

	out += widgets.RenderLegendItem(pageColor, "Pages", "select parameter page") + "\n"
	out += `    Scene 7 → Settings (mode, scale, length, root, slide time)
    Scene 6 → Octave (0-7 per stage)
    Scene 5 → Notes (scale degree 0-7 per stage)
    Scene 4 → Pulse Count (1-8 per stage)
    Scene 3 → Ratchets (1-8 per stage)
    Scene 2 → Gate (rows 7-2: length, row 1: on/off, row 0: slide)
    Scene 1 → Probability (0-100% per stage)
    Scene 0 → Accumulator (sub-pages: value/reset/mode via top row)` + "\n"
	out += widgets.RenderLegendItem(gridColor, "Grid", "8 columns = 8 stages, 8 rows = values")

	return out
}
