package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// session-view colors. Bright green = currently-playing pattern; amber =
// queued pattern; dim white = pattern that has content but isn't
// playing / queued; off = empty pattern slot or no-device track.
var (
	sessionPlaying    = LED{R: 0, G: 200, B: 0}
	sessionQueued     = LED{R: 255, G: 160, B: 0}
	sessionHasContent = LED{R: 30, G: 30, B: 30}
	sessionEmpty      = LED{R: 0, G: 0, B: 0}
)

// sessionPad queues a pattern on a track via the 8x8 session grid.
//
// Layout:
//
//	row in [0,7] is the track index (row 0 = track 0, top of the grid).
//	col in [0,7] is the pattern index on that track.
//
// Writes QueuedPattern on the track's cursor; the playback engine
// promotes it to CurrentPattern at the next pattern boundary.
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
	if track == nil || track.Type == model.DeviceNone {
		return
	}
	project.Playback.Cursors[row].QueuedPattern = col
}

// sessionLEDs renders the 8x8 LED frame for the session view.
//
// Each row is a track; each column is a pattern slot within that track's
// 16 patterns (only the first 8 are shown on the pad grid — patterns 8..15
// aren't reachable from the pads).
//
// Per-cell color priority: playing > queued > has-content > empty.
func sessionLEDs(project *model.Project) []LED {
	if project == nil {
		return nil
	}
	leds := make([]LED, 0, 64)
	for row := 0; row < 8; row++ {
		track := project.Tracks[row]
		if track == nil || track.Type == model.DeviceNone {
			// Empty row: leave pads dark. Emitting explicit zero LEDs is
			// load-bearing because SetLEDs rewrites the full frame each
			// tick and anything not emitted keeps its old color.
			for col := 0; col < 8; col++ {
				leds = append(leds, LED{Row: row, Col: col, R: sessionEmpty.R, G: sessionEmpty.G, B: sessionEmpty.B})
			}
			continue
		}
		cursor := project.Playback.Cursors[row]
		for col := 0; col < 8; col++ {
			c := sessionCellColor(track, cursor, col)
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}
	return leds
}

// sessionCellColor picks the LED color for one cell of the session grid.
// Precedence: playing > queued > has-content > empty. Tracks without a
// device never reach this — caller filters them.
func sessionCellColor(track *model.Track, cursor model.Cursor, col int) LED {
	if cursor.CurrentPattern == col {
		return sessionPlaying
	}
	if cursor.QueuedPattern == col {
		return sessionQueued
	}
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil && sessionDrumPatternHasContent(track.Drum.Patterns[col]) {
			return sessionHasContent
		}
	case model.DevicePiano:
		if track.Piano != nil && sessionPianoPatternHasContent(track.Piano.Patterns[col]) {
			return sessionHasContent
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil && sessionMetropolixPatternHasContent(track.Metropolix.Patterns[col]) {
			return sessionHasContent
		}
	}
	return sessionEmpty
}

// sessionDrumPatternHasContent reports whether any step of any lane in the
// pattern is Active. A nil pattern counts as empty.
func sessionDrumPatternHasContent(pat *devices.DrumPattern) bool {
	if pat == nil {
		return false
	}
	for lane := range pat.Notes {
		for step := range pat.Notes[lane].Steps {
			if pat.Notes[lane].Steps[step].Active {
				return true
			}
		}
	}
	return false
}

// sessionPianoPatternHasContent reports whether the piano pattern has any
// notes. A nil pattern counts as empty.
func sessionPianoPatternHasContent(pat *devices.PianoPattern) bool {
	if pat == nil {
		return false
	}
	return len(pat.Notes) > 0
}

// sessionMetropolixPatternHasContent reports whether any stage in the
// pattern has Gate=true. A nil pattern counts as empty.
func sessionMetropolixPatternHasContent(pat *devices.MetropolixPattern) bool {
	if pat == nil {
		return false
	}
	for i := range pat.Stages {
		if pat.Stages[i].Gate {
			return true
		}
	}
	return false
}
