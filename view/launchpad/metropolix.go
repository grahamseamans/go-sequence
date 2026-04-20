package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// Metropolix launchpad colors. First-cut UI: one page only (stage editor),
// 8x8 grid, rows = scale-degree 0..7, cols = stage 0..7.
var (
	mxStageActive    = LED{R: 255, G: 100, B: 50}  // lit cell: stage's current note (Gate on)
	mxStageActiveOff = LED{R: 60, G: 25, B: 15}    // dim cell: stage's current note (Gate off)
	mxStageDim       = LED{R: 20, G: 10, B: 5}     // dim: other rows within an active stage column
	mxStageInactive  = LED{R: 0, G: 0, B: 0}       // dark: col >= pat.Length
	mxSelectedCol    = LED{R: 255, G: 255, B: 255} // white: selected-stage marker (top row)
)

// metropolixPad handles a pad press on the Metropolix view. First-cut UI:
// the only page is the stage-note grid.
//
//	cols 0..7  = stage 0..7 (ignored when col >= pat.Length)
//	rows 0..7  = scale-degree 0..7 within the pattern's Scale
//	             (note: this is the stage's Note field, which is clamped 0..7
//	             by Metropolix.Validate; it indexes into the scale lookup at
//	             compile time)
//
// Tap behavior: sets stage[col].Note = row AND stage[col].Gate = true
// AND spec.Selected = col. Tapping the same row again re-arms gate but
// doesn't toggle it — gate clearing is a TUI action ('g' key).
//
// Side column (col 8) and top row (row 8) are currently unused on this
// view; no paging yet.
//
// Caller (HandlePad in launchpad.go) holds track.Lock and calls
// MarkTrackDirty after this returns; we just mutate spec.
func metropolixPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
		return
	}
	// Main grid only. Ignore the side column and top row for now.
	if row < 0 || row > 7 || col < 0 || col > 7 {
		return
	}
	if col >= pat.Length {
		return
	}
	stage := &pat.Stages[col]
	stage.Note = row
	stage.Gate = true
	spec.Selected = col
}

// metropolixLEDs renders the 8x8 LED frame for the Metropolix stage editor.
//
// Per column (stage):
//
//	col >= pat.Length      — all rows off (stage is out of play range).
//	Gate == true           — the stage's Note row lights bright; other rows
//	                         in the column are dim.
//	Gate == false          — the stage's Note row shows a dim/muted tint;
//	                         other rows off. (Still visible at a glance, but
//	                         clearly distinct from gated stages.)
//
// The currently-selected stage column is marked with a white dot on row 7
// above the column. No playhead indicator in this first cut — mapping
// pattern-tick to stage is more involved than drum/piano.
func metropolixLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil || track.Metropolix == nil {
		return nil
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	leds := make([]LED, 0, 64)

	for col := 0; col < 8; col++ {
		if col >= pat.Length {
			for row := 0; row < 8; row++ {
				leds = append(leds, LED{Row: row, Col: col,
					R: mxStageInactive.R, G: mxStageInactive.G, B: mxStageInactive.B})
			}
			continue
		}
		stage := &pat.Stages[col]
		noteRow := stage.Note
		if noteRow < 0 {
			noteRow = 0
		}
		if noteRow > 7 {
			noteRow = 7
		}
		for row := 0; row < 8; row++ {
			var c LED
			switch {
			case row == noteRow && stage.Gate:
				c = mxStageActive
			case row == noteRow && !stage.Gate:
				c = mxStageActiveOff
			case stage.Gate:
				c = mxStageDim
			default:
				c = mxStageInactive
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	// Selected-stage marker: overwrite row 7 of the selected column with white.
	// This intentionally stomps the Note indicator on that column — acceptable
	// because the user moves selection often and wants an unambiguous cue.
	if spec.Selected >= 0 && spec.Selected < pat.Length {
		leds = append(leds, LED{Row: 7, Col: spec.Selected,
			R: mxSelectedCol.R, G: mxSelectedCol.G, B: mxSelectedCol.B})
	}

	return leds
}
