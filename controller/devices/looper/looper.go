package looper

import (
	"sort"

	"go-sequence/model/devices"
)

// Compile renders a single looper pattern into a CompiledPattern.
// For the looper, compilation is trivial: wrap the recorded events
// in a CompiledPattern with the pattern's length. Pure: no I/O, no
// mutation of spec, no globals.
//
// seed is unused today; kept in the signature so probability/humanize can
// be added without churning every call site.
func Compile(pat *devices.LooperPattern, seed uint64) devices.CompiledPattern {
	if pat == nil {
		return devices.CompiledPattern{Length: 1}
	}

	length := pat.Length
	if length < 1 {
		length = 1
	}

	events := make([]devices.TimedEvent, len(pat.Events))
	copy(events, pat.Events)

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return devices.CompiledPattern{
		Events: events,
		Length: length,
	}
}
