package launchpad

import (
	"sort"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// piano-view colors. Pure RGB (no pulse/blink — new LED type is static).
var (
	pianoCellEmpty    = LED{R: 8, G: 8, B: 20}      // very dim: empty cell
	pianoCellOff      = LED{R: 0, G: 0, B: 0}       // black: out of pattern bounds
	pianoCellNote     = LED{R: 80, G: 200, B: 255}  // cyan: note body
	pianoCellSelected = LED{R: 255, G: 100, B: 200} // magenta: selected note
	pianoCellPlayhead = LED{R: 255, G: 255, B: 255} // white: playhead column overlay
)

// pianoPad handles a pad press on the 8x8 piano view.
//
// Layout: 8 rows x 8 cols.
//
//	rows:  pitches. row 0 is the highest pitch, row 7 the lowest.
//	       pitch(row) = centerPitch + 4 - row, so the selected/centered
//	       pitch sits in the middle of the visible range.
//	cols:  beats. The whole editing pattern is split into 8 equal slices,
//	       so col width = pat.Length / 8 beats.
//
// Tapping a cell under an existing note selects that note (and re-centers
// the viewport on it). Tapping an empty cell adds a new 1-beat note at
// velocity 100 starting at the cell's beat. If 1 beat doesn't fit, the
// note is clamped down to the remaining space.
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

	// pitch: row 0 = centerPitch+4, row 7 = centerPitch-3.
	centerPitch := int(state.CenterPitch)
	pitch := centerPitch + 4 - row
	if pitch < 0 || pitch > 127 {
		return
	}

	// beat: split pat.Length into 8 equal slices.
	colWidth := pat.Length / 8
	beat := float64(col) * colWidth
	beatEnd := beat + colWidth

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

	// Empty cell: add a new note. Target duration is 1 beat, clamped to
	// whatever space remains in the pattern.
	dur := 1.0
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
	// Keep notes sorted by start time and restore the selection on the
	// note we just created (match on start+pitch).
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

// pianoLEDs renders the 8x8 piano view. Rows are pitches, cols are beats.
// Note cells light cyan (selected = magenta); the playhead column draws
// white on top; out-of-pattern cells are black; empty cells are very dim.
func pianoLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil || track.Piano == nil {
		return nil
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	playheadCol := pianoPlayingCol(track, project)

	leds := make([]LED, 0, 64)
	centerPitch := int(state.CenterPitch)
	colWidth := 0.0
	if pat.Length > 0 {
		colWidth = pat.Length / 8
	}

	for row := 0; row < 8; row++ {
		pitch := centerPitch + 4 - row
		for col := 0; col < 8; col++ {
			var c LED
			if pitch < 0 || pitch > 127 || colWidth <= 0 {
				c = pianoCellOff
				leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
				continue
			}
			colBeat := float64(col) * colWidth
			colBeatEnd := colBeat + colWidth

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
	return leds
}

// pianoPlayingCol returns the column (0..7) currently being played on
// this track, or -1 when unavailable (not playing, uncompiled pattern,
// or the edit view is on a different pattern than the one playing).
func pianoPlayingCol(track *model.Track, project *model.Project) int {
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
	posInBeats := float64(posInPattern) / float64(devices.PPQ)

	pat := track.Piano.Patterns[track.Piano.EditingPatternIdx]
	if pat == nil || pat.Length <= 0 {
		return -1
	}
	colWidth := pat.Length / 8
	if colWidth <= 0 {
		return -1
	}
	col := int(posInBeats / colWidth)
	if col < 0 || col >= 8 {
		return -1
	}
	return col
}
