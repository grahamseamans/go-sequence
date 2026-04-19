package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// sessionPad queues a pattern on a track via the 8x8 session grid.
//
// row in [0,7] is the track index (row 0 = track 0, top of the grid).
// col in [0,7] is the pattern index for that track.
//
// Ported from controller/devices/session.HandlePad.
func sessionPad(project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if row < 0 || row >= 8 || col < 0 || col >= 8 {
		return
	}
	if project == nil {
		return
	}
	track := project.Tracks[row]
	if track == nil {
		return
	}

	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil {
			track.Drum.Schedule.Queued = col
		}
	case model.DevicePiano:
		if track.Piano != nil {
			track.Piano.Schedule.Queued = col
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil {
			track.Metropolix.Schedule.Queued = col
		}
	case model.DeviceNone:
		// No device on this track — nothing to queue.
	}
}

// sessionLEDs returns the LED frame for the session view. Stub.
func sessionLEDs(project *model.Project) []LED { return nil }
