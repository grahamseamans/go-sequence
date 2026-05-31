package keyboard

import (
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// looperMIDI handles an incoming MIDI event on the looper view.
//
// Two modes:
//
//  1. Not recording: preview. Send the event straight through to the track's
//     output so the user hears what they play.
//  2. Recording: capture every event with a tick offset relative to the
//     current pattern position. Both NoteOn and NoteOff are captured so
//     playback reproduces the exact performance.
//
// Caller (HandleMIDI in keyboard.go) holds track.Lock and calls
// MarkTrackDirty after we return, so we only mutate spec here.
func looperMIDI(track *model.Track, project *model.Project, trackIdx int, out midi.ToExternal, ev midi.Event) {
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Not recording — preview straight through.
	if !d.Recording {
		if err := out.Send(track.PortName, track.Channel, ev); err != nil {
			debug.Log("keyboard", "looper preview send to %q ch%d: %v", track.PortName, track.Channel, err)
		}
		return
	}

	// Recording — stamp with pattern-relative tick.
	cursor := project.Playback.Cursors[trackIdx]
	globalTick := project.Playback.Tick.Load()
	pos := globalTick - cursor.T0Tick
	if pos < 0 {
		pos = 0
	}
	tick := pos % pat.Length

	pat.Events = append(pat.Events, devices.TimedEvent{
		Tick:  tick,
		Event: ev,
	})
}
