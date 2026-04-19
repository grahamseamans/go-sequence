// Package controller: input.go — the central event router and sole writer
// to model state.
//
// This file exposes two public entry points — both free functions, no
// types — per DESIGN §3 ("controller is bags of functions, no types") and
// §4.8 (focus/UI state lives on model.UIState, not on a controller-owned
// struct):
//
//  1. RunInput — the fan-in goroutine. Starts per-track MIDI-in
//     subscribers and consumes pad events from the surface until ctx is
//     cancelled. Per-track subscribers are parented on the derived ctx so
//     they shut down automatically when RunInput returns.
//  2. HandleKey — called synchronously from the view (Bubbletea)
//     goroutine with each keystroke. Global hotkeys (transport, focus) are
//     handled directly; everything else routes to the focused device.
//
// All writes to model state flow through these two entry points. On every
// mutation we nil the affected pattern's Machine slots and non-blocking-
// send TWO CompileRequests (one per slot) on project.Compile.Channel; the
// compile goroutine then renders a fresh CompiledPattern into each slot.
//
// Concurrency shape (DESIGN §6):
//
//   - Spec writes hold track.Lock(). Compiler holds track.RLock() while
//     reading, so writes briefly block compiles. Writes are bursty and
//     sub-millisecond.
//   - project.UI (Focus, LastFocusedTrack, SystemProject) is mutated only
//     by RunInput's goroutine plus HandleKey on the view goroutine. That's
//     two writers — acceptable only because HandleKey and RunInput never
//     contend for the same field within a single logical operation. If
//     multi-writer ever becomes a problem we'd switch to atomic.Pointer or
//     route keys through a channel; not worth it yet.
//   - project.Compile.Channel is non-blocking: if the buffer fills we log
//     and drop. The compiler is about to drain it anyway, and each drop is
//     followed by the very next tick-wrap signal which effectively
//     re-queues.
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

// RunInput starts per-track MIDI-in subscribers and fans pad events into
// the dispatcher until ctx is cancelled. Invoke in its own goroutine from
// main. All subscribers are parented on a context derived from ctx, so
// they shut down automatically when RunInput returns.
func RunInput(
	ctx context.Context,
	project *model.Project,
	out midi.ToExternal,
	in midi.FromExternal,
	surf surface.Surface,
	saveOps save.SaveOps,
) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < 8; i++ {
		startMidiSubscriber(subCtx, project, out, in, i)
	}

	padCh := surf.PadEvents()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-padCh:
			if !ok {
				return
			}
			handlePad(project, out, saveOps, ev)
		}
	}
}

// Focused returns the current focus target by value, safe to read from the
// view goroutine without locking.
func Focused(project *model.Project) model.FocusTarget {
	return project.UI.Focus
}

// startMidiSubscriber spins up a goroutine that forwards MIDI-in events
// for one track through the normal dispatch path. Skips tracks that have
// no device or no configured port — silent, because those tracks
// legitimately shouldn't receive MIDI.
func startMidiSubscriber(ctx context.Context, project *model.Project, out midi.ToExternal, in midi.FromExternal, trackIdx int) {
	track := project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone || track.PortName == "" {
		return
	}
	ch, err := in.Subscribe(track.PortName, track.Channel)
	if err != nil {
		debug.Log("input", "midi subscribe failed track=%d port=%q err=%v",
			trackIdx, track.PortName, err)
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				handleMidi(project, out, trackIdx, ev)
			}
		}
	}()
}

// handlePad routes a pad event to the focused UI. Per-track dispatches
// hold track.Lock for the duration of the handler, then mark the track
// dirty if the press could have mutated state (only on press, not
// release).
func handlePad(project *model.Project, out midi.ToExternal, saveOps save.SaveOps, ev surface.PadEvent) {
	switch project.UI.Focus.Kind {
	case model.FocusTrack:
		idx := project.UI.Focus.Track
		track := project.Tracks[idx]
		if track == nil {
			return
		}
		track.Lock()
		switch track.Type {
		case model.DeviceDrum:
			drum.HandlePad(track, project, out, ev.Row, ev.Col, ev.Down)
		case model.DevicePiano:
			piano.HandlePad(track, project, out, ev.Row, ev.Col, ev.Down)
		case model.DeviceMetropolix:
			metropolix.HandlePad(track, project, out, ev.Row, ev.Col, ev.Down)
		default:
			// DeviceNone — no-op device still gets the event for symmetry.
			empty.HandlePad(project, out, ev.Row, ev.Col, ev.Down)
		}
		track.Unlock()
		if ev.Down {
			markTrackDirty(project, idx)
		}

	case model.FocusSession:
		// Session mutates Schedule.Queued on project.Tracks[ev.Row]. Lock
		// THAT track (not the currently-focused track). The "dirty" pattern
		// is the queued column itself — mark it so its Machine slots are
		// ready by the time playback wraps into it.
		if ev.Row < 0 || ev.Row >= 8 {
			return
		}
		tgt := project.Tracks[ev.Row]
		if tgt != nil {
			tgt.Lock()
		}
		session.HandlePad(project, out, ev.Row, ev.Col, ev.Down)
		if tgt != nil {
			tgt.Unlock()
		}
		if ev.Down {
			// ev.Col is the queued pattern index; see session.HandlePad.
			markPatternDirty(project, ev.Row, ev.Col)
		}

	case model.FocusSettings:
		// Settings edits fields on Tracks[LastFocusedTrack]. Lock that
		// track around the handler.
		idx := project.UI.LastFocusedTrack
		tgt := project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		settings.HandlePad(project, out, idx, ev.Row, ev.Col, ev.Down)
		if tgt != nil {
			tgt.Unlock()
		}
		if ev.Down {
			markTrackDirty(project, idx)
		}

	case model.FocusProject:
		// Project-browser doesn't take track locks; its pad handler is
		// currently a no-op, but keep the dispatch symmetric so a future
		// pad UX lands here without ceremony. Load (via HandleKey) is the
		// only path that mutates project-wide — handled in HandleKey.
		save.HandlePad(&project.UI.SystemProject, project, saveOps, out, ev.Row, ev.Col, ev.Down)
	}
}

// handleMidi routes a MIDI-in event to the track's device. Always bumps
// the track's edit counter — any recording device may have mutated spec,
// and non-recording devices' HandleMIDI is a no-op so the extra recompile
// is effectively a cheap false positive we accept for simplicity.
func handleMidi(project *model.Project, out midi.ToExternal, trackIdx int, ev midi.Event) {
	track := project.Tracks[trackIdx]
	if track == nil {
		return
	}
	track.Lock()
	switch track.Type {
	case model.DeviceDrum:
		drum.HandleMIDI(track, out, ev)
	case model.DevicePiano:
		piano.HandleMIDI(track, out, ev)
	case model.DeviceMetropolix:
		metropolix.HandleMIDI(track, out, ev)
	}
	track.Unlock()
	markTrackDirty(project, trackIdx)
}

// HandleKey is invoked synchronously from the view (Bubbletea) goroutine.
// Global hotkeys (transport, focus) are handled directly; everything else
// routes to the focused device under the appropriate track lock.
func HandleKey(project *model.Project, out midi.ToExternal, saveOps save.SaveOps, key string) {
	// Global hotkeys: never forwarded to devices.
	switch key {
	case " ", "space":
		if Playing(project) {
			Pause(project)
		} else {
			Play(project)
		}
		return
	case "+", "=":
		SetTempo(project, project.Tempo+1)
		return
	case "-":
		SetTempo(project, project.Tempo-1)
		return
	case "1", "2", "3", "4", "5", "6", "7", "8":
		idx := int(key[0] - '1')
		project.UI.Focus = model.FocusTarget{Kind: model.FocusTrack, Track: idx}
		project.UI.LastFocusedTrack = idx
		return
	case ",":
		project.UI.Focus.Kind = model.FocusSettings
		return
	case "S":
		project.UI.Focus.Kind = model.FocusProject
		// Refresh the save-list cache on entry so the browser shows the
		// current state of disk, not whatever was cached last time we
		// were focused here.
		save.Refresh(&project.UI.SystemProject, saveOps)
		return
	case "tab":
		project.UI.Focus.Kind = model.FocusSession
		return
	}

	// Not a global hotkey — route to the focused device.
	switch project.UI.Focus.Kind {
	case model.FocusTrack:
		idx := project.UI.Focus.Track
		track := project.Tracks[idx]
		if track == nil {
			return
		}
		track.Lock()
		switch track.Type {
		case model.DeviceDrum:
			drum.HandleKey(track, project, out, key)
		case model.DevicePiano:
			piano.HandleKey(track, project, out, key)
		case model.DeviceMetropolix:
			metropolix.HandleKey(track, project, out, key)
		default:
			empty.HandleKey(project, out, key)
		}
		track.Unlock()
		markTrackDirty(project, idx)

	case model.FocusSession:
		// Session.HandleKey is currently a no-op, but dispatch for
		// symmetry. No dirty-mark: no state change possible.
		session.HandleKey(project, out, key)

	case model.FocusSettings:
		idx := project.UI.LastFocusedTrack
		tgt := project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		settings.HandleKey(project, out, idx, key)
		if tgt != nil {
			tgt.Unlock()
		}
		markTrackDirty(project, idx)

	case model.FocusProject:
		// An "enter" while focused on the project browser is a potential
		// load. If we're in edit-commit mode (IsEditing returns true),
		// enter commits a name rather than loading — no need for the
		// post-load Pause + recompile in that case.
		wasLoad := (key == "enter" || key == "return") && !save.IsEditing(&project.UI.SystemProject)
		save.HandleKey(&project.UI.SystemProject, project, saveOps, out, key)
		if wasLoad {
			// Project contents may have been replaced. Per DESIGN §4.5:
			// stop playback and recompile every track's currently-playing
			// pattern so playback has something to read on Play.
			Pause(project)
			ResetCursors(project)
			for i := 0; i < 8; i++ {
				markPlayingDirty(project, i)
			}
		}
	}
}

// markTrackDirty trashes both Machine slots of the edited pattern on the
// named track and signals the compiler to refill them. The "edited
// pattern" is resolved from the device's EditingPatternIdx for device
// handlers, or from the just-queued pattern for session handlers — we
// pass the pattern index in explicitly via markPatternDirty below.
//
// This wrapper figures out the editing pattern for the focused-device
// case, which is what the pad/key/midi handlers want.
func markTrackDirty(project *model.Project, trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := project.Tracks[trackIdx]
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
	markPatternDirty(project, trackIdx, patternIdx)
}

// markPlayingDirty is like markTrackDirty but targets the currently-
// playing pattern (Schedule.Playing) rather than the editing one. Used
// after project load to prime the Machine slots playback will read from.
func markPlayingDirty(project *model.Project, trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := project.Tracks[trackIdx]
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
	markPatternDirty(project, trackIdx, patternIdx)
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
func markPatternDirty(project *model.Project, trackIdx, patternIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	if patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	track := project.Tracks[trackIdx]
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
		case project.Compile.Channel <- model.CompileRequest{Track: trackIdx, Pattern: patternIdx, Slot: slot}:
		default:
			debug.Log("input", "compileCh full, dropping compile request for track=%d pattern=%d slot=%d", trackIdx, patternIdx, slot)
		}
	}
}
