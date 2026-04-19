package model

import (
	"sync"
	"sync/atomic"
	"time"

	"go-sequence/midi"
)

// Runtime state for a Project. Every type here is referenced by Project's
// `json:"-"` fields; none of it is persisted. Lives in model/ per DESIGN
// §3.2 — "Model owns ALL state", the persisted/runtime distinction is
// just a JSON tag.

// CompiledPattern is the output of a device's Compile — a pre-rendered
// sequence of events for one pattern iteration. Produced by the compile
// goroutine, consumed by playback.
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

// Cursor is a per-track playback position within its currently-playing pattern.
type Cursor struct {
	Tick int64
}

// FocusKind selects which domain is active in the UI.
type FocusKind int

const (
	FocusTrack FocusKind = iota // a per-track device view (Drum/Piano/Metropolix)
	FocusSession
	FocusSettings
	FocusProject // was "Save" — project browser (load/save/rename/delete)
)

// FocusTarget is the current UI focus. Mutated only by the input-routing
// goroutine; read by view via a small value copy.
type FocusTarget struct {
	Kind  FocusKind
	Track int // valid when Kind == FocusTrack; 0-7
}

// UIState holds all transient UI state that isn't per-track device state.
type UIState struct {
	Focus            FocusTarget
	LastFocusedTrack int // remembered when user navigates away from a track view
}

// PlaybackState is the runtime state owned by the playback goroutine.
// All atomics so the tick path stays lock-free; the non-atomic fields are
// touched only by Play/Pause/SetTempo, which run on the input goroutine.
type PlaybackState struct {
	Cursors [8]Cursor

	// Double-buffered per-track events. Playback reads current; compile
	// writes next. At wrap boundary, current ← next (atomic swap).
	CurrentEvents [8]atomic.Pointer[CompiledPattern]
	NextEvents    [8]atomic.Pointer[CompiledPattern]

	// EditCounters[i] is bumped by controller mutators on any spec
	// mutation. Compile stamps the value onto each CompiledPattern; wrap
	// handler uses it to detect stale current events.
	EditCounters [8]atomic.Uint64

	Playing atomic.Bool
	Tick    atomic.Int64 // global tick count since last Play()

	// LastGlobalTick: previous globalTick value; used to compute delta
	// each tick. Only written by the tick goroutine.
	LastGlobalTick atomic.Int64

	// T0 is the wall-clock anchor for tick calculation. Written by
	// Play/SetTempo (input goroutine), read by tick goroutine. Guarded
	// by T0Mu.
	T0   time.Time
	T0Mu sync.RWMutex

	// Tempo in BPM. Read by tick goroutine every iteration so mid-play
	// tempo changes are picked up without restart.
	Tempo atomic.Int64
}

// CompileState is the runtime state owned by the compile goroutine (plus
// the input goroutine which produces on Channel).
type CompileState struct {
	// Channel carries trackIdx values. Mutators send; RunCompile reads.
	// Buffered to ~16 so bursty edits never block.
	Channel chan int

	// LoopCounter disambiguates seeds when multiple compiles land in the
	// same nanosecond. Bumped atomically inside RunCompile.
	LoopCounter atomic.Uint64
}

// NewCompileState returns a CompileState with a buffered channel ready
// to use. Call once at project construction.
func NewCompileState() CompileState {
	return CompileState{Channel: make(chan int, 16)}
}
