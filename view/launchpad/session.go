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
	// sessionSceneDim lights the right-column "play all" scene buttons so they
	// read as "available" — a dim white, reusing the has-content brightness.
	sessionSceneDim = LED{R: 30, G: 30, B: 30}
)

// Session grid orientation — same as the TUI and the physical hardware:
//   - COLUMNS are instruments/tracks: col 0 = instrument 1 (left) ... col 7 =
//     instrument 8 (right).
//   - ROWS are patterns: pattern 1 on the TOP row ... pattern 8 on the bottom.
//
// hw.go puts physical row 0 at the BOTTOM, so a pattern p lives on physical
// row (7 - p), and vice versa. The right-hand round buttons (col 8) are the
// "play all" / scene launchers — one per pattern row.

// sessionRowPattern converts between a physical grid row and a pattern index.
// Self-inverse: pattern 0 (top) <-> physical row 7; pattern 7 <-> row 0.
func sessionRowPattern(r int) int { return 7 - r }

// sessionPad queues (or un-queues) the cell under a grid press: the column is
// the instrument/track, the row is the pattern (pattern 1 on top). The
// toggle + per-track lock + dirty-mark live in controller.ToggleQueue, shared
// with the TUI session view.
func sessionPad(project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	controller.ToggleQueue(project, col, sessionRowPattern(row))
}

// sessionLEDs renders the session frame: the 8x8 grid (col = instrument, row =
// pattern, pattern 1 on top) plus the 8 right-column "play all" scene buttons.
// Only patterns 0..7 are reachable from the grid (patterns 8..15 aren't).
//
// Per-cell color priority: playing > queued > has-content > empty.
func sessionLEDs(project *model.Project) []LED {
	if project == nil {
		return nil
	}
	leds := make([]LED, 0, 72)
	for col := 0; col < 8; col++ {
		track := project.Tracks[col]
		hasDevice := track != nil && track.Type != model.DeviceNone
		var cursor model.Cursor
		if hasDevice {
			cursor = project.Playback.Cursors[col]
		}
		for pattern := 0; pattern < 8; pattern++ {
			row := sessionRowPattern(pattern)
			c := sessionEmpty
			if hasDevice {
				c = sessionCellColor(track, cursor, pattern)
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}
	// Right-column "play all" scene buttons (Col 8, rows 0-7), one per pattern
	// row. Emitted every frame — SetLEDs rewrites the whole frame, so anything
	// not emitted keeps its stale color. Dim white = "available".
	for row := 0; row < 8; row++ {
		leds = append(leds, LED{Row: row, Col: 8, R: sessionSceneDim.R, G: sessionSceneDim.G, B: sessionSceneDim.B})
	}
	return leds
}

// sessionCellColor picks the LED color for one (track, pattern) cell.
// Precedence: playing > queued > has-content > empty. Tracks without a
// device never reach this — caller filters them.
func sessionCellColor(track *model.Track, cursor model.Cursor, pattern int) LED {
	if cursor.CurrentPattern == pattern {
		return sessionPlaying
	}
	if cursor.QueuedPattern == pattern {
		return sessionQueued
	}
	if model.PatternHasContent(track, pattern) {
		return sessionHasContent
	}
	return sessionEmpty
}
