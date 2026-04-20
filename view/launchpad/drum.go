package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drum-view colors. The old launchpad render used pulse/blink channels; the
// new LED type is pure RGB so everything is static (what-you-see-is-what-you-
// get). Bright colors scale to the step's velocity at render time.
var (
	drumStepOff      = LED{R: 20, G: 20, B: 20}    // dim grey: inactive step
	drumStepOn       = LED{R: 255, G: 100, B: 0}   // orange: active step
	drumLaneMarker   = LED{R: 0, G: 100, B: 255}   // blue: selected-lane indicator
	drumPlayheadCol  = LED{R: 255, G: 255, B: 255} // white: playing-step column overlay
	drumLaneHasHits  = LED{R: 100, G: 40, B: 80}   // magenta-ish: lane has any active step
	drumLaneEmpty    = LED{R: 30, G: 12, B: 22}    // dim magenta: lane is empty
	drumLaneSelected = LED{R: 255, G: 255, B: 255} // white: currently selected lane
	drumCmdClearLane = LED{R: 180, G: 50, B: 50}   // red-ish: clear-lane command
	drumCmdClearPat  = LED{R: 220, G: 40, B: 40}   // brighter red: clear-pattern command
	drumCmdRecordOff = LED{R: 60, G: 0, B: 0}      // dim red: record-arm (off)
	drumCmdRecordOn  = LED{R: 255, G: 0, B: 0}     // bright red: record-arm (on)
)

// drumPad handles a pad press or release on the drum view.
//
// Layout (matches the old sequencer/drum.go HandlePad, minus preview/copy/
// length command rows which the new model doesn't support yet):
//
//	rows 4-7 (top half):       step toggles for the selected lane.
//	                           pad at (row, col) == step (7-row)*8 + col.
//	rows 0-3, cols 0-3:        lane select (4x4 = 16 lanes).
//	                           pad at (row, col) == lane row*4 + col.
//	rows 0-3, cols 4-7:        command pads (see switch below).
//
// Caller (HandlePad in launchpad.go) holds track.Lock and calls
// MarkTrackDirty after this returns; we just mutate spec.
//
// TODO: paging. This shows lanes 0-15 directly on the 4x4 lane grid and
// steps 0-31 on the 4x8 step grid — there's no view offset yet. A later
// pass can add horizontal paging if 32 steps need finer-grained editing.
func drumPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
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

	// Bottom-left 4x4: lane select (+ preview + record-arm edit).
	if row < 4 && col < 4 {
		laneIdx := row*4 + col
		if laneIdx < 0 || laneIdx >= 16 {
			return
		}
		state.SelectedNoteIdx = laneIdx

		// Preview: send this lane's drum note directly. Always — pad taps
		// on a lane-select pad are audible regardless of record-arm.
		kit := devices.GetKit(track.Kit)
		_ = out.Send(track.PortName, track.Channel, midi.Event{
			Type:     midi.NoteOn,
			Note:     kit.Notes[laneIdx],
			Velocity: 100,
		})

		// Record-arm: also write the hit into the editing pattern at the
		// cursor. Playback tick isn't plumbed to input today, so we use
		// the cursor as the write-head (same choice as drumMIDI).
		if state.Recording {
			s := &pat.Notes[laneIdx].Steps[state.Cursor]
			s.Active = true
			s.Velocity = 100
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
		case row == 3 && col == 5: // record arm toggle
			state.Recording = !state.Recording
		}
		return
	}
}

// drumLEDs renders the 8x8 LED frame for the drum view.
//
// Layout mirrors drumPad:
//
//	rows 4-7:            steps 0-31 of the selected lane. Active steps
//	                     scale by velocity; inactive steps dim. The
//	                     currently-playing step lights white.
//	rows 0-3, cols 0-3:  lane grid. Selected lane white; lanes with any
//	                     active step in the editing pattern get the
//	                     "has hits" tint; empty lanes dim.
//	rows 0-3, cols 4-7:  command pads — clear-lane, clear-pattern,
//	                     record-arm (lit when armed).
func drumLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil || track.Drum == nil {
		return nil
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	playingStep := drumPlayingStep(track, project)

	leds := make([]LED, 0, 64)

	// Step grid on rows 4..7 (stepIdx = (7-row)*8 + col).
	selectedLane := &pat.Notes[state.SelectedNoteIdx]
	for row := 4; row <= 7; row++ {
		for col := 0; col < 8; col++ {
			stepIdx := (7-row)*8 + col
			var c LED
			step := &selectedLane.Steps[stepIdx]
			if step.Active {
				c = drumScaleLED(drumStepOn, step.Velocity)
			} else {
				c = drumStepOff
			}
			if playingStep >= 0 && stepIdx == playingStep {
				c = drumPlayheadCol
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
				c = drumLaneSelected
			case drumLaneHasAnyHit(&pat.Notes[laneIdx]):
				c = drumLaneHasHits
			default:
				c = drumLaneEmpty
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	// Selected-lane row indicator on the right side: light col 4 on the
	// row that corresponds to the selected lane's row-group. Cheap visual
	// hint that the "top half shows steps for THIS lane".
	selRow := state.SelectedNoteIdx / 4
	leds = append(leds, LED{Row: selRow, Col: 4, R: drumLaneMarker.R, G: drumLaneMarker.G, B: drumLaneMarker.B})

	// Command pads on rows 0..3, cols 5..7.
	// row 0, col 5: clear lane
	leds = append(leds, LED{Row: 0, Col: 5, R: drumCmdClearLane.R, G: drumCmdClearLane.G, B: drumCmdClearLane.B})
	// row 0, col 6: clear pattern
	leds = append(leds, LED{Row: 0, Col: 6, R: drumCmdClearPat.R, G: drumCmdClearPat.G, B: drumCmdClearPat.B})
	// row 3, col 5: record arm (lit brightly when armed, dim when not)
	rec := drumCmdRecordOff
	if state.Recording {
		rec = drumCmdRecordOn
	}
	leds = append(leds, LED{Row: 3, Col: 5, R: rec.R, G: rec.G, B: rec.B})

	return leds
}

// drumScaleLED scales an LED color by a 0..127 velocity. Velocity 0 yields
// the dim "off" color; velocity 127 yields the full-brightness color.
func drumScaleLED(base LED, velocity uint8) LED {
	v := float64(velocity) / 127.0
	if v < 0.2 {
		v = 0.2 // even very-quiet hits should be visible
	}
	return LED{
		R: uint8(float64(base.R) * v),
		G: uint8(float64(base.G) * v),
		B: uint8(float64(base.B) * v),
	}
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
// Computation (see task brief): pos = globalTick - cursor.T0Tick; reduce
// into the current pattern's compiled length; divide by ticksPerStep.
func drumPlayingStep(track *model.Track, project *model.Project) int {
	if track.Drum == nil {
		return -1
	}
	// Resolve the playing pattern's compiled slot. If it's nil we haven't
	// compiled yet — return -1 so the LED highlight skips the playhead.
	playingIdx := track.Drum.Schedule.Playing
	if playingIdx < 0 || playingIdx >= len(track.Drum.Patterns) {
		return -1
	}
	playingPat := track.Drum.Patterns[playingIdx]
	if playingPat == nil {
		return -1
	}
	// Find the track's cursor. The project has Cursors[8] — we need the
	// track's own index, which we don't have as a parameter here. Lift it
	// from the UI focus since drumLEDs is only called when this drum is
	// the focused track.
	trackIdx := project.UI.Focus.Track
	if trackIdx < 0 || trackIdx >= 8 || project.Tracks[trackIdx] != track {
		// Focus doesn't match — fall back to a linear search. Cheap; 8
		// tracks max.
		trackIdx = -1
		for i := 0; i < 8; i++ {
			if project.Tracks[i] == track {
				trackIdx = i
				break
			}
		}
		if trackIdx < 0 {
			return -1
		}
	}
	cursor := project.Playback.Cursors[trackIdx]
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
	if track.Drum.EditingPatternIdx != playingIdx {
		return -1
	}
	if stepIdx < 0 || stepIdx >= 32 {
		return -1
	}
	return stepIdx
}
