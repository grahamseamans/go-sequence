package metropolix

import (
	"sort"

	"go-sequence/controller/devices/rng"
	"go-sequence/controller/surface"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Package metropolix is the stateless Metropolix-style melodic sequencer
// device: a compiler turning a Metropolix spec into TimedEvents, plus input
// handlers that mutate the spec. The spec in the model holds only
// human-editable data; all behavior (stage walk, accumulator evolution,
// probability rolls, slide pitch bends) lives here.
//
// Compile is the ONE place probability rolls happen — per DESIGN §7. Same
// (spec, seed) → same output. Fresh seed every loop → fresh rolls every loop.
//
// Callers (Compiler, InputManager) hold the Track's mutex; functions here
// never touch it themselves.

// --- Pages (Launchpad parameter pages, selected via scene-button column) ---

const (
	mxPageAccumulator = 0 // sub-pages: value, reset, mode
	mxPageProbability = 1
	mxPageGate        = 2 // rows 7-2 gate length, row 1 gate on/off, row 0 slide
	mxPageRatchets    = 3
	mxPagePulseCount  = 4
	mxPageNotes       = 5
	mxPageOctave      = 6
	mxPageSettings    = 7
)

// Accumulator sub-pages (navigated on row 8 cols 1 / 2).
const (
	mxAccumSubValue = 0
	mxAccumSubReset = 1
	mxAccumSubMode  = 2
)

// Accumulator modes.
const (
	mxAccumModeReset    = 0 // on reset: zero offset and count, direction = +1
	mxAccumModePingPong = 1 // on reset: flip direction, zero count
	mxAccumModeHold     = 2 // on reset: stop accumulating, hold last value
)

// gateLengthTicks is the gate hold duration per stage.GateLength index,
// expressed in 16th-note steps. Index 0 is "trigger" (immediate NoteOff);
// index 5 is "full legato". Multiplied by ticksPerStep at use.
var gateLengthTicks = [6]int64{0, 1, 2, 4, 8, 16}

// scaleIntervals are the scale-degree → semitone lookups. Index matches
// devices.ScaleType. These are ported verbatim from the old sequencer package.
var scaleIntervals = map[devices.ScaleType][]int{
	devices.ScaleChromatic:        {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
	devices.ScaleMajor:            {0, 2, 4, 5, 7, 9, 11, 12},
	devices.ScaleMinor:            {0, 2, 3, 5, 7, 8, 10, 12},
	devices.ScalePentatonic:       {0, 2, 4, 7, 9, 12, 14, 16},
	devices.ScaleDorian:           {0, 2, 3, 5, 7, 9, 10, 12},
	devices.ScalePhrygian:         {0, 1, 3, 5, 7, 8, 10, 12},
	devices.ScaleLydian:           {0, 2, 4, 6, 7, 9, 11, 12},
	devices.ScaleMixolydian:       {0, 2, 4, 5, 7, 9, 10, 12},
	devices.ScaleLocrian:          {0, 1, 3, 5, 6, 8, 10, 12},
	devices.ScaleHarmonicMinor:    {0, 2, 3, 5, 7, 8, 11, 12},
	devices.ScaleMelodicMinor:     {0, 2, 3, 5, 7, 9, 11, 12},
	devices.ScaleBlues:            {0, 3, 5, 6, 7, 10, 12, 15},
	devices.ScaleWholeTone:        {0, 2, 4, 6, 8, 10, 12},
	devices.ScaleDimHalfWhole:     {0, 1, 3, 4, 6, 7, 9, 10},
	devices.ScaleDimWholeHalf:     {0, 2, 3, 5, 6, 8, 9, 11},
	devices.ScaleHungarianMinor:   {0, 2, 3, 6, 7, 8, 11, 12},
	devices.ScaleDoubleHarmonic:   {0, 1, 4, 5, 7, 8, 11, 12},
	devices.ScalePhrygianDominant: {0, 1, 4, 5, 7, 8, 10, 12},
	devices.ScaleHirajoshi:        {0, 2, 3, 7, 8, 12, 14, 15},
	devices.ScaleInSen:            {0, 1, 5, 7, 10, 12, 13, 17},
	devices.ScaleYo:               {0, 2, 4, 7, 9, 12, 14, 16},
	devices.ScaleBhairavi:         {0, 1, 3, 5, 7, 8, 10, 12},
}

// Compile renders a single Metropolix pattern into a CompiledPattern: one
// full loop of TimedEvents. Pure function of (pattern, seed). The caller
// (Compiler goroutine) picks which pattern to render.
//
// Walk order depends on pat.Mode. Accumulator state is LOCAL to this call —
// the spec is never mutated. Per the Grok design review: accumulators start at
// zero each compile ("fresh rolls per loop" extends to accumulator drift).
//
// Probability is rolled ONCE per stage. If it fails, the stage emits nothing
// (no note, no ratchets). Ratchets within a gated stage all play — they share
// the stage's single probability outcome.
func Compile(pat *devices.MetropolixPattern, seed uint64) devices.CompiledPattern {
	prng := rng.NewRng(seed)

	if pat == nil {
		return devices.CompiledPattern{Length: 1}
	}

	// Clamp pat.Length defensively. Validate() normally guarantees 1..8.
	patLen := pat.Length
	if patLen < 1 {
		patLen = 1
	}
	if patLen > 8 {
		patLen = 8
	}

	// Local accumulator state — per-stage offset, trigger count, direction.
	// The model spec has no persisted Accum fields; state starts fresh at
	// each compile and never writes back to the spec.
	var accum, accumCount, accumDir [8]int
	for i := range accumDir {
		accumDir[i] = 1
	}

	ticksPerStep := int64(devices.PPQ / 4)

	// walk is the ordered list of stage indices this pattern iteration visits.
	// For Forward / Reverse / Random it has patLen entries; for Pendulum it
	// has 2*patLen-2 entries (or 1 when patLen == 1).
	walk := stageWalk(pat.Mode, patLen, prng)

	// Pre-compute each stage's pitch *after* accumulator evolution applies at
	// the end of the visit. We need the "current visit's" pitch for the NoteOn
	// and the "next visit's" pitch for slide bending — so we materialize a
	// per-visit pitch slice as we walk.
	events := make([]devices.TimedEvent, 0, len(walk)*4)
	currentTick := int64(0)

	// visitPitch[i] is the pitch emitted when visiting walk[i], computed BEFORE
	// the accumulator update for that stage runs. Built lazily alongside the
	// walk so slide bends can see the next visit's pitch.
	visitPitch := make([]int, len(walk))

	for i, stageIdx := range walk {
		stage := &pat.Stages[stageIdx]
		stageTicks := int64(stage.PulseCount) * ticksPerStep

		pitch := calculatePitch(pat, stageIdx, accum[stageIdx])
		visitPitch[i] = pitch

		// Probability roll: single roll per stage visit. Controls whether any
		// notes fire this visit — ratchets share the same outcome.
		fires := stage.Gate && prng.Probability(uint8(stage.Probability))

		if fires && stage.Ratchets > 0 {
			ratchets := stage.Ratchets
			ratchetInterval := stageTicks / int64(ratchets)
			if ratchetInterval < 1 {
				ratchetInterval = 1
			}

			gateTicks := gateLengthTicks[stage.GateLength] * ticksPerStep

			for r := 0; r < ratchets; r++ {
				onTick := currentTick + int64(r)*ratchetInterval

				events = append(events, devices.TimedEvent{
					Tick: onTick,
					Event: midi.Event{
						Type:     midi.NoteOn,
						Note:     uint8(pitch),
						Velocity: 100,
					},
				})

				var offTick int64
				if gateTicks == 0 {
					// Trigger mode: NoteOff immediately after NoteOn.
					offTick = onTick
				} else {
					// Clamp gate so it never overlaps the next ratchet or
					// overshoots the stage boundary.
					maxGate := ratchetInterval
					if r == ratchets-1 {
						maxGate = stageTicks - int64(r)*ratchetInterval
					}
					gt := gateTicks
					if gt > maxGate {
						gt = maxGate
					}
					offTick = onTick + gt
				}

				events = append(events, devices.TimedEvent{
					Tick: offTick,
					Event: midi.Event{
						Type: midi.NoteOff,
						Note: uint8(pitch),
					},
				})
			}
		}

		// Apply accumulator AFTER the stage fires (evolves for next visit).
		applyAccumulator(stage, stageIdx, &accum, &accumCount, &accumDir)

		currentTick += stageTicks
	}

	// Slide pitch bends. For each visit i where the stage has Slide set and
	// the following visit (i+1, wrapping) lands on a different pitch, emit
	// SlideTime steps of pitch-bend leading into the next stage, then reset
	// the bend to 0 at the new stage's NoteOn boundary.
	if pat.SlideTime > 0 {
		stageEndTicks := make([]int64, len(walk)+1)
		stageEndTicks[0] = 0
		for i, stageIdx := range walk {
			stageEndTicks[i+1] = stageEndTicks[i] + int64(pat.Stages[stageIdx].PulseCount)*ticksPerStep
		}
		for i, stageIdx := range walk {
			stage := &pat.Stages[stageIdx]
			if !stage.Slide {
				continue
			}
			// Next visit index (wrap within this compile's walk).
			next := (i + 1) % len(walk)
			startPitch := visitPitch[i]
			endPitch := visitPitch[next]
			if startPitch == endPitch {
				continue
			}

			slideStartTick := stageEndTicks[i+1]
			for k := 0; k < pat.SlideTime; k++ {
				progress := float64(k) / float64(pat.SlideTime)
				bendSemitones := float64(endPitch-startPitch) * progress
				events = append(events, devices.TimedEvent{
					Tick: slideStartTick + int64(k),
					Event: midi.Event{
						Type:      midi.PitchBend,
						BendValue: int16(bendSemitones * 4096),
					},
				})
			}
			// Reset bend at the arrival boundary.
			events = append(events, devices.TimedEvent{
				Tick: slideStartTick + int64(pat.SlideTime),
				Event: midi.Event{
					Type:      midi.PitchBend,
					BendValue: 0,
				},
			})
		}
	}

	// Total pattern length in ticks = sum of PulseCount*ticksPerStep over all
	// visits in the walk. For Pendulum this is longer than the non-pendulum
	// forms since the walk is 2L-2 stages.
	length := int64(0)
	for _, stageIdx := range walk {
		length += int64(pat.Stages[stageIdx].PulseCount) * ticksPerStep
	}
	if length < 1 {
		length = 1
	}

	// Clamp any events that landed past the pattern boundary (e.g. slide
	// bend reset after the final stage). Ticks must fit inside [0, length).
	for i := range events {
		if events[i].Tick >= length {
			events[i].Tick = length - 1
		}
		if events[i].Tick < 0 {
			events[i].Tick = 0
		}
	}

	// Stable sort: keeps NoteOn before NoteOff when they collide at t=0-length
	// tick and preserves PitchBend ordering within a slide ramp.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return devices.CompiledPattern{
		Events: events,
		Length: length,
	}
}

// stageWalk returns the sequence of stage indices visited in one loop of
// the pattern, per mode. Random mode consumes prng for each step.
func stageWalk(mode devices.PlaybackMode, patLen int, prng *rng.Rng) []int {
	switch mode {
	case devices.ModeReverse:
		walk := make([]int, patLen)
		for i := 0; i < patLen; i++ {
			walk[i] = patLen - 1 - i
		}
		return walk

	case devices.ModePendulum:
		if patLen <= 1 {
			return []int{0}
		}
		// 0, 1, ..., patLen-1, patLen-2, ..., 1 → length 2*patLen-2.
		walk := make([]int, 0, 2*patLen-2)
		for i := 0; i < patLen; i++ {
			walk = append(walk, i)
		}
		for i := patLen - 2; i >= 1; i-- {
			walk = append(walk, i)
		}
		return walk

	case devices.ModeRandom:
		walk := make([]int, patLen)
		for i := 0; i < patLen; i++ {
			walk[i] = prng.IntN(patLen)
		}
		return walk

	default: // ModeForward
		walk := make([]int, patLen)
		for i := 0; i < patLen; i++ {
			walk[i] = i
		}
		return walk
	}
}

// calculatePitch resolves a stage's MIDI note via the pattern's scale, root,
// and octave, adding the per-stage accumulator offset. Clamped to [0, 127].
func calculatePitch(pat *devices.MetropolixPattern, stageIdx int, accumOffset int) int {
	stage := &pat.Stages[stageIdx]
	scale := scaleIntervals[pat.Scale]
	if len(scale) == 0 {
		scale = scaleIntervals[devices.ScaleChromatic]
	}
	scaleLen := len(scale)

	noteIdx := stage.Note % scaleLen
	octaveShift := stage.Note / scaleLen
	basePitch := int(pat.RootNote) + scale[noteIdx] + octaveShift*12

	// Octave: 4 is the "middle" default; offset from 4.
	basePitch += (stage.Octave - 4) * 12

	basePitch += accumOffset

	if basePitch < 0 {
		basePitch = 0
	}
	if basePitch > 127 {
		basePitch = 127
	}
	return basePitch
}

// applyAccumulator evolves local accumulator state for one stage visit,
// mirroring the Accumulator / AccumReset / AccumMode semantics of the original
// sequencer. Does nothing when the stage's Accumulator increment is zero.
func applyAccumulator(stage *devices.MetropolixStage, stageIdx int, accum, accumCount, accumDir *[8]int) {
	if stage.Accumulator == 0 {
		return
	}

	if accumDir[stageIdx] == 0 {
		accumDir[stageIdx] = 1
	}

	accumCount[stageIdx]++

	if stage.AccumReset > 0 && accumCount[stageIdx] >= stage.AccumReset {
		switch stage.AccumMode {
		case mxAccumModeReset:
			accum[stageIdx] = 0
			accumCount[stageIdx] = 0
			accumDir[stageIdx] = 1
		case mxAccumModePingPong:
			accumDir[stageIdx] = -accumDir[stageIdx]
			accumCount[stageIdx] = 0
		case mxAccumModeHold:
			// Peg count at the limit so the reset branch keeps triggering
			// the "do nothing" path, and skip the accumulator update below.
			accumCount[stageIdx] = stage.AccumReset
			return
		}
	}

	accum[stageIdx] += stage.Accumulator * accumDir[stageIdx]
}

// --- Input handlers ---
//
// Signature note: these match drum.go — they take `*model.Track` (not
// `*model.Metropolix` directly) so future handlers can reach Track-level
// fields (PortName, Channel) for preview sends. Metropolix has no preview
// currently, so the track isn't used for MIDI output, but the uniform shape
// keeps the InputManager wiring simple.

// HandlePad dispatches a pad press/release to the page-specific handler.
// Releases (down == false) are no-ops.
func HandlePad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix

	// Top row (row 8): accumulator sub-page navigation (only on the accum page).
	if row == 8 {
		if spec.Page == mxPageAccumulator {
			if col == 1 && spec.AccumSubPage > 0 {
				spec.AccumSubPage--
			} else if col == 2 && spec.AccumSubPage < 2 {
				spec.AccumSubPage++
			}
		}
		return
	}

	// Right column (col 8): page select (scene buttons).
	if col == 8 {
		if row >= 0 && row <= 7 {
			spec.Page = row
		}
		return
	}

	// Main grid: page-specific handler.
	pat := spec.Patterns[spec.EditingPatternIdx]
	switch spec.Page {
	case mxPageSettings:
		handleSettingsPad(spec, pat, row, col)
	case mxPageOctave:
		if col < pat.Length {
			pat.Stages[col].Octave = row
		}
	case mxPageNotes:
		if col < pat.Length {
			pat.Stages[col].Note = row
		}
	case mxPagePulseCount:
		if col < pat.Length {
			pat.Stages[col].PulseCount = row + 1
		}
	case mxPageRatchets:
		if col < pat.Length {
			pat.Stages[col].Ratchets = row + 1
		}
	case mxPageProbability:
		if col < pat.Length {
			// Row 0..7 → 0..100%. Linear, saturating at 100.
			p := row * 100 / 7
			if p > 100 {
				p = 100
			}
			pat.Stages[col].Probability = p
		}
	case mxPageGate:
		if col < pat.Length {
			switch {
			case row >= 2 && row <= 7:
				pat.Stages[col].GateLength = row - 2
			case row == 1:
				pat.Stages[col].Gate = !pat.Stages[col].Gate
			case row == 0:
				pat.Stages[col].Slide = !pat.Stages[col].Slide
			}
		}
	case mxPageAccumulator:
		handleAccumulatorPad(spec, pat, row, col)
	}
}

// handleSettingsPad covers the PageSettings grid (mode, scale, length, root,
// slide time). Root row is 0..7 of a 12-tone octave; we keep the current
// octave and overwrite the chromatic index.
func handleSettingsPad(spec *devices.Metropolix, pat *devices.MetropolixPattern, row, col int) {
	switch row {
	case 7: // Mode
		if col >= 0 && col < 4 {
			pat.Mode = devices.PlaybackMode(col)
		}
	case 6: // Scale (only the first 4 scales accessible from this row)
		if col >= 0 && col < 4 {
			pat.Scale = devices.ScaleType(col)
		}
	case 5: // Length
		if col >= 0 && col < 8 {
			pat.Length = col + 1
			if spec.Selected >= pat.Length {
				spec.Selected = pat.Length - 1
			}
		}
	case 4: // Root note (within current octave)
		if col >= 0 && col < 8 {
			currentOctave := int(pat.RootNote) / 12
			root := currentOctave*12 + col
			if root < 0 {
				root = 0
			}
			if root > 127 {
				root = 127
			}
			pat.RootNote = uint8(root)
		}
	case 3: // Slide time
		if col >= 0 && col < 8 {
			pat.SlideTime = col + 1
		}
	}
}

// handleAccumulatorPad handles the three accumulator sub-pages: value,
// reset-count, and mode. Controlled via AccumSubPage (navigated on row 8).
func handleAccumulatorPad(spec *devices.Metropolix, pat *devices.MetropolixPattern, row, col int) {
	if col < 0 || col >= pat.Length {
		return
	}
	stage := &pat.Stages[col]

	switch spec.AccumSubPage {
	case mxAccumSubValue:
		// Rows 0..7 → -4..+3 (center at row 4 == 0).
		stage.Accumulator = row - 4
	case mxAccumSubReset:
		// Rows 0..7 → reset counts 0..7 (0 = never).
		stage.AccumReset = row
	case mxAccumSubMode:
		if row >= 0 && row < 3 {
			stage.AccumMode = row
		}
	}
}

// HandleKey mutates the editing stage / pattern via keyboard shortcuts.
// Mirrors the shortcuts of the old sequencer metropolix view.
func HandleKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]

	// Clamp selected defensively in case Length shrunk on load.
	if spec.Selected >= pat.Length {
		spec.Selected = pat.Length - 1
	}
	if spec.Selected < 0 {
		spec.Selected = 0
	}
	stage := &pat.Stages[spec.Selected]

	switch key {
	case "h", "left":
		if spec.Selected > 0 {
			spec.Selected--
		}
	case "l", "right":
		if spec.Selected < pat.Length-1 {
			spec.Selected++
		}
	case "j", "down":
		// Lower pitch by one scale degree, borrowing an octave when needed.
		if stage.Note > 0 {
			stage.Note--
		} else if stage.Octave > 0 {
			stage.Octave--
			stage.Note = 7
		}
	case "k", "up":
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
		stage.Probability -= 10
		if stage.Probability < 0 {
			stage.Probability = 0
		}
	case "P":
		stage.Probability += 10
		if stage.Probability > 100 {
			stage.Probability = 100
		}
	case "m":
		pat.Mode = (pat.Mode + 1) % 4
	case "[":
		if pat.Length > 1 {
			pat.Length--
			if spec.Selected >= pat.Length {
				spec.Selected = pat.Length - 1
			}
		}
	case "]":
		if pat.Length < 8 {
			pat.Length++
		}
	case "<", ",":
		if spec.EditingPatternIdx > 0 {
			spec.EditingPatternIdx--
		}
	case ">", ".":
		if spec.EditingPatternIdx < devices.NumPatterns-1 {
			spec.EditingPatternIdx++
		}
	case "q":
		pat.Scale = (pat.Scale + 1) % devices.ScaleCount
	case "z":
		if pat.RootNote > 0 {
			pat.RootNote--
		}
	case "x":
		if pat.RootNote < 127 {
			pat.RootNote++
		}
	}
}

// HandleMIDI currently ignores incoming MIDI. The old sequencer left a stub
// comment ("could record incoming notes to stages") — intentional drop until
// we design stage-recording semantics. Do NOT silently record here.
func HandleMIDI(track *model.Track, out midi.ToExternal, ev midi.Event) {
	// No-op: Metropolix does not record from MIDI input today.
	// TODO(stage-recording): define note → stage mapping when we add this.
}

// Render is a stub; step 5 will port the TUI view from sequencer/metropolix.go.
// TODO(step5): port rendering from sequencer/metropolix.go when view/ rewires.
func Render(track *model.Track, project *model.Project) string { return "" }

// RenderLEDs is a stub; step 5 will port the LED layout from sequencer/metropolix.go.
// TODO(step5): port rendering from sequencer/metropolix.go when view/ rewires.
func RenderLEDs(track *model.Track, project *model.Project) []surface.LED { return nil }
