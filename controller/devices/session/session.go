package session

import (
	"go-sequence/controller/surface"
	"go-sequence/midi"
	"go-sequence/model"
)

// Package session is the project-level "clip launcher" device: an 8x8 grid
// where columns are tracks and rows are patterns. Tapping a cell queues that
// pattern on that track. Session owns no state of its own — it reads and
// writes project.Tracks[*].<Device>.Schedule.
//
// Signature diverges from a per-track device: there is no `state` parameter
// and no Compile function. Session does not emit MIDI events; it only mutates
// per-track Schedule.Queued.

// HandlePad queues a pattern on a track.
//
// row in [0,7] is the pattern index (row 0 = pattern 0, top of the grid).
// col in [0,7] is the track index.
//
// Ported note: the old sequencer/session.go used `row = track, col =
// pattern`. That was inconsistent with the rest of the UI which treats rows
// as vertical and columns as horizontal. The task description asks for
// `row = track, col = pattern`, so that's what we use here: the row picks
// the track and the col picks the pattern. Revisit in step 5 when view/
// rewires and we can confirm the visual mapping against the Launchpad.
func HandlePad(project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if row < 0 || row >= 8 || col < 0 || col >= 8 {
		return
	}
	if project == nil {
		return
	}
	track := project.Tracks[row]
	if track == nil {
		return
	}

	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil {
			track.Drum.Schedule.Queued = col
		}
	case model.DevicePiano:
		if track.Piano != nil {
			track.Piano.Schedule.Queued = col
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil {
			track.Metropolix.Schedule.Queued = col
		}
	case model.DeviceNone:
		// No device on this track — nothing to queue.
	}
}

// HandleKey handles keyboard input on the session view.
//
// The old sequencer/session.go had an internal (cursorRow, cursorCol) and
// used h/j/k/l/space/enter to drive a soft cursor. Per DESIGN §4.8 focus
// lives on model.UIState, not per-device, so cursor-style UI state is
// dropped here. If we want a keyboard launcher later, it should come back
// via model.UIState, not on a hidden Session field.
func HandleKey(project *model.Project, out midi.ToExternal, key string) {
	// Intentional no-op until session-cursor state is added to
	// model.UIState.
}

// Render is a stub; step 5 will port the TUI view from sequencer/session.go.
// TODO(step5): port rendering from sequencer/session.go when view/ rewires.
func Render(project *model.Project) string { return "" }

// RenderLEDs is a stub; step 5 will port the LED layout from sequencer/session.go.
// TODO(step5): port rendering from sequencer/session.go when view/ rewires.
func RenderLEDs(project *model.Project) []surface.LED { return nil }
