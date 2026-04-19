package tui

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Piano view tables (ported from controller/devices/piano).

// pianoViewScales is beats-per-column for each zoom level.
var pianoViewScales = []float64{
	0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0, 2.0, 4.0,
}

// pianoEditHorizSteps is the beat delta for each horizontal-edit sensitivity.
var pianoEditHorizSteps = []float64{
	0.015625, 0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0,
}

// pianoEditVertSteps is the semitone delta for each vertical-edit sensitivity.
var pianoEditVertSteps = []int{1, 12}

const (
	pianoViewSmushed = 12
	pianoViewSpread  = 24
)

// pianoMinNoteDurationBeats is the smallest duration a note may have.
const pianoMinNoteDurationBeats = 0.25

// pianoKey handles a keyboard event on the piano view.
//
// Ported from controller/devices/piano.HandleKey. Caller holds track.mu.
func pianoKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	editH := pianoEditHoriz(state.EditHoriz)
	editV := pianoEditVert(state.EditVert)

	switch key {
	case "h", "left":
		pianoSelectByTime(state, -1)
	case "l", "right":
		pianoSelectByTime(state, 1)
	case "j", "down":
		pianoSelectByPitch(state, -1)
	case "k", "up":
		pianoSelectByPitch(state, 1)

	case "y":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			delta := -editH
			if n.Start+delta < 0 {
				delta = -n.Start
			}
			pianoMoveNoteTime(pat, state.SelectedNote, delta)
			pianoCenterOnSelection(state)
		}
	case "o":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			delta := editH
			if n.Start+n.Duration+delta > pat.Length {
				delta = pat.Length - n.Start - n.Duration
			}
			pianoMoveNoteTime(pat, state.SelectedNote, delta)
			pianoCenterOnSelection(state)
		}
	case "u":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			pianoMoveNotePitch(pat, state.SelectedNote, -editV)
			pianoCenterOnSelection(state)
		}
	case "i":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			pianoMoveNotePitch(pat, state.SelectedNote, editV)
			pianoCenterOnSelection(state)
		}

	case "n":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			if n.Duration > editH {
				pianoSetNoteDuration(pat, state.SelectedNote, n.Duration-editH)
			}
		}
	case "m":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				pianoSetNoteDuration(pat, state.SelectedNote, n.Duration+editH)
			}
		}

	case "q":
		if state.ViewScale < len(pianoViewScales)-1 {
			state.ViewScale++
		}
	case "w":
		if state.ViewScale > 0 {
			state.ViewScale--
		}
	case "a":
		state.ViewRows = pianoViewSmushed
	case "s":
		state.ViewRows = pianoViewSpread

	case "d":
		if state.EditHoriz < len(pianoEditHorizSteps)-1 {
			state.EditHoriz++
		}
	case "f":
		if state.EditHoriz > 0 {
			state.EditHoriz--
		}
	case "e":
		if state.EditVert < len(pianoEditVertSteps)-1 {
			state.EditVert++
		}
	case "r":
		if state.EditVert > 0 {
			state.EditVert--
		}

	case " ":
		dur := pianoEditHoriz(state.EditHoriz) * 4
		pianoAddNote(pat, state.CenterBeat, dur, uint8(state.CenterPitch), 100)
		state.SelectedNote = len(pat.Notes) - 1
		pianoCenterOnSelection(state)

	case "x":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			pianoDeleteNote(pat, state.SelectedNote)
			if state.SelectedNote >= len(pat.Notes) {
				state.SelectedNote = len(pat.Notes) - 1
			}
			if state.SelectedNote >= 0 {
				pianoCenterOnSelection(state)
			}
		}

	case "[":
		if pat.Length > 1.0 {
			pianoSetPatternLength(pat, pat.Length-1.0)
		}
	case "]":
		if pat.Length < 64.0 {
			pianoSetPatternLength(pat, pat.Length+1.0)
		}

	case "c":
		pianoClearPattern(pat)
		state.SelectedNote = -1

	case "<":
		if state.EditingPatternIdx > 0 {
			state.EditingPatternIdx--
			state.SelectedNote = -1
		}
	case ">", ".":
		if state.EditingPatternIdx < devices.NumPatterns-1 {
			state.EditingPatternIdx++
			state.SelectedNote = -1
		}

	case "R":
		state.Recording = !state.Recording
	}

	pianoSortNotes(pat, state)
}

// pianoRender returns the TUI view for the piano device. Stub.
func pianoRender(track *model.Track, project *model.Project) string { return "" }

// --- lookup helpers (safe index into tables) ---

func pianoViewScale(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoViewScales) {
		idx = len(pianoViewScales) - 1
	}
	return pianoViewScales[idx]
}

func pianoEditHoriz(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditHorizSteps) {
		idx = len(pianoEditHorizSteps) - 1
	}
	return pianoEditHorizSteps[idx]
}

func pianoEditVert(idx int) int {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditVertSteps) {
		idx = len(pianoEditVertSteps) - 1
	}
	return pianoEditVertSteps[idx]
}

// --- spec mutators (package-private; callers hold track.mu) ---

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

func pianoDeleteNote(pat *devices.PianoPattern, idx int) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	pat.Notes = append(pat.Notes[:idx], pat.Notes[idx+1:]...)
}

func pianoMoveNoteTime(pat *devices.PianoPattern, idx int, deltaBeat float64) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	n.Start += deltaBeat
	if n.Start < 0 {
		n.Start = 0
	}
	if n.Start+n.Duration > pat.Length {
		n.Start = pat.Length - n.Duration
	}
}

func pianoMoveNotePitch(pat *devices.PianoPattern, idx int, deltaSemitones int) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	target := int(n.Pitch) + deltaSemitones
	if target < 0 {
		target = 0
	}
	if target > 127 {
		target = 127
	}
	n.Pitch = uint8(target)
}

func pianoSetNoteDuration(pat *devices.PianoPattern, idx int, duration float64) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	if duration < pianoMinNoteDurationBeats {
		duration = pianoMinNoteDurationBeats
	}
	if n.Start+duration > pat.Length {
		duration = pat.Length - n.Start
	}
	n.Duration = duration
}

func pianoSetPatternLength(pat *devices.PianoPattern, beats float64) {
	if beats < 1.0 || beats > 64.0 {
		return
	}
	pat.Length = beats
}

func pianoClearPattern(pat *devices.PianoPattern) {
	pat.Notes = pat.Notes[:0]
}

// --- selection / viewport helpers ---

func pianoCenterOnSelection(state *devices.Piano) {
	pat := state.Patterns[state.EditingPatternIdx]
	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[state.SelectedNote]
	state.CenterBeat = n.Start
	state.CenterPitch = float64(n.Pitch)
}

func pianoSelectByTime(state *devices.Piano, direction int) {
	pat := state.Patterns[state.EditingPatternIdx]
	if len(pat.Notes) == 0 {
		return
	}
	state.SelectedNote += direction
	if state.SelectedNote < 0 {
		state.SelectedNote = len(pat.Notes) - 1
	} else if state.SelectedNote >= len(pat.Notes) {
		state.SelectedNote = 0
	}
	pianoCenterOnSelection(state)
}

func pianoSelectByPitch(state *devices.Piano, direction int) {
	pat := state.Patterns[state.EditingPatternIdx]
	if len(pat.Notes) == 0 {
		return
	}

	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		state.SelectedNote = 0
		pianoCenterOnSelection(state)
		return
	}

	current := pat.Notes[state.SelectedNote]
	targetPitch := int(current.Pitch) + direction

	bestIdx := -1
	bestDist := 1000.0
	for searchPitch := targetPitch; searchPitch >= 0 && searchPitch <= 127; searchPitch += direction {
		for i, n := range pat.Notes {
			if int(n.Pitch) != searchPitch {
				continue
			}
			dist := n.Start - current.Start
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			break
		}
		if direction == 0 {
			break
		}
	}

	if bestIdx >= 0 {
		state.SelectedNote = bestIdx
		pianoCenterOnSelection(state)
	}
}

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
