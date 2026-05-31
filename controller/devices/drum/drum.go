package drum

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model/devices"
)

// Package drum is the stateless drum device compiler. It turns a Drum
// spec + kit + seed into a CompiledPattern. UI logic (pad, TUI key, MIDI
// record) lives under view/{launchpad,tui,keyboard}/drum.go.
//
// Callers (Compiler goroutine) hold the Track's mutex; functions here
// never touch it themselves.

// noteDurationTicks is how long each drum NoteOn is held before the matching
// NoteOff. Drum hits are short, so a fixed short duration is fine.
const noteDurationTicks int64 = 60

// Compile renders a single drum pattern into a CompiledPattern.
//
// Pure: no I/O, no mutation of spec, no globals. Same (pattern, kit, seed)
// → same output. The caller picks which pattern to render (the Compiler
// goroutine reads track.Drum.Patterns[req.Pattern] and passes it in), so
// the cursor.CurrentPattern indirection no longer lives here.
//
// Signature diverges from DESIGN.md §5.4: Compile takes an explicit
// `kit devices.Kit` because the kit lives on the Track, not on the Drum
// spec.
//
// seed is unused today; kept in the signature so probability/humanize can
// be added without churning every call site. TODO(probability): build an
// rng from seed when per-step probability lands.
func Compile(pat *devices.DrumPattern, kit devices.Kit, seed uint64) devices.CompiledPattern {
	if pat == nil {
		return devices.CompiledPattern{Length: 1}
	}

	length := int64(pat.Length)
	if length < 1 {
		length = 1
	}

	ticksPerStep := int64(devices.PPQ / 4)
	// stepsInPattern is how many 16th-note steps fit within pat.Length.
	// Steps beyond this index exist in the spec's fixed [32]DrumStep array
	// but are not played — the pattern's length is the gate.
	stepsInPattern := int(length / ticksPerStep)
	if stepsInPattern > 32 {
		stepsInPattern = 32
	}

	events := make([]devices.TimedEvent, 0, stepsInPattern*4)

	for laneIdx := 0; laneIdx < 16; laneIdx++ {
		lane := &pat.Notes[laneIdx]
		note := kit.Notes[laneIdx]

		for stepIdx := 0; stepIdx < stepsInPattern; stepIdx++ {
			step := lane.Steps[stepIdx]
			if !step.Active {
				continue
			}

			velocity := step.Velocity
			if velocity == 0 {
				velocity = 100
			}

			baseTick := int64(stepIdx) * ticksPerStep
			onTick := baseTick + int64(step.Nudge)
			// Clamp to [0, length). Nudge may push a step off the front or
			// past the end; snap to the pattern instead of dropping it.
			if onTick < 0 {
				onTick = 0
			}
			if onTick >= length {
				onTick = length - 1
			}

			offTick := onTick + noteDurationTicks
			if offTick >= length {
				offTick = length - 1
			}

			events = append(events, devices.TimedEvent{
				Tick: onTick,
				Event: midi.Event{
					Type:     midi.NoteOn,
					Note:     note,
					Velocity: velocity,
				},
			})
			events = append(events, devices.TimedEvent{
				Tick: offTick,
				Event: midi.Event{
					Type:     midi.NoteOff,
					Note:     note,
					Velocity: 0,
				},
			})
		}
	}

	// Stable sort so equal-tick events keep insertion order (NoteOn before
	// NoteOff when they collide at the pattern boundary, etc.).
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return devices.CompiledPattern{
		Events: events,
		Length: length,
	}
}
