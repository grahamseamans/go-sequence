package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
	"go-sequence/widgets"
)

// piano view constants.
const (
	pianoMaxCols            = 64 // hard cap on grid columns drawn
	pianoMinNoteDurationBts = 0.25
)

var pianoNoteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// Piano TUI styling. Flat palette — no runtime theme wiring yet; the
// per-role colors here match the old sequencer's launchpad LED colors so
// the TUI and launchpad reads as the same instrument. If/when we wire
// theme.Theme in, these move into theme.Palette roles.
var (
	pianoStyleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d4d4ff"))
	pianoStyleRec      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff3030"))
	pianoStylePitchLbl = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8aaa"))
	pianoStyleEmpty    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3a48"))
	pianoStyleNote     = lipgloss.NewStyle().Foreground(lipgloss.Color("#50c8ff"))
	pianoStyleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff64c8"))
	pianoStylePlayhead = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	pianoStyleOOB      = lipgloss.NewStyle().Foreground(lipgloss.Color("#24242c"))
)

// pianoKey handles a keyboard event on the piano view.
//
// Caller (HandleKey in tui.go) holds track.Lock and calls MarkTrackDirty
// after this returns — so we only mutate spec here.
//
// Keybindings preserved from sequencer/pianoroll.go:
//
//	h / left, l / right   select previous / next note by time
//	j / down, k / up      select next / previous note by pitch
//	y / o                 move selected note earlier / later by one
//	                      horiz-edit step
//	u / i                 move selected note down / up by one vert-edit step
//	n / m                 shorten / lengthen selected note by one horiz step
//	space                 add a note at (CenterBeat, CenterPitch),
//	                      4x horiz step wide, velocity 100
//	x                     delete selected note
//	[ / ]                 pattern length - / + one beat (clamped 1..64)
//	c                     clear editing pattern
//	< / >                 previous / next pattern (. aliases >)
//	q / w                 zoom out / in
//	a / s                 rows mode: smushed / spread
//	d / f                 horiz edit step: coarser / finer
//	e / r                 vert edit step: coarser / finer
//	R                     toggle recording (capital R; lowercase r is vert-edit)
func pianoKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return
	}

	editH := pianoEditHoriz(state.EditHoriz)
	editV := pianoEditVert(state.EditVert)

	switch key {
	case "h", "left":
		// Select previous note by time (wraps).
		if len(pat.Notes) > 0 {
			state.SelectedNote--
			if state.SelectedNote < 0 {
				state.SelectedNote = len(pat.Notes) - 1
			}
			pianoCenterOnSelectionInline(state, pat)
		}
	case "l", "right":
		if len(pat.Notes) > 0 {
			state.SelectedNote++
			if state.SelectedNote >= len(pat.Notes) {
				state.SelectedNote = 0
			}
			pianoCenterOnSelectionInline(state, pat)
		}

	case "j", "down":
		pianoSelectByPitchInline(state, pat, -1)
	case "k", "up":
		pianoSelectByPitchInline(state, pat, 1)

	case "y":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			delta := -editH
			if n.Start+delta < 0 {
				delta = -n.Start
			}
			n.Start += delta
			pianoCenterOnSelectionInline(state, pat)
		}
	case "o":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			delta := editH
			if n.Start+n.Duration+delta > pat.Length {
				delta = pat.Length - n.Start - n.Duration
			}
			if delta < 0 {
				delta = 0
			}
			n.Start += delta
			pianoCenterOnSelectionInline(state, pat)
		}

	case "u":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			target := int(n.Pitch) - editV
			if target < 0 {
				target = 0
			}
			n.Pitch = uint8(target)
			pianoCenterOnSelectionInline(state, pat)
		}
	case "i":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			target := int(n.Pitch) + editV
			if target > 127 {
				target = 127
			}
			n.Pitch = uint8(target)
			pianoCenterOnSelectionInline(state, pat)
		}

	case "n":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			d := n.Duration - editH
			if d < pianoMinNoteDurationBts {
				d = pianoMinNoteDurationBts
			}
			n.Duration = d
		}
	case "m":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			d := n.Duration + editH
			if n.Start+d > pat.Length {
				d = pat.Length - n.Start
			}
			if d < pianoMinNoteDurationBts {
				d = pianoMinNoteDurationBts
			}
			n.Duration = d
		}

	case "q":
		if state.ViewScale < len(pianoViewScales)-1 {
			state.ViewScale++
		}
	case "w":
		if state.ViewScale > 0 {
			state.ViewScale--
		}
	case "a":
		state.ViewRows = pianoViewSmushed
	case "s":
		state.ViewRows = pianoViewSpread

	case "d":
		if state.EditHoriz < len(pianoEditHorizSteps)-1 {
			state.EditHoriz++
		}
	case "f":
		if state.EditHoriz > 0 {
			state.EditHoriz--
		}
	case "e":
		if state.EditVert < len(pianoEditVertSteps)-1 {
			state.EditVert++
		}
	case "r":
		if state.EditVert > 0 {
			state.EditVert--
		}

	case " ", "space":
		// Add a note at CenterBeat / CenterPitch, width = editH*4.
		start := state.CenterBeat
		if start < 0 {
			start = 0
		}
		dur := editH * 4
		if dur < pianoMinNoteDurationBts {
			dur = pianoMinNoteDurationBts
		}
		if start+dur > pat.Length {
			dur = pat.Length - start
		}
		if dur <= 0 {
			break
		}
		pitch := uint8(state.CenterPitch)
		pat.Notes = append(pat.Notes, devices.NoteEvent{
			Start: start, Duration: dur, Pitch: pitch, Velocity: 100,
		})
		state.SelectedNote = len(pat.Notes) - 1

	case "x":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			pat.Notes = append(pat.Notes[:state.SelectedNote], pat.Notes[state.SelectedNote+1:]...)
			if state.SelectedNote >= len(pat.Notes) {
				state.SelectedNote = len(pat.Notes) - 1
			}
			if state.SelectedNote >= 0 {
				pianoCenterOnSelectionInline(state, pat)
			}
		}

	case "[":
		if pat.Length > 1.0 {
			pat.Length -= 1.0
		}
	case "]":
		if pat.Length < 64.0 {
			pat.Length += 1.0
		}

	case "c":
		pat.Notes = pat.Notes[:0]
		state.SelectedNote = -1

	case "<", ",":
		if state.EditingPatternIdx > 0 {
			state.EditingPatternIdx--
			state.SelectedNote = -1
		}
	case ">", ".":
		if state.EditingPatternIdx < devices.NumPatterns-1 {
			state.EditingPatternIdx++
			state.SelectedNote = -1
		}

	case "R":
		state.Recording = !state.Recording
	}

	// Keep notes sorted by start time; preserve the user's selection by
	// matching start+pitch after the sort.
	var selected *devices.NoteEvent
	if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
		n := pat.Notes[state.SelectedNote]
		selected = &n
	}
	sort.SliceStable(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})
	if selected != nil {
		for i, n := range pat.Notes {
			if n.Start == selected.Start && n.Pitch == selected.Pitch {
				state.SelectedNote = i
				break
			}
		}
	}
}

// pianoRender returns the TUI view for the piano device.
//
// Layout (styled with lipgloss):
//
//	header:   pattern, pitch/beat center, view scale, horiz/vert edit, REC
//	grid:     state.ViewRows pitches x N cols. rows render top (highest pitch)
//	          to bottom (lowest). cols render startBeat..endBeat in beats
//	          at viewScale beats-per-col.
//	          note start:     ● (selected: ◉, playhead-overlap: ▶)
//	          note body:      ─ (overlap: ═)
//	          empty cell:     · (playhead: ▶)
//	          out-of-pattern: -
//	selected: single line with pitch / start / duration / velocity.
//	help:     widgets.RenderKeyHelp sections for all keybindings.
func pianoRender(track *model.Track, project *model.Project) string {
	if track == nil || track.Piano == nil {
		return ""
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return ""
	}

	viewScale := pianoViewScale(state.ViewScale)
	editH := pianoEditHoriz(state.EditHoriz)
	editV := pianoEditVert(state.EditVert)

	rows := state.ViewRows
	if rows <= 0 {
		rows = pianoViewSpread
	}
	cols := int(pat.Length/viewScale) + 1
	if cols < 16 {
		cols = 16
	}
	if cols > pianoMaxCols {
		cols = pianoMaxCols
	}

	// Viewport: center horizontal range on CenterBeat. Top row is the
	// highest displayed pitch; row (rows/2) holds CenterPitch.
	beatsPerCol := viewScale
	totalBeats := float64(cols) * beatsPerCol
	startBeat := state.CenterBeat - totalBeats/2
	startPitch := int(state.CenterPitch) + rows/2

	playheadBeat, playheadValid := pianoPlayingBeat(track, project)
	playheadCol := -1
	if playheadValid && state.EditingPatternIdx == state.Schedule.Playing && playheadBeat >= startBeat {
		playheadCol = int((playheadBeat - startBeat) / beatsPerCol)
	}

	vertMode := "spread"
	if state.ViewRows == pianoViewSmushed {
		vertMode = "smushed"
	}

	var b strings.Builder

	// Header — pattern, position, view, edit, rec.
	playInfo := ""
	if state.EditingPatternIdx != state.Schedule.Playing {
		playInfo = fmt.Sprintf(" (playing %d)", state.Schedule.Playing+1)
	}
	header := fmt.Sprintf("PIANO  Pattern %d/%d%s  Len %.2g  Center %s%d @ %.2f",
		state.EditingPatternIdx+1, devices.NumPatterns, playInfo,
		pat.Length,
		pianoNoteNames[int(state.CenterPitch)%12], int(state.CenterPitch)/12,
		state.CenterBeat)
	b.WriteString(pianoStyleHeader.Render(header))
	b.WriteString("\n")
	fmt.Fprintf(&b, "View: %s/col %s  Edit: %s horiz, %d semi vert",
		pianoFmtStep(viewScale), vertMode, pianoFmtStep(editH), editV)
	if state.Recording {
		b.WriteString("  ")
		b.WriteString(pianoStyleRec.Render("● REC"))
	}
	b.WriteString("\n\n")

	// Grid.
	for row := 0; row < rows; row++ {
		pitch := startPitch - row
		if pitch < 0 || pitch > 127 {
			b.WriteString("     ")
			for col := 0; col < cols; col++ {
				b.WriteString(" ")
			}
			b.WriteString("\n")
			continue
		}
		label := fmt.Sprintf("%2s%-2d ", pianoNoteNames[pitch%12], pitch/12)
		b.WriteString(pianoStylePitchLbl.Render(label))
		for col := 0; col < cols; col++ {
			colBeat := startBeat + float64(col)*beatsPerCol
			colBeatEnd := colBeat + beatsPerCol
			isPlayhead := col == playheadCol

			if colBeat < 0 || colBeat >= pat.Length {
				b.WriteString(pianoStyleOOB.Render("-"))
				continue
			}

			// Find notes crossing this cell. Track whether any note STARTS
			// here (to draw a start-cap) and whether ≥2 notes overlap here
			// (for the heavier body rune).
			startIdx := -1
			hitCount := 0
			for i := range pat.Notes {
				n := &pat.Notes[i]
				if int(n.Pitch) != pitch {
					continue
				}
				noteEnd := n.Start + n.Duration
				if n.Start < colBeatEnd && noteEnd > colBeat {
					hitCount++
					if n.Start >= colBeat && n.Start < colBeatEnd && startIdx < 0 {
						startIdx = i
					}
				}
			}

			var ch string
			var style lipgloss.Style
			switch {
			case startIdx == state.SelectedNote && startIdx >= 0:
				ch, style = "◉", pianoStyleSelected
			case startIdx >= 0 && isPlayhead:
				ch, style = "▶", pianoStylePlayhead
			case startIdx >= 0:
				ch, style = "●", pianoStyleNote
			case hitCount > 1:
				ch, style = "═", pianoStyleNote
			case hitCount == 1 && isPlayhead:
				ch, style = "▶", pianoStylePlayhead
			case hitCount == 1:
				ch, style = "─", pianoStyleNote
			case isPlayhead:
				ch, style = "▶", pianoStylePlayhead
			default:
				ch, style = "·", pianoStyleEmpty
			}
			b.WriteString(style.Render(ch))
		}
		b.WriteString("\n")
	}

	// Selected-note summary.
	if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
		n := &pat.Notes[state.SelectedNote]
		fmt.Fprintf(&b, "\nSelected: %s%d  start=%.2f  dur=%.2f  vel=%d\n",
			pianoNoteNames[int(n.Pitch)%12], int(n.Pitch)/12,
			n.Start, n.Duration, n.Velocity)
	}

	// Key help — structured via widgets (matches old sequencer/pianoroll.go).
	b.WriteString("\n")
	b.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
		{Title: "Select", Keys: []widgets.KeyBinding{
			{Key: "hjkl", Desc: "select notes"},
		}},
		{Title: "Move", Keys: []widgets.KeyBinding{
			{Key: "yuio", Desc: "move note"},
			{Key: "n / m", Desc: "shorter / longer"},
		}},
		{Title: "Notes", Keys: []widgets.KeyBinding{
			{Key: "space", Desc: "add note"},
			{Key: "x", Desc: "delete note"},
		}},
		{Title: "View", Keys: []widgets.KeyBinding{
			{Key: "q / w", Desc: "zoom out/in"},
			{Key: "a / s", Desc: "smushed/spread"},
		}},
		{Title: "Grid", Keys: []widgets.KeyBinding{
			{Key: "d / f", Desc: "horiz coarse/fine"},
			{Key: "e / r", Desc: "vert coarse/fine"},
		}},
		{Title: "Pattern", Keys: []widgets.KeyBinding{
			{Key: "< / >", Desc: "prev/next pattern"},
			{Key: "[ / ]", Desc: "length -/+"},
			{Key: "c", Desc: "clear"},
			{Key: "R", Desc: "record toggle"},
		}},
	}))
	b.WriteString("\n")

	return b.String()
}

// --- piano-view tables (zoom / edit sensitivity) ---

// pianoViewScales is beats-per-column for each zoom level.
var pianoViewScales = []float64{
	0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0, 2.0, 4.0,
}

// pianoEditHorizSteps is the beat delta for each horizontal-edit sensitivity.
var pianoEditHorizSteps = []float64{
	0.015625, 0.03125, 0.0625, 0.125, 0.25, 0.5, 1.0,
}

// pianoEditVertSteps is the semitone delta for each vertical-edit sensitivity.
var pianoEditVertSteps = []int{1, 12}

const (
	pianoViewSmushed = 12
	pianoViewSpread  = 24
)

func pianoViewScale(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoViewScales) {
		idx = len(pianoViewScales) - 1
	}
	return pianoViewScales[idx]
}

func pianoEditHoriz(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditHorizSteps) {
		idx = len(pianoEditHorizSteps) - 1
	}
	return pianoEditHorizSteps[idx]
}

func pianoEditVert(idx int) int {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditVertSteps) {
		idx = len(pianoEditVertSteps) - 1
	}
	return pianoEditVertSteps[idx]
}

// pianoFmtStep formats a beat step as a fraction (or plain number).
func pianoFmtStep(step float64) string {
	switch step {
	case 0.015625:
		return "1/64"
	case 0.03125:
		return "1/32"
	case 0.0625:
		return "1/16"
	case 0.125:
		return "1/8"
	case 0.25:
		return "1/4"
	case 0.5:
		return "1/2"
	case 1.0:
		return "1"
	case 2.0:
		return "2"
	case 4.0:
		return "4"
	default:
		return fmt.Sprintf("%.3f", step)
	}
}

// pianoCenterOnSelectionInline snaps the viewport to the selected note.
func pianoCenterOnSelectionInline(state *devices.Piano, pat *devices.PianoPattern) {
	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[state.SelectedNote]
	state.CenterBeat = n.Start
	state.CenterPitch = float64(n.Pitch)
}

// pianoSelectByPitchInline walks through notes in pitch order, picking the
// nearest-in-start-time note at the next/previous used pitch.
func pianoSelectByPitchInline(state *devices.Piano, pat *devices.PianoPattern, direction int) {
	if len(pat.Notes) == 0 {
		return
	}
	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		state.SelectedNote = 0
		pianoCenterOnSelectionInline(state, pat)
		return
	}

	current := pat.Notes[state.SelectedNote]
	targetPitch := int(current.Pitch) + direction

	bestIdx := -1
	bestDist := 1000.0
	for p := targetPitch; p >= 0 && p <= 127; p += direction {
		for i, n := range pat.Notes {
			if int(n.Pitch) != p {
				continue
			}
			dist := n.Start - current.Start
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			break
		}
		if direction == 0 {
			break
		}
	}
	if bestIdx >= 0 {
		state.SelectedNote = bestIdx
		pianoCenterOnSelectionInline(state, pat)
	}
}

// pianoPlayingBeat returns the current playback position (in beats) within
// the editing pattern, or (0, false) when unavailable.
func pianoPlayingBeat(track *model.Track, project *model.Project) (float64, bool) {
	if track.Piano == nil {
		return 0, false
	}
	playingIdx := track.Piano.Schedule.Playing
	if playingIdx < 0 || playingIdx >= len(track.Piano.Patterns) {
		return 0, false
	}
	playingPat := track.Piano.Patterns[playingIdx]
	if playingPat == nil {
		return 0, false
	}
	trackIdx := -1
	for i := 0; i < 8; i++ {
		if project.Tracks[i] == track {
			trackIdx = i
			break
		}
	}
	if trackIdx < 0 {
		return 0, false
	}
	cursor := project.Playback.Cursors[trackIdx]
	slot := cursor.CurrentSlot
	if slot < 0 || slot > 1 {
		slot = 0
	}
	m := playingPat.Machine[slot].Load()
	if m == nil || m.Length <= 0 {
		return 0, false
	}
	globalTick := project.Playback.Tick.Load()
	pos := globalTick - cursor.T0Tick
	posInPattern := ((pos % m.Length) + m.Length) % m.Length
	return float64(posInPattern) / float64(devices.PPQ), true
}
