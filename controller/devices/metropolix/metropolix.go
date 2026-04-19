package metropolix

import (
	"sort"

	"go-sequence/controller/devices/rng"
	"go-sequence/midi"
	"go-sequence/model/devices"
)

// Package metropolix is the stateless Metropolix-style melodic sequencer
// device compiler. It turns a Metropolix spec + seed into a CompiledPattern
// (one full loop of TimedEvents). UI logic lives under
// view/{launchpad,tui,keyboard}/metropolix.go.
//
// Compile is the ONE place probability rolls happen — per DESIGN §7. Same
// (spec, seed) → same output. Fresh seed every loop → fresh rolls every loop.
//
// Callers (Compiler goroutine) hold the Track's mutex; functions here
// never touch it themselves.

// Accumulator modes (mirror view/launchpad/metropolix constants).
const (
	mxAccumModeReset    = 0
	mxAccumModePingPong = 1
	mxAccumModeHold     = 2
)

// gateLengthTicks is the gate hold duration per stage.GateLength index,
// expressed in 16th-note steps. Index 0 is "trigger" (immediate NoteOff);
// index 5 is "full legato". Multiplied by ticksPerStep at use.
var gateLengthTicks = [6]int64{0, 1, 2, 4, 8, 16}

// scaleIntervals are the scale-degree → semitone lookups. Index matches
// devices.ScaleType.
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
	var accum, accumCount, accumDir [8]int
	for i := range accumDir {
		accumDir[i] = 1
	}

	ticksPerStep := int64(devices.PPQ / 4)

	walk := stageWalk(pat.Mode, patLen, prng)

	events := make([]devices.TimedEvent, 0, len(walk)*4)
	currentTick := int64(0)

	visitPitch := make([]int, len(walk))

	for i, stageIdx := range walk {
		stage := &pat.Stages[stageIdx]
		stageTicks := int64(stage.PulseCount) * ticksPerStep

		pitch := calculatePitch(pat, stageIdx, accum[stageIdx])
		visitPitch[i] = pitch

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
					offTick = onTick
				} else {
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

		applyAccumulator(stage, stageIdx, &accum, &accumCount, &accumDir)

		currentTick += stageTicks
	}

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
			events = append(events, devices.TimedEvent{
				Tick: slideStartTick + int64(pat.SlideTime),
				Event: midi.Event{
					Type:      midi.PitchBend,
					BendValue: 0,
				},
			})
		}
	}

	length := int64(0)
	for _, stageIdx := range walk {
		length += int64(pat.Stages[stageIdx].PulseCount) * ticksPerStep
	}
	if length < 1 {
		length = 1
	}

	for i := range events {
		if events[i].Tick >= length {
			events[i].Tick = length - 1
		}
		if events[i].Tick < 0 {
			events[i].Tick = 0
		}
	}

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

// applyAccumulator evolves local accumulator state for one stage visit.
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
			accumCount[stageIdx] = stage.AccumReset
			return
		}
	}

	accum[stageIdx] += stage.Accumulator * accumDir[stageIdx]
}
