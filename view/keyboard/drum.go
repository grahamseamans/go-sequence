package keyboard

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drumMIDI records incoming MIDI hits into the editing pattern when
// recording is armed.
//
// Signature ports from controller/devices/drum.HandleMIDI. Caller holds
// track.mu.
func drumMIDI(track *model.Track, out midi.ToExternal, ev midi.Event) {
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	if !state.Recording {
		return
	}
	if ev.Type != midi.NoteOn || ev.Velocity == 0 {
		return
	}

	// Reverse-lookup MIDI note → lane index via the kit.
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

	// We don't have the live playback tick here (not plumbed yet). Write at
	// the cursor and advance — matches the pad-recording shortcut in
	// view/launchpad/drum.go.
	pat := state.Patterns[state.EditingPatternIdx]
	s := &pat.Notes[laneIdx].Steps[state.Cursor]
	s.Active = true
	s.Velocity = ev.Velocity
}
