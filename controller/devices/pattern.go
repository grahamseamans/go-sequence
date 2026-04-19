package devices

import "go-sequence/midi"

// CompiledPattern is the output of a device's Compile — a pre-rendered
// sequence of events for one pattern iteration. Produced by the compile
// goroutine, consumed by PlaybackEngine. Never persisted.
type CompiledPattern struct {
	Events      []TimedEvent
	Length      int64  // pattern length in ticks
	EditCounter uint64 // track's editCounter at compile time; used for stale-detection at wrap
}

// TimedEvent is a MIDI event scheduled at a specific tick within a pattern.
type TimedEvent struct {
	Tick  int64
	Event midi.Event
}
