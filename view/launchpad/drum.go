package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drum-view colors. Ported from the old sequencer/drum.go RenderLEDs. The
// old code drove launchpad pulse/flash channels via midi.ChannelPulse for
// things like the playhead; the new LED type is pure RGB (what-you-see-is-
// what-you-get), so anything that used to pulse is a bright static color
// here. If the launchpad-native pulse gets plumbed through later, these
// cells are the ones to promote.
var (
	drumStepsColor     = LED{R: 234, G: 73, B: 116}  // active step
	drumStepsEmpty     = LED{R: 80, G: 30, B: 50}    // inactive step
	drumNoteHasContent = LED{R: 148, G: 18, B: 126}  // lane has any hit
	drumNoteEmpty      = LED{R: 40, G: 10, B: 30}    // lane is empty
	drumNoteSelected   = LED{R: 255, G: 255, B: 255} // selected lane (white)
	drumCommandsColor  = LED{R: 253, G: 157, B: 110} // default command pad
	drumPlayheadColor  = LED{R: 255, G: 255, B: 255} // playhead (was pulse)
	drumPreviewActive  = LED{R: 0, G: 255, B: 0}     // preview armed
	drumRecordActive   = LED{R: 255, G: 0, B: 0}     // record armed
	drumOffColor       = LED{R: 0, G: 0, B: 0}       // beyond pattern length
)

// drumPad handles a pad press or release on the drum view.
//
// Layout (ported verbatim from old sequencer/drum.go HandlePad — see
// renderLaunchpadHelp there for the legend):
//
//	rows 4-7:           step toggles for the selected lane.
//	                    pad at (row, col) == step (7-row)*8 + col.
//	rows 0-3, cols 0-3: lane select (4x4 = 16 lanes).
//	                    pad at (row, col) == lane row*4 + col.
//	                    If Preview: also sends a live preview note.
//	                    If Recording && Playing: also toggles the step
//	                    at the current playhead.
//	rows 0-3, cols 4-7: command pads.
//	                    (0, 4) = clear lane (no confirm here; pad UX)
//	                    (0, 5) = clear pattern (no confirm here)
//	                    (3, 4) = preview toggle
//	                    (3, 5) = record toggle
//	                    Other command cells (copy/paste/nudge/length etc.)
//	                    are left as visual hints only — not wired, matching
//	                    the old code which also stopped short on those.
//
// Caller (HandlePad in launchpad.go) holds track.Lock and calls
// MarkTrackDirty after this returns; we just mutate spec.
func drumPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil {
		return
	}
	state, ok := track.Device.(*devices.Drum)
	if !ok || state == nil {
		return
	}
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Top 4 rows: step toggle for the selected lane.
	if row >= 4 && row <= 7 {
		stepIdx := (7-row)*8 + col
		if stepIdx < 0 || stepIdx >= 32 {
			return
		}
		s := &pat.Notes[state.SelectedNoteIdx].Steps[stepIdx]
		s.Active = !s.Active
		if s.Active && s.Velocity == 0 {
			s.Velocity = 100
		}
		state.Cursor = stepIdx
		return
	}

	// Bottom-left 4x4: lane select (+ preview + record-while-playing).
	if row < 4 && col < 4 {
		laneIdx := row*4 + col
		if laneIdx < 0 || laneIdx >= 16 {
			return
		}
		state.SelectedNoteIdx = laneIdx

		// Preview: send this lane's drum note if Preview mode is armed.
		if state.Preview {
			kit := devices.GetKit(track.Kit)
			_ = out.Send(track.PortName, track.Channel, midi.Event{
				Type:     midi.NoteOn,
				Note:     kit.Notes[laneIdx],
				Velocity: 100,
			})
		}

		// Record-while-playing: if recording and transport is running, toggle
		// the step at the current playhead. Matches the old UX: tapping a
		// lane pad while armed "records" the hit at the live playback
		// position (derived from globalTick, not from the edit cursor).
		if state.Recording && project.Playback.Playing.Load() {
			playingStep := drumPlayingStep(track, project)
			if playingStep >= 0 && playingStep < 32 {
				s := &pat.Notes[laneIdx].Steps[playingStep]
				s.Active = !s.Active
				if s.Active && s.Velocity == 0 {
					s.Velocity = 100
				}
			}
		}
		return
	}

	// Bottom-right 4x4: command pads.
	if row < 4 && col >= 4 {
		switch {
		case row == 0 && col == 4: // clear lane
			for i := range pat.Notes[state.SelectedNoteIdx].Steps {
				pat.Notes[state.SelectedNoteIdx].Steps[i] = devices.DrumStep{}
			}
		case row == 0 && col == 5: // clear pattern (all lanes)
			for l := range pat.Notes {
				for s := range pat.Notes[l].Steps {
					pat.Notes[l].Steps[s] = devices.DrumStep{}
				}
			}
		case row == 3 && col == 4: // preview toggle
			state.Preview = !state.Preview
		case row == 3 && col == 5: // record arm toggle
			state.Recording = !state.Recording
		}
		return
	}
}

// drumLEDs renders the 8x8 LED frame for the drum view.
//
// Layout mirrors drumPad (ported from old sequencer/drum.go RenderLEDs):
//
//	rows 4-7:            steps 0-31 of the selected lane. Active steps
//	                     lit, inactive dim, playing step white, steps
//	                     beyond the pattern length blacked out.
//	rows 0-3, cols 0-3:  lane grid. Selected lane white; lanes with any
//	                     active step tinted "has content"; empty lanes
//	                     very dim.
//	rows 0-3, cols 4-7:  command pads — clear-lane, clear-pattern, plus
//	                     a block of reserved cells lit with the default
//	                     command color. Preview (3,4) and Record (3,5)
//	                     light bright green/red when armed.
func drumLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil {
		return nil
	}
	state, ok := track.Device.(*devices.Drum)
	if !ok || state == nil {
		return nil
	}
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	playingStep := drumPlayingStep(track, project)
	patternSteps := pat.Length / (devices.PPQ / 4)
	if patternSteps < 1 {
		patternSteps = 1
	}
	if patternSteps > 32 {
		patternSteps = 32
	}

	leds := make([]LED, 0, 64)

	// Step grid on rows 4..7 (stepIdx = (7-row)*8 + col).
	selectedLane := &pat.Notes[state.SelectedNoteIdx]
	for row := 4; row <= 7; row++ {
		for col := 0; col < 8; col++ {
			stepIdx := (7-row)*8 + col
			var c LED
			switch {
			case stepIdx >= patternSteps:
				c = drumOffColor // beyond pattern length
			case stepIdx == playingStep:
				c = drumPlayheadColor // was pulse in old code
			case selectedLane.Steps[stepIdx].Active:
				c = drumStepsColor
			default:
				c = drumStepsEmpty
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	// Lane grid on rows 0..3, cols 0..3 (laneIdx = row*4 + col).
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			laneIdx := row*4 + col
			var c LED
			switch {
			case laneIdx == state.SelectedNoteIdx:
				c = drumNoteSelected
			case drumLaneHasAnyHit(&pat.Notes[laneIdx]):
				c = drumNoteHasContent
			default:
				c = drumNoteEmpty
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	// Command pads on rows 0..3, cols 4..7. Default to the command color;
	// the two toggles (preview, record) override bright when armed.
	for row := 0; row < 4; row++ {
		for col := 4; col < 8; col++ {
			c := drumCommandsColor
			if row == 3 && col == 4 && state.Preview {
				c = drumPreviewActive
			}
			if row == 3 && col == 5 && state.Recording {
				c = drumRecordActive
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	return leds
}

// drumLaneHasAnyHit returns true iff any step in the lane is active.
func drumLaneHasAnyHit(lane *devices.NoteLane) bool {
	for i := range lane.Steps {
		if lane.Steps[i].Active {
			return true
		}
	}
	return false
}

// drumPlayingStep returns the step index (0..31) currently being played on
// this track, or -1 if the pattern hasn't compiled yet or playback is idle.
//
// Unlike the TUI's drumPlayingStep (which hides the playhead when the user
// is editing a different pattern than the one playing), this returns a
// step for any editing-pattern state — the launchpad step row shows the
// editing pattern, and a playing-but-not-editing pattern naturally has no
// visible playhead because the row shows a different lane set of steps.
func drumPlayingStep(track *model.Track, project *model.Project) int {
	state, ok := track.Device.(*devices.Drum)
	if !ok || state == nil {
		return -1
	}
	// Resolve the track's index in project.Tracks; prefer the UI focus as
	// a fast path, fall back to linear search if it doesn't line up.
	trackIdx := project.UI.Focus.Track
	if trackIdx < 0 || trackIdx >= 8 || project.Tracks[trackIdx] != track {
		trackIdx = project.TrackIndex(track)
		if trackIdx < 0 {
			return -1
		}
	}
	cursor := project.Playback.Cursors[trackIdx]
	playingIdx := cursor.CurrentPattern
	if playingIdx < 0 || playingIdx >= len(state.Patterns) {
		return -1
	}
	playingPat := state.Patterns[playingIdx]
	if playingPat == nil {
		return -1
	}
	slot := cursor.CurrentSlot
	if slot < 0 || slot > 1 {
		slot = 0
	}
	m := playingPat.Machine[slot].Load()
	if m == nil || m.Length <= 0 {
		return -1
	}
	globalTick := project.Playback.Tick.Load()
	pos := globalTick - cursor.T0Tick
	posInPattern := ((pos % m.Length) + m.Length) % m.Length
	stepIdx := int(posInPattern / (devices.PPQ / 4))
	// Editing pattern may differ from playing pattern; the highlight only
	// makes sense on the playing pattern's step grid. If we're editing a
	// different pattern, hide the playhead.
	if state.EditingPatternIdx != playingIdx {
		return -1
	}
	if stepIdx < 0 || stepIdx >= 32 {
		return -1
	}
	return stepIdx
}
