package tui

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drumKey handles a keyboard event on the drum view.
//
// Signature ports from controller/devices/drum.HandleKey: takes `*model.Track`.
// Mutates spec; caller holds track.mu.
func drumKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]

	switch key {
	case "h", "left":
		if state.Cursor > 0 {
			state.Cursor--
		}
	case "l", "right":
		if state.Cursor < 31 {
			state.Cursor++
		}
	case "j", "down":
		if state.SelectedNoteIdx < 15 {
			state.SelectedNoteIdx++
		}
	case "k", "up":
		if state.SelectedNoteIdx > 0 {
			state.SelectedNoteIdx--
		}
	case " ":
		drumToggleStep(pat, state.SelectedNoteIdx, state.Cursor)
	case "c":
		drumClearLane(pat, state.SelectedNoteIdx)
	case "C":
		drumClearPattern(pat)
	case "<", ",":
		if state.EditingPatternIdx > 0 {
			state.EditingPatternIdx--
		}
	case ">", ".":
		if state.EditingPatternIdx < devices.NumPatterns-1 {
			state.EditingPatternIdx++
		}
	case "r", "R":
		state.Recording = !state.Recording
	}
}

// drumRender returns the TUI view for the drum device. Stub; the rendering
// body will be ported from sequencer/drum.go in a later pass.
func drumRender(track *model.Track, project *model.Project) string { return "" }

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
