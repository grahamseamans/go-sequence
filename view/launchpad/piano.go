package launchpad

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// pianoViewScales is beats-per-column for each zoom level. Mirrors the table
// in controller/devices/piano.
var pianoViewScales = []float64{
	0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0, 2.0, 4.0,
}

// minNoteDurationBeats is the smallest duration a note may have.
const pianoMinNoteDurationBeats = 0.25

// pianoPad handles a pad press or release on the piano view.
//
// Ported from controller/devices/piano.HandlePad. Releases (down == false)
// are no-ops. Semantics: the 8x8 grid renders a window into the piano roll
// centered on (CenterBeat, CenterPitch) at the current ViewScale zoom. A tap
// on a cell either selects an existing note under that cell or adds a new
// note at that beat/pitch.
func pianoPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]

	basePitch := int(state.CenterPitch) - 4
	viewScale := pianoViewScale(state.ViewScale)
	startBeat := state.CenterBeat - 4*viewScale

	pitch := uint8(basePitch + row)
	beat := startBeat + float64(col)*viewScale
	beatEnd := beat + viewScale

	if beat < 0 || beat >= pat.Length || int(basePitch+row) < 0 || int(basePitch+row) > 127 {
		return
	}

	// Cell hit existing note → select it.
	for i := range pat.Notes {
		n := &pat.Notes[i]
		if n.Pitch != pitch {
			continue
		}
		noteEnd := n.Start + n.Duration
		if n.Start < beatEnd && noteEnd > beat {
			state.SelectedNote = i
			pianoCenterOnSelection(state)
			return
		}
	}

	// Empty cell → add a new note the width of one column, clamped to the
	// minimum duration.
	dur := viewScale
	if dur < pianoMinNoteDurationBeats {
		dur = pianoMinNoteDurationBeats
	}
	pianoAddNote(pat, beat, dur, pitch, 100)
	state.SelectedNote = len(pat.Notes) - 1
	pianoSortNotes(pat, state)
	pianoCenterOnSelection(state)
}

// pianoLEDs returns the LED frame for the piano view. Stub.
func pianoLEDs(track *model.Track, project *model.Project) []LED { return nil }

// pianoViewScale returns beats-per-column for the given ViewScale index, clamped.
func pianoViewScale(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoViewScales) {
		idx = len(pianoViewScales) - 1
	}
	return pianoViewScales[idx]
}

// pianoAddNote appends a note to the pattern, clamping to pattern bounds.
// Drops the note if it can't fit.
func pianoAddNote(pat *devices.PianoPattern, start, duration float64, pitch, velocity uint8) {
	if duration < pianoMinNoteDurationBeats {
		duration = pianoMinNoteDurationBeats
	}
	if start < 0 {
		start = 0
	}
	if start+duration > pat.Length {
		return
	}
	pat.Notes = append(pat.Notes, devices.NoteEvent{
		Start: start, Duration: duration, Pitch: pitch, Velocity: velocity,
	})
}

// pianoCenterOnSelection scrolls the viewport to show the selected note.
func pianoCenterOnSelection(state *devices.Piano) {
	pat := state.Patterns[state.EditingPatternIdx]
	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[state.SelectedNote]
	state.CenterBeat = n.Start
	state.CenterPitch = float64(n.Pitch)
}

// pianoSortNotes keeps the pattern's Notes sorted by start time ascending,
// preserving the user's current selection by matching start+pitch after the
// sort.
func pianoSortNotes(pat *devices.PianoPattern, state *devices.Piano) {
	var selected *devices.NoteEvent
	if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
		n := pat.Notes[state.SelectedNote]
		selected = &n
	}

	sort.SliceStable(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})

	if selected == nil {
		return
	}
	for i, n := range pat.Notes {
		if n.Start == selected.Start && n.Pitch == selected.Pitch {
			state.SelectedNote = i
			return
		}
	}
}
