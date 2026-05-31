// Package launchpad is the launchpad view surface: pad input loop, LED
// frame renderer, and per-domain dispatchers that translate pad events
// into controller calls.
//
// Architecture (DESIGN §3.6 / §3.7): views own UI input loops. RunRoutine
// is the goroutine entry point; it reads from surf.PadEvents() and
// dispatches each event to the focused domain's pad handler. The focused
// domain is read from project.UI.Focus.
package launchpad

import (
	"context"
	"time"

	"go-sequence/controller"
	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// RunRoutine is the launchpad goroutine. Reads surf.PadEvents(), dispatches
// to per-domain handlers based on project.UI.Focus. Blocks until ctx is done.
//
// Invoke from main in its own goroutine. Shutdown is cooperative: ctx.Done
// wins over the pad channel, so a canceled context returns promptly even
// if pad events are still buffered.
func RunRoutine(ctx context.Context, project *model.Project, out midi.ToExternal, saveOps save.SaveOps, surf Surface) {
	padCh := surf.PadEvents()
	ledTicker := time.NewTicker(16 * time.Millisecond) // ~60 Hz
	defer ledTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-padCh:
			if !ok {
				return
			}
			HandlePad(project, out, saveOps, ev)
		case <-ledTicker.C:
			surf.SetLEDs(RenderLEDs(project, saveOps))
		}
	}
}

// HandlePad dispatches one pad event to the focused domain's handler. Also
// does the track.Lock + mark-dirty bookkeeping that was in the old
// controller/input.go handlePad.
func HandlePad(project *model.Project, out midi.ToExternal, saveOps save.SaveOps, ev PadEvent) {
	switch project.UI.Focus.Kind {
	case model.FocusTrack:
		idx := project.UI.Focus.Track
		track := project.Tracks[idx]
		if track == nil {
			return
		}
		track.Lock()
		switch track.Device.(type) {
		case *devices.Drum:
			drumPad(track, project, out, ev.Row, ev.Col, ev.Down)
		case *devices.Looper:
			looperPad(track, project, out, ev)
		default:
			emptyPad(project, out, ev.Row, ev.Col, ev.Down)
		}
		track.Unlock()
		if ev.Down {
			controller.MarkTrackDirty(project, idx)
		}

	case model.FocusSession:
		// Right column (Col 8) = "play all" scene buttons: cue the pattern on
		// that row across every track at once. Press-only.
		if ev.Col == 8 {
			if ev.Down {
				controller.QueueScene(project, sessionRowPattern(ev.Row))
			}
			return
		}
		// Top control row (Row 8) is unused in the session view.
		if ev.Row == 8 {
			return
		}
		// 8x8 grid: col = instrument, row = pattern. The toggle, the per-track
		// lock, and the dirty-mark all live in controller.ToggleQueue (via
		// sessionPad), shared with the TUI.
		sessionPad(project, out, ev.Row, ev.Col, ev.Down)

	case model.FocusSettings:
		// Settings edits fields on Tracks[Focus.Track]. Lock that track
		// around the handler.
		idx := project.UI.Focus.Track
		tgt := project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		settingsPad(project, out, idx, ev.Row, ev.Col, ev.Down)
		if tgt != nil {
			tgt.Unlock()
		}
		if ev.Down {
			controller.MarkTrackDirty(project, idx)
		}

	case model.FocusProject:
		// Project browser's pad handler is a no-op today, but keep the
		// dispatch symmetric so a future pad UX lands here without ceremony.
		projectPad(&project.UI.SystemProject, project, saveOps, out, ev.Row, ev.Col, ev.Down)
	}
}

// RenderLEDs returns the LED frame for the focused domain. Called by the
// top-level tick-render loop (frame renderer).
func RenderLEDs(project *model.Project, saveOps save.SaveOps) []LED {
	switch project.UI.Focus.Kind {
	case model.FocusTrack:
		idx := project.UI.Focus.Track
		track := project.Tracks[idx]
		if track == nil {
			return nil
		}
		switch track.Device.(type) {
		case *devices.Drum:
			return drumLEDs(track, project)
		case *devices.Looper:
			return looperLEDs(track, project)
		default:
			return emptyLEDs(project)
		}
	case model.FocusSession:
		return sessionLEDs(project)
	case model.FocusSettings:
		return settingsLEDs(project)
	case model.FocusProject:
		return projectLEDs(&project.UI.SystemProject, project, saveOps)
	}
	return nil
}
