package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drumPad handles a pad press or release on the drum view.
//
// Signature ports from controller/devices/drum.HandlePad: takes `*model.Track`
// so the handler can reach the track's PortName, Channel, and Kit for preview
// MIDI sends. Releases (down == false) are no-ops — the drum device is
// press-only today.
func drumPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]

	// Top 4 rows (rows 4-7): step toggle for the selected lane.
	if row >= 4 && row <= 7 {
		stepIdx := (7-row)*8 + col
		if stepIdx < 0 || stepIdx >= 32 {
			return
		}
		drumToggleStep(pat, state.SelectedNoteIdx, stepIdx)
		state.Cursor = stepIdx
		return
	}

	// Bottom-left 4x4: note-lane select (and recording/preview pad hits).
	if row < 4 && col < 4 {
		laneIdx := row*4 + col
		if laneIdx < 0 || laneIdx >= 16 {
			return
		}
		state.SelectedNoteIdx = laneIdx
		if state.Cursor >= 32 {
			state.Cursor = 31
		}

		// Preview: send the lane's drum note directly. No queueing, no
		// compile — the hit is audible immediately.
		kit := devices.GetKit(track.Kit)
		note := kit.Notes[laneIdx]
		_ = out.Send(track.PortName, track.Channel, midi.Event{
			Type:     midi.NoteOn,
			Note:     note,
			Velocity: 100,
		})

		// Record-arm + pad tap = write this hit into the editing pattern at
		// the cursor position. (Old behavior used currentStep at live tick;
		// we don't have playback position here, so use the cursor as the
		// write head. Revisit when we can read playback tick.)
		if state.Recording {
			drumSetStep(pat, laneIdx, state.Cursor, 100)
		}
		return
	}

	// Bottom-right 4x4: command pads.
	if row < 4 && col >= 4 {
		switch {
		case row == 0 && col == 4: // clear lane
			drumClearLane(pat, state.SelectedNoteIdx)
		case row == 0 && col == 5: // clear pattern
			drumClearPattern(pat)
		case row == 3 && col == 5: // record toggle
			state.Recording = !state.Recording
		}
		return
	}
}

// drumLEDs returns the LED frame for the drum view. Stubbed for now;
// rendering will be ported from sequencer/drum.go in a later pass.
func drumLEDs(track *model.Track, project *model.Project) []LED { return nil }

// --- spec mutators (package-private; callers hold track.mu) ---

// drumToggleStep flips a step's Active bit, resetting velocity when turning on.
func drumToggleStep(pat *devices.DrumPattern, lane, step int) {
	if lane < 0 || lane >= 16 || step < 0 || step >= 32 {
		return
	}
	s := &pat.Notes[lane].Steps[step]
	s.Active = !s.Active
	if s.Active && s.Velocity == 0 {
		s.Velocity = 100
	}
}

// drumSetStep forces a step Active with the given velocity.
func drumSetStep(pat *devices.DrumPattern, lane, step int, velocity uint8) {
	if lane < 0 || lane >= 16 || step < 0 || step >= 32 {
		return
	}
	s := &pat.Notes[lane].Steps[step]
	s.Active = true
	s.Velocity = velocity
}

// drumClearLane zeroes every step in one lane.
func drumClearLane(pat *devices.DrumPattern, lane int) {
	if lane < 0 || lane >= 16 {
		return
	}
	for i := range pat.Notes[lane].Steps {
		pat.Notes[lane].Steps[i] = devices.DrumStep{}
	}
}

// drumClearPattern zeroes every step in every lane.
func drumClearPattern(pat *devices.DrumPattern) {
	for l := range pat.Notes {
		for s := range pat.Notes[l].Steps {
			pat.Notes[l].Steps[s] = devices.DrumStep{}
		}
	}
}
