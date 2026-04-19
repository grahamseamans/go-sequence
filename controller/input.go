// Package controller: input.go — the central event router and sole writer
// to model state.
//
// InputManager fans in events from three sources and dispatches them:
//
//  1. Pad events from the Launchpad (surface.PadEvents()).
//  2. Keyboard events from the TUI, delivered synchronously via HandleKey.
//  3. Per-track MIDI-in, one goroutine per track subscribed to the track's
//     configured (PortName, Channel).
//
// All writes to model state (specs, schedules, transport-adjacent fields)
// flow through InputManager. It's also the single source that decides when
// a pattern's Machine slots are stale: on mutation it nils both slots and
// non-blocking-sends TWO CompileRequests (one per slot) on compileCh. The
// compile goroutine then renders a fresh CompiledPattern into each slot.
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

	"go-sequence/controller/devices/drum"
	"go-sequence/controller/devices/empty"
	"go-sequence/controller/devices/metropolix"
	"go-sequence/controller/devices/piano"
	"go-sequence/controller/devices/save"
	"go-sequence/controller/devices/session"
	"go-sequence/controller/devices/settings"
	"go-sequence/controller/surface"
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
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
	out     midi.ToExternal
	in      midi.FromExternal
	surface surface.Surface

	compileCh chan<- model.CompileRequest

	// Focus state. Mutated only by InputManager (Run's select loop + the view
	// goroutine via HandleKey). Read by view via Focused() as a value copy.
	focused          FocusTarget
	lastFocusedTrack int // remembered when navigating to Settings/Session/Save

	// Save is the only stateful device singleton — it carries the save-
	// browser cursor and text-edit buffer. All other per-domain packages
	// (drum, piano, metropolix, session, settings, empty) expose free
	// functions and need no singleton.
	save    *save.Save
	saveOps save.SaveOps

	// Per-track MIDI subscriber cancels, so Close() can unsubscribe cleanly.
	midiCancels [8]context.CancelFunc
}

// NewInputManager wires InputManager to its collaborators. Does NOT start
// any goroutines — call Run in its own goroutine to begin fan-in.
func NewInputManager(
	project *model.Project,
	out midi.ToExternal,
	in midi.FromExternal,
	surf surface.Surface,
	compileCh chan<- model.CompileRequest,
	saveOps save.SaveOps,
) *InputManager {
	return &InputManager{
		project:   project,
		out:       out,
		in:        in,
		surface:   surf,
		compileCh: compileCh,
		focused:   FocusTarget{Kind: FocusTrack, Track: 0},
		save:      &save.Save{},
		saveOps:   saveOps,
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
			drum.HandlePad(track, im.project, im.out, ev.Row, ev.Col, ev.Down)
		case model.DevicePiano:
			piano.HandlePad(track, im.project, im.out, ev.Row, ev.Col, ev.Down)
		case model.DeviceMetropolix:
			metropolix.HandlePad(track, im.project, im.out, ev.Row, ev.Col, ev.Down)
		default:
			// DeviceNone — no-op device still gets the event for symmetry.
			empty.HandlePad(im.project, im.out, ev.Row, ev.Col, ev.Down)
		}
		track.Unlock()
		if ev.Down {
			im.markTrackDirty(idx)
		}

	case FocusSession:
		// Session mutates Schedule.Queued on project.Tracks[ev.Row]. Lock
		// THAT track (not the currently-focused track). The "dirty" pattern
		// is the queued column itself — mark it so its Machine slots are
		// ready by the time playback wraps into it.
		if ev.Row < 0 || ev.Row >= 8 {
			return
		}
		tgt := im.project.Tracks[ev.Row]
		if tgt != nil {
			tgt.Lock()
		}
		session.HandlePad(im.project, im.out, ev.Row, ev.Col, ev.Down)
		if tgt != nil {
			tgt.Unlock()
		}
		if ev.Down {
			// ev.Col is the queued pattern index; see session.HandlePad.
			im.markPatternDirty(ev.Row, ev.Col)
		}

	case FocusSettings:
		// Settings edits fields on Tracks[lastFocusedTrack]. Lock that track
		// around the handler.
		idx := im.lastFocusedTrack
		tgt := im.project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		settings.HandlePad(im.project, im.out, idx, ev.Row, ev.Col, ev.Down)
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
		drum.HandleMIDI(track, im.out, ev)
	case model.DevicePiano:
		piano.HandleMIDI(track, im.out, ev)
	case model.DeviceMetropolix:
		metropolix.HandleMIDI(track, im.out, ev)
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
		if Playing(im.project) {
			Pause(im.project)
		} else {
			Play(im.project)
		}
		return
	case "+", "=":
		SetTempo(im.project, im.project.Tempo+1)
		return
	case "-":
		SetTempo(im.project, im.project.Tempo-1)
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
			drum.HandleKey(track, im.project, im.out, key)
		case model.DevicePiano:
			piano.HandleKey(track, im.project, im.out, key)
		case model.DeviceMetropolix:
			metropolix.HandleKey(track, im.project, im.out, key)
		default:
			empty.HandleKey(im.project, im.out, key)
		}
		track.Unlock()
		im.markTrackDirty(idx)

	case FocusSession:
		// Session.HandleKey is currently a no-op, but dispatch for symmetry.
		// No dirty-mark: no state change possible.
		session.HandleKey(im.project, im.out, key)

	case FocusSettings:
		idx := im.lastFocusedTrack
		tgt := im.project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		settings.HandleKey(im.project, im.out, idx, key)
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
		// TODO(step4c-followup): expose save.Save.IsEditing() so we can
		// distinguish commit-name from load and skip the unnecessary
		// Pause + 8 recompiles in the commit-name case.
		wasLoad := key == "enter" || key == "return"
		im.save.HandleKey(im.project, im.saveOps, im.out, key)
		if wasLoad {
			// Project contents may have been replaced. Per DESIGN §4.5: stop
			// playback and recompile every track's currently-playing
			// pattern so playback has something to read on Play.
			Pause(im.project)
			ResetCursors(im.project)
			for i := 0; i < 8; i++ {
				im.markPlayingDirty(i)
			}
		}
	}
}

// markTrackDirty trashes both Machine slots of the edited pattern on the
// named track and signals the compiler to refill them. The "edited pattern"
// is resolved from the device's EditingPatternIdx for device handlers, or
// from the just-queued pattern for session handlers — we pass the pattern
// index in explicitly via markPatternDirty below.
//
// This wrapper figures out the editing pattern for the focused-device case,
// which is what the pad/key/midi handlers want.
func (im *InputManager) markTrackDirty(trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := im.project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return
	}
	var patternIdx int
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			return
		}
		patternIdx = track.Drum.EditingPatternIdx
	case model.DevicePiano:
		if track.Piano == nil {
			return
		}
		patternIdx = track.Piano.EditingPatternIdx
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			return
		}
		patternIdx = track.Metropolix.EditingPatternIdx
	default:
		return
	}
	im.markPatternDirty(trackIdx, patternIdx)
}

// markPlayingDirty is like markTrackDirty but targets the currently-playing
// pattern (Schedule.Playing) rather than the editing one. Used after project
// load to prime the Machine slots playback will read from.
func (im *InputManager) markPlayingDirty(trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := im.project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return
	}
	var patternIdx int
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			return
		}
		patternIdx = track.Drum.Schedule.Playing
	case model.DevicePiano:
		if track.Piano == nil {
			return
		}
		patternIdx = track.Piano.Schedule.Playing
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			return
		}
		patternIdx = track.Metropolix.Schedule.Playing
	default:
		return
	}
	im.markPatternDirty(trackIdx, patternIdx)
}

// markPatternDirty trashes both Machine slots of the named pattern on the
// named track, then non-blocking-sends ONE CompileRequest per slot so the
// compiler refills them with fresh rolls. Used by session (queue-change)
// and device handlers (spec edits).
//
// Trashing before sending is load-bearing: the nil-store MUST be visible
// before playback reads the slot — atomic.Store provides the needed
// publish semantics. (Compile unconditionally overwrites the slot, so the
// nil isn't strictly needed for compile's correctness, but playback
// observing a stale cached CompiledPattern between the edit and the
// compile completing would briefly emit wrong events. The nil makes
// playback silent for that window instead.)
func (im *InputManager) markPatternDirty(trackIdx, patternIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	if patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	track := im.project.Tracks[trackIdx]
	if track == nil {
		return
	}
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil && track.Drum.Patterns[patternIdx] != nil {
			track.Drum.Patterns[patternIdx].Machine[0].Store(nil)
			track.Drum.Patterns[patternIdx].Machine[1].Store(nil)
		}
	case model.DevicePiano:
		if track.Piano != nil && track.Piano.Patterns[patternIdx] != nil {
			track.Piano.Patterns[patternIdx].Machine[0].Store(nil)
			track.Piano.Patterns[patternIdx].Machine[1].Store(nil)
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil && track.Metropolix.Patterns[patternIdx] != nil {
			track.Metropolix.Patterns[patternIdx].Machine[0].Store(nil)
			track.Metropolix.Patterns[patternIdx].Machine[1].Store(nil)
		}
	default:
		return
	}
	for slot := 0; slot < 2; slot++ {
		select {
		case im.compileCh <- model.CompileRequest{Track: trackIdx, Pattern: patternIdx, Slot: slot}:
		default:
			debug.Log("input", "compileCh full, dropping compile request for track=%d pattern=%d slot=%d", trackIdx, patternIdx, slot)
		}
	}
}
