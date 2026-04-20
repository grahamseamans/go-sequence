package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
	"go-sequence/theme"
)

// drumKey handles a keyboard event on the drum view.
//
// Caller (HandleKey in tui.go) holds track.Lock and calls MarkTrackDirty
// after this returns. We just mutate spec.
//
// Keybindings (ported from old sequencer/drum.go HandleKey, adapted to the
// new model — see the comment at the top of each case):
//
// When a confirm dialog is open (state.ConfirmKind != ConfirmNone):
//
//	y / Y                 confirm (run the pending destructive action)
//	n / N / esc / q /     dismiss (any other key dismisses too; matches old
//	    any other key     "any other key cancels" UX hint)
//
// Otherwise:
//
//	h / left              move cursor left through steps
//	l / right             move cursor right
//	j / down              select next drum lane (0..15)
//	k / up                select previous drum lane
//	space / x             toggle the step under the cursor
//	[ / ]                 decrease / increase pattern length, in 16th-note
//	                      steps (new model only has a pattern-global length)
//	c                     clear the selected lane -> confirm dialog
//	C                     clear the whole editing pattern -> confirm dialog
//	< or ,                previous pattern
//	> or .                next pattern
//	r / R                 toggle record-arm
//	p / P                 toggle preview-arm
//
// Dropped from the old UI:
//
//	per-lane length       the new model has pattern-global length only.
//	                      [ / ] now resize the pattern, not a lane.
func drumKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Confirm-dialog state takes over the entire keymap. y confirms, any
	// other key dismisses. Matches the old HandleKey's confirmMode branch.
	if state.ConfirmKind != devices.ConfirmNone {
		switch key {
		case "y", "Y":
			switch state.ConfirmKind {
			case devices.ConfirmClearNote:
				for i := range pat.Notes[state.SelectedNoteIdx].Steps {
					pat.Notes[state.SelectedNoteIdx].Steps[i] = devices.DrumStep{}
				}
			case devices.ConfirmClearPattern:
				for l := range pat.Notes {
					for s := range pat.Notes[l].Steps {
						pat.Notes[l].Steps[s] = devices.DrumStep{}
					}
				}
			}
		}
		// Any key dismisses (y having already executed above).
		state.ConfirmKind = devices.ConfirmNone
		state.ConfirmMsg = ""
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
	case "[":
		// Shrink pattern by one 16th-note step. Minimum is 1 step so the
		// compiled pattern always has a non-zero Length.
		stepTicks := devices.PPQ / 4
		if pat.Length > stepTicks {
			pat.Length -= stepTicks
		}
	case "]":
		// Grow pattern by one 16th-note step, capped at 32 steps to match
		// the NoteLane.Steps array bound.
		stepTicks := devices.PPQ / 4
		if pat.Length < 32*stepTicks {
			pat.Length += stepTicks
		}
	case "c":
		// Open confirm dialog only if there's content to clear.
		if drumLaneHasAnyStep(&pat.Notes[state.SelectedNoteIdx]) {
			state.ConfirmKind = devices.ConfirmClearNote
			state.ConfirmMsg = fmt.Sprintf("Clear lane %d?", state.SelectedNoteIdx+1)
		}
	case "C":
		if drumPatternHasAnyStep(pat) {
			state.ConfirmKind = devices.ConfirmClearPattern
			state.ConfirmMsg = fmt.Sprintf("Clear pattern %d?", state.EditingPatternIdx+1)
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
	case "p", "P":
		state.Preview = !state.Preview
	}
}

// drumRender returns the TUI view for the drum device.
//
// Layout (ported from old sequencer/drum.go View()):
//
//	header: "DRUM Pattern N[ (playing:M)] Step S/L Note X [REC] [PRE]"
//	        — playing-pattern info shown only when editing != playing.
//	        — REC / PRE flags shown when recording / preview armed.
//	confirm: when ConfirmKind != ConfirmNone, a dialog block replaces the
//	         main grid; y confirms, any other key dismisses.
//	grid:   16 lanes (rows) x up to 32 steps (cols), single char per cell.
//	        — beyond the pattern length: blank '-' (or '□' under the cursor).
//	        — on the playhead step (active lane, playing pattern): '▶' / '▷'.
//	        — active step: '●' / '◉' (cursor).
//	        — empty step: '·' / '○' (cursor).
//	footer: key-help summary.
func drumRender(track *model.Track, project *model.Project, th *theme.Theme) string {
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

	sym := themeSymbols(th)
	headerStyle := themeStyle(th, roleFG).Bold(true)
	recStyle := themeStyle(th, roleWarning).Bold(true)
	preStyle := themeStyle(th, roleAccent).Bold(true)
	laneLblStyle := themeStyle(th, roleMuted)
	laneSelStyle := themeStyle(th, roleAccent).Bold(true)
	stepActiveStyle := themeStyle(th, roleActive)
	stepPlayheadStyle := themeStyle(th, roleSuccess).Bold(true)
	stepCursorStyle := themeStyle(th, roleCursor).Bold(true)
	stepEmptyStyle := themeStyle(th, roleMuted)
	stepBeyondStyle := themeStyle(th, roleMuted)
	confirmBorderStyle := themeStyle(th, roleWarning)
	confirmMsgStyle := themeStyle(th, roleFG).Bold(true)

	var b strings.Builder
	// Header
	playInfo := ""
	if state.EditingPatternIdx != state.Schedule.Playing {
		playInfo = fmt.Sprintf(" (playing:%d)", state.Schedule.Playing+1)
	}
	header := fmt.Sprintf("DRUM  Pattern %d%s  Step %d/%d  Note %d",
		state.EditingPatternIdx+1, playInfo, state.Cursor+1, stepsPerPattern,
		state.SelectedNoteIdx+1)
	b.WriteString(headerStyle.Render(header))
	if state.Recording {
		b.WriteString("  ")
		b.WriteString(recStyle.Render("REC"))
	}
	if state.Preview {
		b.WriteString("  ")
		b.WriteString(preStyle.Render("PRE"))
	}
	b.WriteString("\n\n")

	// Confirm dialog takes over the body. Mirrors the old View() block.
	if state.ConfirmKind != devices.ConfirmNone {
		border := "─────────────────────────────────────────────────"
		b.WriteString(confirmBorderStyle.Render(border))
		b.WriteString("\n\n  ")
		b.WriteString(confirmMsgStyle.Render(state.ConfirmMsg))
		b.WriteString("\n\n  [y] Yes    [n] No\n\n")
		b.WriteString(confirmBorderStyle.Render(border))
		b.WriteString("\n")
		return b.String()
	}

	// Grid: 16 lanes, 32 columns (always full width — the old UI drew the
	// full 32-col grid and marked "beyond length" cells specially).
	for lane := 0; lane < 16; lane++ {
		selected := lane == state.SelectedNoteIdx
		selMark := " "
		if selected {
			selMark = ">"
		}
		lbl := fmt.Sprintf("%s%2d ", selMark, lane+1)
		if selected {
			b.WriteString(laneSelStyle.Render(lbl))
		} else {
			b.WriteString(laneLblStyle.Render(lbl))
		}
		for step := 0; step < 32; step++ {
			isCursor := selected && step == state.Cursor
			// Only the selected lane shows the playhead — the playhead lives
			// on the currently-editing pattern (see drumPlayingStep, which
			// returns -1 when editing != playing).
			isPlayhead := selected && step == playingStep
			beyondLen := step >= stepsPerPattern
			active := pat.Notes[lane].Steps[step].Active

			var ch rune
			var style = stepEmptyStyle
			switch {
			case beyondLen && isCursor:
				ch, style = sym.CursorBeyond, stepBeyondStyle
			case beyondLen:
				ch, style = sym.StepBeyond, stepBeyondStyle
			case isPlayhead && isCursor:
				ch, style = sym.CursorPlayhead, stepPlayheadStyle
			case isPlayhead:
				ch, style = sym.StepPlayhead, stepPlayheadStyle
			case active && isCursor:
				ch, style = sym.CursorActive, stepCursorStyle
			case active:
				ch, style = sym.StepActive, stepActiveStyle
			case isCursor:
				ch, style = sym.CursorEmpty, stepCursorStyle
			default:
				ch, style = sym.StepEmpty, stepEmptyStyle
			}
			b.WriteString(style.Render(string(ch)))
		}
		b.WriteString("\n")
	}

	// Footer / key-help
	b.WriteString("\n")
	b.WriteString("  h/l move cursor   j/k select lane   space/x toggle step\n")
	b.WriteString("  [ / ]  shorten/lengthen pattern     < >  prev/next pattern\n")
	b.WriteString("  c clear lane   C clear pattern   r record   p preview\n")

	return b.String()
}

// drumLaneHasAnyStep returns true if any step in the lane is active.
func drumLaneHasAnyStep(lane *devices.NoteLane) bool {
	for i := range lane.Steps {
		if lane.Steps[i].Active {
			return true
		}
	}
	return false
}

// drumPatternHasAnyStep returns true if any step in any lane is active.
func drumPatternHasAnyStep(pat *devices.DrumPattern) bool {
	for i := range pat.Notes {
		if drumLaneHasAnyStep(&pat.Notes[i]) {
			return true
		}
	}
	return false
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
		// Playhead only makes sense on the pattern the user is editing; the
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
