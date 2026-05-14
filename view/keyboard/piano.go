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
//     output so the user hears what they played. Both NoteOn and NoteOff
//     are forwarded so the note has a matching release.
//  2. Recording: capture NoteOn/NoteOff pairs. On NoteOn, we stash the
//     start beat and attack velocity in track.Piano.Pending (keyed by
//     pitch). On NoteOff (or NoteOn vel=0) we close the pending note —
//     Duration = currentBeat - startBeat, floored to
//     pianoMinNoteDurationBeats — and append to pat.Notes.
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

	// Not recording — preview this event straight through. Forward NoteOn
	// and NoteOff both so the user hears the full gesture (press+release).
	if !state.Recording {
		if ev.Type == midi.NoteOn || ev.Type == midi.NoteOff {
			_ = out.Send(track.PortName, track.Channel, ev)
		}
		return
	}

	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil || pat.Length <= 0 {
		return
	}

	// Recording — treat NoteOn vel=0 as NoteOff (running-status encoding).
	isNoteOn := ev.Type == midi.NoteOn && ev.Velocity > 0
	isNoteOff := ev.Type == midi.NoteOff || (ev.Type == midi.NoteOn && ev.Velocity == 0)

	if !isNoteOn && !isNoteOff {
		return
	}

	// Writer head for both NoteOn (StartBeat) and NoteOff (EndBeat): the
	// current playback position. If the pattern isn't playing, fall back
	// to CenterBeat so the gesture still lands somewhere deterministic.
	beat := pianoPlaybackBeat(track, project, trackIdx)
	if beat < 0 {
		beat = state.CenterBeat
	}
	if beat < 0 {
		beat = 0
	}

	if state.Pending == nil {
		state.Pending = make(map[uint8]devices.PianoPending)
	}

	switch {
	case isNoteOn:
		// Stash start + attack velocity. If a NoteOn arrives while a note
		// of the same pitch is still pending (e.g. retrigger without
		// NoteOff), drop the stale one on the floor by overwriting — the
		// alternative is a dangling pending forever.
		state.Pending[ev.Note] = devices.PianoPending{
			StartBeat: beat,
			Velocity:  ev.Velocity,
		}

	case isNoteOff:
		pending, ok := state.Pending[ev.Note]
		if !ok {
			// NoteOff without a matching NoteOn (arrived during recording
			// start, from a pre-existing held key, etc.). Nothing to close.
			return
		}
		delete(state.Pending, ev.Note)

		duration := beat - pending.StartBeat
		if duration < pianoMinNoteDurationBeats {
			duration = pianoMinNoteDurationBeats
		}

		start := pending.StartBeat
		if start < 0 {
			start = 0
		}
		if start >= pat.Length {
			// Pending started past the end of the pattern — can happen if
			// playback looped around past NoteOn. Drop.
			return
		}
		if start+duration > pat.Length {
			duration = pat.Length - start
		}
		if duration < pianoMinNoteDurationBeats {
			// Pattern is shorter than the minimum note — bail rather than
			// writing a zero/negative-duration event.
			return
		}

		pat.Notes = append(pat.Notes, devices.NoteEvent{
			Start:    start,
			Duration: duration,
			Pitch:    ev.Note,
			Velocity: pending.Velocity,
		})
		sort.SliceStable(pat.Notes, func(i, j int) bool {
			return pat.Notes[i].Start < pat.Notes[j].Start
		})
	}
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
	cursor := project.Playback.Cursors[trackIdx]
	playingIdx := cursor.CurrentPattern
	if playingIdx < 0 || playingIdx >= len(track.Piano.Patterns) {
		return -1
	}
	playingPat := track.Piano.Patterns[playingIdx]
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
	return float64(posInPattern) / float64(devices.PPQ)
}
