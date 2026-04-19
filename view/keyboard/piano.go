package keyboard

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// pianoEditHorizSteps is the beat delta for each horizontal-edit sensitivity.
var pianoEditHorizSteps = []float64{
	0.015625, 0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0,
}

const pianoMinNoteDurationBeats = 0.25

// pianoMIDI records incoming MIDI notes into the editing pattern when
// recording is armed.
//
// Ported from controller/devices/piano.HandleMIDI. The recorded Start is the
// current CenterBeat, the Duration is the current edit-horizontal size —
// crude but deterministic until the live playback tick is plumbed through
// the input path.
func pianoMIDI(track *model.Track, out midi.ToExternal, ev midi.Event) {
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	if !state.Recording {
		return
	}
	if ev.Type != midi.NoteOn || ev.Velocity == 0 {
		return
	}

	pat := state.Patterns[state.EditingPatternIdx]
	dur := pianoEditHoriz(state.EditHoriz) * 4
	if dur < pianoMinNoteDurationBeats {
		dur = pianoMinNoteDurationBeats
	}

	start := state.CenterBeat
	if start < 0 {
		start = 0
	}
	if start+dur > pat.Length {
		if pat.Length >= dur {
			start = pat.Length - dur
		} else {
			start = 0
			dur = pat.Length
		}
	}

	pat.Notes = append(pat.Notes, devices.NoteEvent{
		Start:    start,
		Duration: dur,
		Pitch:    ev.Note,
		Velocity: ev.Velocity,
	})
	sort.SliceStable(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})
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
