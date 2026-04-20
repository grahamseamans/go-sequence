package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// drumKey handles a keyboard event on the drum view.
//
// Caller (HandleKey in tui.go) holds track.Lock and calls MarkTrackDirty
// after this returns. We just mutate the spec.
//
// Keybindings preserved from the old sequencer/drum.go:
//
//	h / left              move edit cursor left (within 32 steps)
//	l / right             move edit cursor right
//	j / down              select next drum lane (0..15)
//	k / up                select previous drum lane
//	space                 toggle the step under the cursor
//	c                     clear the selected lane (no confirm)
//	C                     clear the whole editing pattern (no confirm)
//	< or ,                previous pattern
//	> or .                next pattern
//	r / R                 toggle record-arm
//
// Dropped from the old UI:
//
//	[ / ]                 per-lane length — new model has pattern-global
//	                      length only, not per-lane. Dropped.
//	y / n / esc           confirm-dialog — no dialog in new UI (actions
//	                      are immediate).
//
// Added:
//
//	x                     alias for space (toggle step at cursor).
func drumKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return
	}

	switch key {
	case "h", "left":
		if state.Cursor > 0 {
			state.Cursor--
		}
	case "l", "right":
		if state.Cursor < 31 {
			state.Cursor++
		}
	case "j", "down":
		if state.SelectedNoteIdx < 15 {
			state.SelectedNoteIdx++
		}
	case "k", "up":
		if state.SelectedNoteIdx > 0 {
			state.SelectedNoteIdx--
		}
	case " ", "space", "x":
		s := &pat.Notes[state.SelectedNoteIdx].Steps[state.Cursor]
		s.Active = !s.Active
		if s.Active && s.Velocity == 0 {
			s.Velocity = 100
		}
	case "c":
		for i := range pat.Notes[state.SelectedNoteIdx].Steps {
			pat.Notes[state.SelectedNoteIdx].Steps[i] = devices.DrumStep{}
		}
	case "C":
		for l := range pat.Notes {
			for s := range pat.Notes[l].Steps {
				pat.Notes[l].Steps[s] = devices.DrumStep{}
			}
		}
	case "<", ",":
		if state.EditingPatternIdx > 0 {
			state.EditingPatternIdx--
		}
	case ">", ".":
		if state.EditingPatternIdx < devices.NumPatterns-1 {
			state.EditingPatternIdx++
		}
	case "r", "R":
		state.Recording = !state.Recording
	}
}

// drumRender returns the TUI view for the drum device.
//
// Layout:
//
//	header: pattern, cursor, kit, length, REC flag
//	grid:   16 lanes (rows) x 32 steps (cols)
//	        column headers mark the current playhead (if any)
//	        active steps: active, inactive steps: dot
//	        cursor cell is highlighted
//	footer: key-help summary
//
// Plain-text only — no colour/styling in this first pass. Styling via a
// theme package is a follow-up.
func drumRender(track *model.Track, project *model.Project) string {
	if track == nil || track.Drum == nil {
		return ""
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return ""
	}

	stepsPerPattern := pat.Length / (devices.PPQ / 4)
	if stepsPerPattern < 1 {
		stepsPerPattern = 1
	}
	if stepsPerPattern > 32 {
		stepsPerPattern = 32
	}

	playingStep := drumPlayingStep(track, project)

	kit := devices.GetKit(track.Kit)

	var b strings.Builder
	// Header
	rec := ""
	if state.Recording {
		rec = " REC"
	}
	playInfo := ""
	if state.EditingPatternIdx != state.Schedule.Playing {
		playInfo = fmt.Sprintf(" (playing %d)", state.Schedule.Playing+1)
	}
	fmt.Fprintf(&b, "Drum  Pattern %d/%d%s  Lane %d  Cursor %d/%d  Kit %s  Len %d%s\n\n",
		state.EditingPatternIdx+1, devices.NumPatterns, playInfo,
		state.SelectedNoteIdx+1, state.Cursor+1, stepsPerPattern,
		kit.Name, stepsPerPattern, rec)

	// Playhead column indicator — a down-arrow above the playing step.
	b.WriteString("    ")
	for step := 0; step < stepsPerPattern; step++ {
		if step == playingStep {
			b.WriteString("v")
		} else {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n")

	// Grid: 16 lanes, each with stepsPerPattern cells.
	for lane := 0; lane < 16; lane++ {
		selMark := " "
		if lane == state.SelectedNoteIdx {
			selMark = ">"
		}
		fmt.Fprintf(&b, "%s%2d ", selMark, lane+1)
		for step := 0; step < stepsPerPattern; step++ {
			isCursor := lane == state.SelectedNoteIdx && step == state.Cursor
			active := pat.Notes[lane].Steps[step].Active

			var ch string
			switch {
			case isCursor && active:
				ch = "#"
			case isCursor:
				ch = "_"
			case active:
				ch = "X"
			default:
				ch = "."
			}
			b.WriteString(ch)
		}
		b.WriteString("\n")
	}

	// Footer / key-help
	b.WriteString("\n")
	b.WriteString("  h/l move cursor   j/k select lane   space toggle   r record\n")
	b.WriteString("  c clear lane      C clear pattern   < > prev/next pattern\n")

	return b.String()
}

// drumPlayingStep returns the currently-playing step index (0..31) for this
// track, or -1 when unavailable (not playing, uncompiled pattern, or the
// edit view is on a different pattern than the one playing).
func drumPlayingStep(track *model.Track, project *model.Project) int {
	if track.Drum == nil {
		return -1
	}
	playingIdx := track.Drum.Schedule.Playing
	if playingIdx < 0 || playingIdx >= len(track.Drum.Patterns) {
		return -1
	}
	playingPat := track.Drum.Patterns[playingIdx]
	if playingPat == nil {
		return -1
	}
	if track.Drum.EditingPatternIdx != playingIdx {
		// Playhead only makes sense on the lane the user is editing; the
		// edit and playing patterns differ, so don't draw a playhead.
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
	stepIdx := int(posInPattern / (devices.PPQ / 4))
	if stepIdx < 0 || stepIdx >= 32 {
		return -1
	}
	return stepIdx
}
