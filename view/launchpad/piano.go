package launchpad

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Piano launchpad layout (8x8 grid).
//
// The old sequencer used the full 8x8 for the roll with no on-pad nav
// (pan/zoom were keyboard-only). The new port keeps pan/zoom state
// (CenterBeat / CenterPitch / ViewScale) driving the display AND
// reserves the top row for nav/command pads so the launchpad alone is
// usable without a keyboard.
//
// Row 7 (top row) — nav/commands:
//
//	col 0:  pan left  (CenterBeat -= viewScale * 4)
//	col 1:  pan right (CenterBeat += viewScale * 4)
//	col 2:  pan down  (CenterPitch -= 4)
//	col 3:  pan up    (CenterPitch += 4)
//	col 4:  zoom out  (ViewScale++)
//	col 5:  zoom in   (ViewScale--)
//	col 6:  record arm toggle (lit bright red when armed)
//	col 7:  clear editing pattern
//
// Rows 0-6 — piano roll (7 pitches x 8 beats):
//
//	basePitch := int(CenterPitch) - 3
//	pitch     := basePitch + row             (row 0 = lowest shown)
//	startBeat := CenterBeat - 4 * viewScale
//	beat      := startBeat + col * viewScale
//
// Tapping a cell under an existing note selects it and re-centers the
// viewport on it. Tapping an empty cell adds a new note at that beat
// and pitch with duration = editH * 4 (matching TUI "space" and the
// old sequencer's HandlePad — clamped to what remains in the pattern).

// pianoRollRows is the number of piano-roll rows on the launchpad grid
// (rows 0..pianoRollRows-1). Row 7 is the nav/command bar.
const pianoRollRows = 7

// piano-view colors. Pure RGB (no pulse/blink — new LED type is static).
var (
	pianoCellEmpty    = LED{R: 8, G: 8, B: 20}      // very dim: empty cell
	pianoCellOff      = LED{R: 0, G: 0, B: 0}       // black: out of pattern bounds
	pianoCellNote     = LED{R: 80, G: 200, B: 255}  // cyan: note body
	pianoCellSelected = LED{R: 255, G: 100, B: 200} // magenta: selected note
	pianoCellPlayhead = LED{R: 255, G: 255, B: 255} // white: playhead column overlay

	pianoNavPan       = LED{R: 60, G: 40, B: 120}  // purple: pan pads
	pianoNavZoom      = LED{R: 40, G: 90, B: 160}  // blue: zoom pads
	pianoNavRecordOff = LED{R: 60, G: 0, B: 0}     // dim red: record-arm off
	pianoNavRecordOn  = LED{R: 255, G: 0, B: 0}    // bright red: record-arm on
	pianoNavClear     = LED{R: 200, G: 60, B: 40}  // red-orange: clear pattern
	pianoNavDark      = LED{R: 0, G: 0, B: 0}      // unused nav slot
)

// pianoPad handles a pad press on the 8x8 piano view. See the file-level
// comment for the layout.
//
// Caller (HandlePad in launchpad.go) already holds track.Lock and calls
// MarkTrackDirty after we return — so we just mutate spec here.
func pianoPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil || pat.Length <= 0 {
		return
	}

	// Top row — nav/commands.
	if row == 7 {
		pianoNav(state, pat, col)
		return
	}

	// Rows 0..6 — piano roll cell.
	viewScale := pianoViewScaleLP(state.ViewScale)
	basePitch := int(state.CenterPitch) - 3
	pitch := basePitch + row
	if pitch < 0 || pitch > 127 {
		return
	}
	startBeat := state.CenterBeat - 4*viewScale
	beat := startBeat + float64(col)*viewScale
	beatEnd := beat + viewScale
	if beat < 0 || beat >= pat.Length {
		return
	}

	// Hit-test existing notes at (pitch, beat..beatEnd).
	for i := range pat.Notes {
		n := &pat.Notes[i]
		if int(n.Pitch) != pitch {
			continue
		}
		noteEnd := n.Start + n.Duration
		if n.Start < beatEnd && noteEnd > beat {
			state.SelectedNote = i
			state.CenterBeat = n.Start
			state.CenterPitch = float64(n.Pitch)
			return
		}
	}

	// Empty cell — add a new note. Duration = editH * 4 (matches "space"
	// in the TUI and old sequencer HandlePad's "add at tap"). Clamp to
	// whatever space remains in the pattern; floor to the grid cell width.
	editH := pianoEditHorizLP(state.EditHoriz)
	dur := editH * 4
	if dur < viewScale {
		dur = viewScale
	}
	if beat+dur > pat.Length {
		dur = pat.Length - beat
	}
	if dur <= 0 {
		return
	}
	pat.Notes = append(pat.Notes, devices.NoteEvent{
		Start:    beat,
		Duration: dur,
		Pitch:    uint8(pitch),
		Velocity: 100,
	})
	// Sort by start time and reselect on match.
	newStart, newPitch := beat, uint8(pitch)
	sort.SliceStable(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})
	state.SelectedNote = -1
	for i, n := range pat.Notes {
		if n.Start == newStart && n.Pitch == newPitch {
			state.SelectedNote = i
			break
		}
	}
	state.CenterBeat = newStart
	state.CenterPitch = float64(newPitch)
}

// pianoNav handles a tap on the row-7 nav/command bar.
func pianoNav(state *devices.Piano, pat *devices.PianoPattern, col int) {
	viewScale := pianoViewScaleLP(state.ViewScale)
	switch col {
	case 0: // pan left
		state.CenterBeat -= viewScale * 4
		if state.CenterBeat < 0 {
			state.CenterBeat = 0
		}
	case 1: // pan right
		state.CenterBeat += viewScale * 4
		if state.CenterBeat > pat.Length {
			state.CenterBeat = pat.Length
		}
	case 2: // pan down (lower pitches)
		state.CenterPitch -= 4
		if state.CenterPitch < 0 {
			state.CenterPitch = 0
		}
	case 3: // pan up (higher pitches)
		state.CenterPitch += 4
		if state.CenterPitch > 127 {
			state.CenterPitch = 127
		}
	case 4: // zoom out (larger viewScale)
		if state.ViewScale < len(pianoViewScalesLP)-1 {
			state.ViewScale++
		}
	case 5: // zoom in (smaller viewScale)
		if state.ViewScale > 0 {
			state.ViewScale--
		}
	case 6: // record-arm toggle
		state.Recording = !state.Recording
	case 7: // clear editing pattern
		pat.Notes = pat.Notes[:0]
		state.SelectedNote = -1
	}
}

// pianoLEDs renders the 8x8 piano view. Row 7 is the nav/command bar;
// rows 0..6 are the piano roll. See the file-level comment for layout.
func pianoLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil || track.Piano == nil {
		return nil
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	viewScale := pianoViewScaleLP(state.ViewScale)
	basePitch := int(state.CenterPitch) - 3
	startBeat := state.CenterBeat - 4*viewScale

	playheadCol := pianoPlayingCol(track, project, startBeat, viewScale)

	leds := make([]LED, 0, 64)

	// Piano roll on rows 0..pianoRollRows-1.
	for row := 0; row < pianoRollRows; row++ {
		pitch := basePitch + row
		for col := 0; col < 8; col++ {
			var c LED
			if pitch < 0 || pitch > 127 {
				c = pianoCellOff
				leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
				continue
			}
			colBeat := startBeat + float64(col)*viewScale
			colBeatEnd := colBeat + viewScale

			if colBeat < 0 || colBeat >= pat.Length {
				c = pianoCellOff
				leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
				continue
			}

			c = pianoCellEmpty
			for i := range pat.Notes {
				n := &pat.Notes[i]
				if int(n.Pitch) != pitch {
					continue
				}
				noteEnd := n.Start + n.Duration
				if n.Start < colBeatEnd && noteEnd > colBeat {
					if i == state.SelectedNote {
						c = pianoCellSelected
					} else {
						c = pianoCellNote
					}
					break
				}
			}
			if col == playheadCol {
				c = pianoCellPlayhead
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	// Top row — nav/command pads (row 7).
	navCols := [8]LED{
		pianoNavPan,  // 0 pan left
		pianoNavPan,  // 1 pan right
		pianoNavPan,  // 2 pan down
		pianoNavPan,  // 3 pan up
		pianoNavZoom, // 4 zoom out
		pianoNavZoom, // 5 zoom in
		pianoNavRecordOff,
		pianoNavClear,
	}
	if state.Recording {
		navCols[6] = pianoNavRecordOn
	}
	_ = pianoNavDark // keep reference so future slots can go dark
	for col, c := range navCols {
		leds = append(leds, LED{Row: 7, Col: col, R: c.R, G: c.G, B: c.B})
	}

	return leds
}

// pianoPlayingCol returns the grid column (0..7) currently being played on
// this track, or -1 when unavailable (not playing, uncompiled pattern,
// the edit view is on a different pattern than the one playing, or the
// playhead falls outside the visible viewport).
func pianoPlayingCol(track *model.Track, project *model.Project, startBeat, viewScale float64) int {
	if track.Piano == nil {
		return -1
	}
	playingIdx := track.Piano.Schedule.Playing
	if playingIdx < 0 || playingIdx >= len(track.Piano.Patterns) {
		return -1
	}
	playingPat := track.Piano.Patterns[playingIdx]
	if playingPat == nil {
		return -1
	}
	if track.Piano.EditingPatternIdx != playingIdx {
		return -1
	}
	trackIdx := -1
	for i := 0; i < 8; i++ {
		if project.Tracks[i] == track {
			trackIdx = i
			break
		}
	}
	if trackIdx < 0 {
		return -1
	}
	cursor := project.Playback.Cursors[trackIdx]
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
	beat := float64(posInPattern) / float64(devices.PPQ)

	if viewScale <= 0 {
		return -1
	}
	col := int((beat - startBeat) / viewScale)
	if col < 0 || col >= 8 {
		return -1
	}
	return col
}

// --- piano-view tables (zoom / edit sensitivity) ---
//
// These mirror the TUI-side tables (pianoViewScales / pianoEditHorizSteps
// in view/tui/piano.go). Duplicated per DESIGN "views are per-package";
// the SHORTCUTS.md entry already flags this for a future lift into a
// shared leaf package.

var pianoViewScalesLP = []float64{
	0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0, 2.0, 4.0,
}

var pianoEditHorizStepsLP = []float64{
	0.015625, 0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0,
}

func pianoViewScaleLP(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoViewScalesLP) {
		idx = len(pianoViewScalesLP) - 1
	}
	return pianoViewScalesLP[idx]
}

func pianoEditHorizLP(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditHorizStepsLP) {
		idx = len(pianoEditHorizStepsLP) - 1
	}
	return pianoEditHorizStepsLP[idx]
}
