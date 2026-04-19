package piano

import (
	"sort"

	"go-sequence/controller/devices/rng"
	"go-sequence/midi"
	"go-sequence/model/devices"
)

// Package piano is the stateless piano-roll device compiler. Turns a Piano
// spec + seed into a CompiledPattern. UI logic (pad, TUI key, MIDI record)
// lives under view/{launchpad,tui,keyboard}/piano.go.
//
// Callers (Compiler goroutine) hold the Track's mutex; functions here
// never touch it themselves.

// Compile renders a single piano pattern into a CompiledPattern.
//
// Pure: no I/O, no mutation of spec, no globals. Same (pattern, seed) →
// same output. Off-ticks that extend past the pattern end are clamped to
// `length - 1` (matches drum.go's boundary convention).
func Compile(pat *devices.PianoPattern, seed uint64) devices.CompiledPattern {
	// Rng is built for future use (probability, humanize). Piano currently has
	// no random behavior; keep the seed plumbing in place.
	// TODO(probability): use r when per-note probability lands.
	_ = rng.NewRng(seed)

	if pat == nil {
		return devices.CompiledPattern{Length: 1}
	}

	length := int64(pat.Length * float64(devices.PPQ))
	if length < 1 {
		length = 1
	}

	events := make([]devices.TimedEvent, 0, len(pat.Notes)*2)

	for i := range pat.Notes {
		n := &pat.Notes[i]

		onTick := int64(n.Start * float64(devices.PPQ))
		if onTick < 0 {
			onTick = 0
		}
		if onTick >= length {
			onTick = length - 1
		}

		offTick := int64((n.Start + n.Duration) * float64(devices.PPQ))
		if offTick <= onTick {
			offTick = onTick + 1
		}
		if offTick >= length {
			offTick = length - 1
		}

		events = append(events, devices.TimedEvent{
			Tick: onTick,
			Event: midi.Event{
				Type:     midi.NoteOn,
				Note:     n.Pitch,
				Velocity: n.Velocity,
				Channel:  0,
			},
		})
		events = append(events, devices.TimedEvent{
			Tick: offTick,
			Event: midi.Event{
				Type:     midi.NoteOff,
				Note:     n.Pitch,
				Velocity: 0,
				Channel:  0,
			},
		})
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return devices.CompiledPattern{
		Events: events,
		Length: length,
	}
}
