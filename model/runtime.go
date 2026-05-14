package model

import (
	"sync"
	"sync/atomic"
	"time"

	"go-sequence/model/system"
)

// Runtime state for a Project. Every type here is referenced by Project's
// `json:"-"` fields; none of it is persisted. Lives in model/ per DESIGN
// §3.2 — "Model owns ALL state", the persisted/runtime distinction is
// just a JSON tag.

// Cursor is the per-track playback anchor. The playhead itself is NOT
// stored — it's computed every tick as (globalTick - T0Tick), per DESIGN.
// All per-track playback state lives here:
//
//   - CurrentPattern: index of the pattern this track is currently
//     playing. Reads from track.<Device>.Patterns[CurrentPattern].
//   - QueuedPattern: index of the pattern that will start at the next
//     pattern boundary, or -1 when nothing is queued. Writers (TUI / pad
//     handlers) just set this; the playback engine swaps it into
//     CurrentPattern on wrap.
//   - T0Tick: the global tick at which CurrentPattern started playing.
//   - CurrentSlot: which Machine slot (0 or 1) playback is reading.
//
// Runtime-only — Cursor is not persisted to save files. Loading a project
// resets every track to (CurrentPattern=0, QueuedPattern=-1).
type Cursor struct {
	CurrentPattern int
	T0Tick         int64
	CurrentSlot    int
	QueuedPattern  int
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
	// Focus identifies the active UI region. Focus.Track always names the
	// most-recently-focused track, regardless of Focus.Kind — so Settings /
	// Session / Project handlers can read Focus.Track to know which track
	// they're operating on when the user navigates away from the track view.
	Focus FocusTarget

	// KeyboardRoute is the track index (0..7) that external MIDI keyboard
	// input is routed to. One keyboard, one destination track at a time.
	// Defaults to 0. Writers go through SetKeyboardRoute so the keyboard
	// routine wakes up without polling.
	KeyboardRoute int

	// KeyboardRouteChanged is a size-1 buffered channel. SetKeyboardRoute
	// does a non-blocking send; the keyboard goroutine selects on it to
	// pick up route changes immediately. json:"-" — runtime-only.
	KeyboardRouteChanged chan struct{} `json:"-"`

	// SystemProject is the entire project-browser widget state (cursor,
	// edit-mode buffer, cached save list). Lives here — not in a separate
	// singleton — because it's UI state belonging to the project, and
	// model/system is a leaf package.
	SystemProject system.Project `json:"-"`

	// SystemSession holds the session-view cursor (track, pattern) for
	// keyboard-driven clip launching from the TUI.
	SystemSession system.Session `json:"-"`

	// SystemSettings holds the settings-view cursor (row, col) and modal
	// picker state for editing track configuration from the TUI.
	SystemSettings system.Settings `json:"-"`
}

// SetKeyboardRoute atomically updates the keyboard route and non-blocking-
// signals the keyboard goroutine. Writers must use this instead of
// writing KeyboardRoute directly — the channel signal is how the
// keyboard routine learns about the change without polling.
func SetKeyboardRoute(project *Project, trackIdx int) {
	project.UI.KeyboardRoute = trackIdx
	select {
	case project.UI.KeyboardRouteChanged <- struct{}{}:
	default:
	}
}

// PlaybackState is the runtime transport and per-track cursor state owned by
// the playback goroutine. No longer carries compiled events: those live in
// each DrumPattern/PianoPattern/MetropolixPattern's Machine slots. Stale
// detection is no longer done by edit-counter comparison either — playback
// marks consumed slots nil and the compile goroutine fills them.
type PlaybackState struct {
	Cursors [8]Cursor

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

// CompileRequest names a single (track, pattern, slot) target for the
// compile goroutine. Each request fills exactly ONE Machine slot with a
// freshly-rolled CompiledPattern. Callers that want both slots filled send
// two requests — one per slot — so the "fresh rolls every loop" contract
// (DESIGN §3.10) applies uniformly whether the trigger was an edit, a
// queue change, or a wrap-and-refill.
type CompileRequest struct {
	Track   int
	Pattern int
	Slot    int
}

// CompileState is the runtime state owned by the compile goroutine (plus
// the input goroutine which produces on Channel).
type CompileState struct {
	// Channel carries CompileRequest values. Mutators send; RunCompile
	// reads. Buffered so bursty edits never block the sender.
	Channel chan CompileRequest

	// LoopCounter disambiguates seeds when multiple compiles land in the
	// same nanosecond. Bumped atomically inside RunCompile.
	LoopCounter atomic.Uint64
}

// NewCompileState returns a CompileState with a buffered channel ready
// to use. Call once at project construction.
func NewCompileState() CompileState {
	return CompileState{Channel: make(chan CompileRequest, 32)}
}
