package looper

import (
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// RecordOrPreview handles one incoming MIDI event on a looper track. It is
// shared by every input source that feeds a looper: the launchpad grid
// keyboard and the external-MIDI-keyboard view.
//
// Two modes:
//
//  1. Not recording and not overdubbing: preview. Send the event straight
//     through to the track's output so the user hears what they play.
//  2. Recording OR overdubbing: capture the event with a tick offset relative
//     to the current pattern position. Both NoteOn and NoteOff are captured so
//     playback reproduces the exact performance.
//
// The Record-vs-Overdub clear-on-arm difference is handled by the pad handler
// (Record arms with a wiped loop, Overdub arms keeping events), so this one
// helper serves both record and overdub.
//
// Callers hold track.Lock and mark the track dirty after we return, so we only
// mutate spec here.
func RecordOrPreview(track *model.Track, project *model.Project, trackIdx int, out midi.ToExternal, ev midi.Event) {
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Not capturing — preview straight through.
	if !(d.Recording || d.Overdub) {
		if err := out.Send(track.PortName, track.Channel, ev); err != nil {
			debug.Log("looper", "preview send to %q ch%d: %v", track.PortName, track.Channel, err)
		}
		return
	}

	// Recording or overdubbing — stamp with pattern-relative tick.
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
