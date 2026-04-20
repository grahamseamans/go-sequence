package keyboard

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drumMIDI handles an incoming MIDI note on the drum view.
//
// Two modes:
//
//  1. Not recording: preview. We send the note straight to the track's
//     output. No queueing, no scheduling — the user gets audible
//     feedback of their keyboard strike.
//  2. Recording: write. We reverse-lookup the MIDI note to a drum lane
//     (via the track's kit), then write into the editing pattern at the
//     cursor position with the incoming velocity.
//
// The caller (HandleMIDI in keyboard.go) holds track.Lock and will call
// MarkTrackDirty after we return, so we only mutate spec here — no
// explicit MarkPatternDirty.
//
// project and trackIdx are accepted for signature symmetry with other view
// packages, even though we don't currently use them here; leaving them on
// the signature keeps the call site stable if we later want to, e.g.,
// stamp the recorded step from live playback tick.
func drumMIDI(track *model.Track, project *model.Project, trackIdx int, out midi.ToExternal, ev midi.Event) {
	if track == nil || track.Drum == nil {
		return
	}
	// Only react to note-on with positive velocity. A note-on with
	// velocity 0 is a note-off encoded in running status; ignore both.
	if ev.Type != midi.NoteOn || ev.Velocity == 0 {
		return
	}

	state := track.Drum

	// Not recording: preview this note straight to the track's output.
	// Send the hit MIDI note as-is (no kit translation) — the user is
	// playing the physical drum keys on their controller and expects to
	// hear exactly what they pressed.
	if !state.Recording {
		_ = out.Send(track.PortName, track.Channel, midi.Event{
			Type:     midi.NoteOn,
			Note:     ev.Note,
			Velocity: ev.Velocity,
		})
		return
	}

	// Recording: map the MIDI note to a lane via the kit's reverse lookup.
	// If this note isn't in the kit, there's no lane to write to — drop
	// the hit (the user hit a pad not part of the current kit mapping).
	kit := devices.GetKit(track.Kit)
	laneIdx := -1
	for i, n := range kit.Notes {
		if n == ev.Note {
			laneIdx = i
			break
		}
	}
	if laneIdx < 0 {
		return
	}

	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return
	}
	step := state.Cursor
	if step < 0 || step >= 32 {
		step = 0
	}
	pat.Notes[laneIdx].Steps[step].Active = true
	pat.Notes[laneIdx].Steps[step].Velocity = ev.Velocity
}
