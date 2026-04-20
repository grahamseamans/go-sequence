package keyboard

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// pianoMinNoteDurationBeats is the floor for any recorded / previewed note.
const pianoMinNoteDurationBeats = 0.25

// pianoMIDI handles an incoming MIDI event on the piano view.
//
// Two modes:
//
//  1. Not recording: preview. Send the event straight through to the track's
//     output so the user hears what they played.
//  2. Recording: append a NoteEvent at the current playback beat with a
//     fixed 1-beat duration (first-pass simplification — we don't thread
//     note-off back to the note's Duration yet).
//
// Caller (HandleMIDI in keyboard.go) holds track.Lock and calls
// MarkTrackDirty after we return, so we only mutate spec here.
//
// project + trackIdx are used to read the live playback tick so recorded
// notes land where the playhead actually is.
func pianoMIDI(track *model.Track, project *model.Project, trackIdx int, out midi.ToExternal, ev midi.Event) {
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano

	// Not recording: preview this event straight through. Forward NoteOn and
	// NoteOff both so the user hears the full gesture.
	if !state.Recording {
		if ev.Type == midi.NoteOn || ev.Type == midi.NoteOff {
			_ = out.Send(track.PortName, track.Channel, ev)
		}
		return
	}

	// Recording: only react to note-on with positive velocity. A note-on
	// with velocity 0 is a note-off encoded in running status; both get
	// dropped in this first pass (see TODO below on duration).
	if ev.Type != midi.NoteOn || ev.Velocity == 0 {
		return
	}

	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil || pat.Length <= 0 {
		return
	}

	// Write-head = current playback beat within the editing pattern. If the
	// pattern isn't compiled yet, or we can't find the cursor, fall back to
	// CenterBeat — deterministic even when the transport is idle.
	start := pianoPlaybackBeat(track, project, trackIdx)
	if start < 0 {
		start = state.CenterBeat
	}
	if start < 0 {
		start = 0
	}

	// TODO: duration-from-note-off. For now we use a fixed 1 beat and clamp
	// to what fits in the pattern; lower bound is pianoMinNoteDurationBeats.
	dur := 1.0
	if start+dur > pat.Length {
		dur = pat.Length - start
	}
	if dur < pianoMinNoteDurationBeats {
		dur = pianoMinNoteDurationBeats
	}
	if start+dur > pat.Length {
		// Pattern is shorter than the minimum note — bail rather than
		// writing a zero/negative-duration event.
		return
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

// pianoPlaybackBeat returns the current playback position (in beats) within
// the editing pattern for this track, or -1 if playback isn't active on a
// compiled pattern we can lock onto.
func pianoPlaybackBeat(track *model.Track, project *model.Project, trackIdx int) float64 {
	if track.Piano == nil {
		return -1
	}
	if trackIdx < 0 || trackIdx >= 8 {
		return -1
	}
	playingIdx := track.Piano.Schedule.Playing
	if playingIdx < 0 || playingIdx >= len(track.Piano.Patterns) {
		return -1
	}
	playingPat := track.Piano.Patterns[playingIdx]
	if playingPat == nil {
		return -1
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
	return float64(posInPattern) / float64(devices.PPQ)
}
