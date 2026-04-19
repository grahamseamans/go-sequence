// Package controller: input.go — the central event router and sole writer
// to model state.
//
// InputManager fans in events from three sources and dispatches them:
//
//   1. Pad events from the Launchpad (surface.PadEvents()).
//   2. Keyboard events from the TUI, delivered synchronously via HandleKey.
//   3. Per-track MIDI-in, one goroutine per track subscribed to the track's
//      configured (PortName, Channel).
//
// All writes to model state (specs, schedules, transport-adjacent fields)
// flow through InputManager. It's also the single source that decides when
// a track's compiled events are stale and need regenerating: on mutation,
// it bumps the track's editCounter (on PlaybackEngine) and non-blocking
// sends the trackIdx on compileCh.
//
// Concurrency shape (DESIGN §6):
//
//   - InputManager writes spec fields under track.Lock(). Compiler holds
//     track.RLock() while reading, so writes briefly block compiles. Writes
//     are bursty and sub-millisecond.
//   - Focus state (`focused`, `lastFocusedTrack`) is mutated only by
//     InputManager's own goroutine (Run's select loop) plus HandleKey which
//     runs on the view goroutine. That's two writers — acceptable only
//     because HandleKey and Run never contend for the same field within a
//     single logical operation and because Focused() returns a value copy.
//     If multi-writer ever becomes a problem we'd switch to atomic.Pointer
//     or route keys through a channel; not worth it yet.
//   - compileCh is non-blocking: if the buffer fills we log and drop. The
//     compiler is about to drain it anyway, and each drop is followed by
//     the very next tick-wrap signal which effectively re-queues.
package controller

import (
	"context"
	"sync/atomic"

	"go-sequence/controller/devices"
	"go-sequence/controller/surface"
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
)

// FocusKind tags which UI context input is routed to. Per DESIGN §4.8 focus
// lives on InputManager, not in Model.
type FocusKind int

const (
	FocusTrack FocusKind = iota
	FocusSession
	FocusSettings
	FocusSave
)

// FocusTarget is the current focus. Track is only meaningful when
// Kind == FocusTrack; for Settings we use lastFocusedTrack to remember
// which track the user was on before tabbing to Settings.
type FocusTarget struct {
	Kind  FocusKind
	Track int // valid when Kind == FocusTrack; 0-7
}

// InputManager owns all input routing and is the sole writer to model state.
type InputManager struct {
	project *model.Project
	pe      *PlaybackEngine
	out     midi.ToExternal
	in      midi.FromExternal
	surface surface.Surface

	compileCh    chan<- int
	editCounters *[8]atomic.Uint64

	// Focus state. Mutated only by InputManager (Run's select loop + the view
	// goroutine via HandleKey). Read by view via Focused() as a value copy.
	focused          FocusTarget
	lastFocusedTrack int // remembered when navigating to Settings/Session/Save

	// Device singletons. Drum/Piano/Metropolix/Session/Settings/Empty are
	// empty structs (value semantics). Save is a stateful singleton that
	// carries the save-browser cursor and text-edit buffer.
	save    *devices.Save
	saveOps devices.SaveOps

	// Per-track MIDI subscriber cancels, so Close() can unsubscribe cleanly.
	midiCancels [8]context.CancelFunc
}

// NewInputManager wires InputManager to its collaborators. Does NOT start
// any goroutines — call Run in its own goroutine to begin fan-in.
func NewInputManager(
	project *model.Project,
	pe *PlaybackEngine,
	out midi.ToExternal,
	in midi.FromExternal,
	surf surface.Surface,
	compileCh chan<- int,
	saveOps devices.SaveOps,
) *InputManager {
	return &InputManager{
		project:      project,
		pe:           pe,
		out:          out,
		in:           in,
		surface:      surf,
		compileCh:    compileCh,
		editCounters: pe.EditCounters(),
		focused:      FocusTarget{Kind: FocusTrack, Track: 0},
		save:         &devices.Save{},
		saveOps:      saveOps,
	}
}

// Run starts per-track MIDI-in subscribers, then consumes pad events from
// the surface until ctx is cancelled. Should be invoked in its own goroutine
// from main.
func (im *InputManager) Run(ctx context.Context) {
	for i := 0; i < 8; i++ {
		im.startMidiSubscriber(ctx, i)
	}

	padCh := im.surface.PadEvents()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-padCh:
			if !ok {
				return
			}
			im.handlePad(ev)
		}
	}
}

// Focused returns the current focus target by value, safe to read from the
// view goroutine without locking.
func (im *InputManager) Focused() FocusTarget { return im.focused }

// startMidiSubscriber spins up a goroutine that forwards MIDI-in events for
// one track through the normal dispatch path. Skips tracks that have no
// device or no configured port — silent, because those tracks legitimately
// shouldn't receive MIDI.
func (im *InputManager) startMidiSubscriber(parentCtx context.Context, trackIdx int) {
	track := im.project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone || track.PortName == "" {
		return
	}
	ch, err := im.in.Subscribe(track.PortName, track.Channel)
	if err != nil {
		debug.Log("input", "midi subscribe failed track=%d port=%q err=%v",
			trackIdx, track.PortName, err)
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	im.midiCancels[trackIdx] = cancel
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				im.handleMidi(trackIdx, ev)
			}
		}
	}()
}

// handlePad routes a pad event to the focused UI. Per-track dispatches hold
// track.Lock for the duration of the handler, then mark the track dirty if
// the press could have mutated state (only on press, not release).
func (im *InputManager) handlePad(ev surface.PadEvent) {
	switch im.focused.Kind {
	case FocusTrack:
		idx := im.focused.Track
		track := im.project.Tracks[idx]
		if track == nil {
			return
		}
		track.Lock()
		switch track.Type {
		case model.DeviceDrum:
			devices.Drum{}.HandlePad(track, im.project, im.out, ev.Row, ev.Col, ev.Down)
		case model.DevicePiano:
			devices.Piano{}.HandlePad(track, im.project, im.out, ev.Row, ev.Col, ev.Down)
		case model.DeviceMetropolix:
			devices.Metropolix{}.HandlePad(track, im.project, im.out, ev.Row, ev.Col, ev.Down)
		default:
			// DeviceNone — no-op device still gets the event for symmetry.
			devices.Empty{}.HandlePad(im.project, im.out, ev.Row, ev.Col, ev.Down)
		}
		track.Unlock()
		if ev.Down {
			im.markTrackDirty(idx)
		}

	case FocusSession:
		// Session mutates Schedule.Queued on project.Tracks[ev.Row]. Lock
		// THAT track (not the currently-focused track) and bump its counter.
		if ev.Row < 0 || ev.Row >= 8 {
			return
		}
		tgt := im.project.Tracks[ev.Row]
		if tgt != nil {
			tgt.Lock()
		}
		devices.Session{}.HandlePad(im.project, im.out, ev.Row, ev.Col, ev.Down)
		if tgt != nil {
			tgt.Unlock()
		}
		if ev.Down {
			im.markTrackDirty(ev.Row)
		}

	case FocusSettings:
		// Settings edits fields on Tracks[lastFocusedTrack]. Lock that track
		// around the handler.
		idx := im.lastFocusedTrack
		tgt := im.project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		devices.Settings{}.HandlePad(im.project, im.out, idx, ev.Row, ev.Col, ev.Down)
		if tgt != nil {
			tgt.Unlock()
		}
		if ev.Down {
			im.markTrackDirty(idx)
		}

	case FocusSave:
		// Save doesn't take track locks; its pad handler is currently a
		// no-op, but keep the dispatch symmetric so a future pad UX lands
		// here without ceremony. Load (via HandleKey) is the only path that
		// mutates project-wide — handled in HandleKey.
		im.save.HandlePad(im.project, im.saveOps, im.out, ev.Row, ev.Col, ev.Down)
	}
}

// handleMidi routes a MIDI-in event to the track's device. Always bumps the
// track's edit counter — any recording device may have mutated spec, and
// non-recording devices' HandleMIDI is a no-op so the extra recompile is
// effectively a cheap false positive we accept for simplicity.
func (im *InputManager) handleMidi(trackIdx int, ev midi.Event) {
	track := im.project.Tracks[trackIdx]
	if track == nil {
		return
	}
	track.Lock()
	switch track.Type {
	case model.DeviceDrum:
		devices.Drum{}.HandleMIDI(track, im.out, ev)
	case model.DevicePiano:
		devices.Piano{}.HandleMIDI(track, im.out, ev)
	case model.DeviceMetropolix:
		devices.Metropolix{}.HandleMIDI(track, im.out, ev)
	}
	track.Unlock()
	im.markTrackDirty(trackIdx)
}

// HandleKey is invoked synchronously from the view (Bubbletea) goroutine.
// Global hotkeys (transport, focus) are handled directly; everything else
// routes to the focused device under the appropriate track lock.
func (im *InputManager) HandleKey(key string) {
	// Global hotkeys: never forwarded to devices.
	switch key {
	case " ", "space":
		if im.pe.Playing() {
			im.pe.Pause()
		} else {
			im.pe.Play()
		}
		return
	case "+", "=":
		newTempo := im.project.Tempo + 1
		im.pe.SetTempo(newTempo)
		im.project.Tempo = newTempo
		return
	case "-":
		newTempo := im.project.Tempo - 1
		im.pe.SetTempo(newTempo)
		im.project.Tempo = newTempo
		return
	case "1", "2", "3", "4", "5", "6", "7", "8":
		idx := int(key[0] - '1')
		im.focused = FocusTarget{Kind: FocusTrack, Track: idx}
		im.lastFocusedTrack = idx
		return
	case ",":
		im.focused.Kind = FocusSettings
		return
	case "S":
		im.focused.Kind = FocusSave
		// Refresh the save-list cache on entry so the browser shows the
		// current state of disk, not whatever was cached last time we were
		// focused here.
		im.save.Refresh(im.saveOps)
		return
	case "tab":
		im.focused.Kind = FocusSession
		return
	}

	// Not a global hotkey — route to the focused device.
	switch im.focused.Kind {
	case FocusTrack:
		idx := im.focused.Track
		track := im.project.Tracks[idx]
		if track == nil {
			return
		}
		track.Lock()
		switch track.Type {
		case model.DeviceDrum:
			devices.Drum{}.HandleKey(track, im.project, im.out, key)
		case model.DevicePiano:
			devices.Piano{}.HandleKey(track, im.project, im.out, key)
		case model.DeviceMetropolix:
			devices.Metropolix{}.HandleKey(track, im.project, im.out, key)
		default:
			devices.Empty{}.HandleKey(im.project, im.out, key)
		}
		track.Unlock()
		im.markTrackDirty(idx)

	case FocusSession:
		// Session.HandleKey is currently a no-op, but dispatch for symmetry.
		// No dirty-mark: no state change possible.
		devices.Session{}.HandleKey(im.project, im.out, key)

	case FocusSettings:
		idx := im.lastFocusedTrack
		tgt := im.project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		devices.Settings{}.HandleKey(im.project, im.out, idx, key)
		if tgt != nil {
			tgt.Unlock()
		}
		im.markTrackDirty(idx)

	case FocusSave:
		// Any "enter" while focused on Save is treated as a potential load
		// (enter is also used to commit a name in edit mode, but the
		// commit-name path is a cheap false positive that just triggers
		// one extra round of recompiles — still correct).
		//
		// TODO(step4c-followup): expose devices.Save.IsEditing() so we can
		// distinguish commit-name from load and skip the unnecessary
		// Pause + 8 recompiles in the commit-name case.
		wasLoad := key == "enter" || key == "return"
		im.save.HandleKey(im.project, im.saveOps, im.out, key)
		if wasLoad {
			// Project contents may have been replaced. Per DESIGN §4.5: stop
			// playback and recompile every track so current_events and
			// next_events don't reference stale specs.
			im.pe.Pause()
			im.pe.ResetCursors()
			for i := 0; i < 8; i++ {
				im.markTrackDirty(i)
			}
		}
	}
}

// markTrackDirty increments the track's edit counter and non-blocking sends
// on compileCh. The counter bump MUST precede the channel send so that if
// the compiler is already draining the channel, the increment is visible by
// the time compileTrack reads editCounters[i].
func (im *InputManager) markTrackDirty(trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	im.editCounters[trackIdx].Add(1)
	select {
	case im.compileCh <- trackIdx:
	default:
		debug.Log("input", "compileCh full, dropping compile request for track=%d", trackIdx)
	}
}
