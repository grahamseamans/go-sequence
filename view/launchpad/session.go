package launchpad

import (
	"go-sequence/controller"
	"go-sequence/midi"
	"go-sequence/model"
)

// session-view colors. Bright green = currently-playing pattern; amber =
// queued pattern; dim white = pattern that has content but isn't
// playing / queued; off = empty pattern slot or no-device track.
var (
	sessionPlaying    = LED{R: 0, G: 200, B: 0}
	sessionQueued     = LED{R: 255, G: 160, B: 0}
	sessionHasContent = LED{R: 30, G: 30, B: 30}
	sessionEmpty      = LED{R: 0, G: 0, B: 0}
	// sessionSceneDim lights the top-row scene-trigger buttons so they read
	// as "available" — a dim white, reusing the has-content brightness.
	sessionSceneDim = LED{R: 30, G: 30, B: 30}
)

// sessionPad queues (or un-queues) a pattern on a track via the 8x8 session
// grid. row in [0,7] is the track; col in [0,7] is the pattern slot. The
// queue/un-queue toggle + locking + dirty-marking live in
// controller.ToggleQueue, shared with the TUI session view.
func sessionPad(project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	controller.ToggleQueue(project, row, col)
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
	// Top-row scene-trigger buttons (Row 8, cols 0-7). Emitted every frame —
	// SetLEDs rewrites the whole frame, so anything not emitted keeps its
	// stale color. Dim white = "scene available."
	for col := 0; col < 8; col++ {
		leds = append(leds, LED{Row: 8, Col: col, R: sessionSceneDim.R, G: sessionSceneDim.G, B: sessionSceneDim.B})
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
	if model.PatternHasContent(track, col) {
		return sessionHasContent
	}
	return sessionEmpty
}
