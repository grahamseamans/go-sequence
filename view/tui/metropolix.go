package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
	"go-sequence/theme"
)

// metropolixModeNames mirrors the PlaybackMode enum order.
var metropolixModeNames = [...]string{"Forward", "Reverse", "Pendulum", "Random"}

// metropolixScaleNames mirrors the ScaleType enum order (must match the
// iota order in model/devices/metropolix.go).
var metropolixScaleNames = [...]string{
	"Chromatic", "Major", "Minor", "Pentatonic",
	"Dorian", "Phrygian", "Lydian", "Mixolydian", "Locrian",
	"Harm Min", "Mel Min", "Blues", "Whole Tone",
	"Dim H-W", "Dim W-H", "Hungarian", "Dbl Harm",
	"Phryg Dom", "Hirajoshi", "In Sen", "Yo", "Bhairavi",
}

// metropolixNoteNames maps 0..11 to the 12-tone chromatic names.
var metropolixNoteNames = [...]string{
	"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B",
}

// metropolixPageNames is the display name for each of the 8 Launchpad pages.
// Indexed by spec.Page.
var metropolixPageNames = [...]string{
	"Main", "Octave", "Pulse", "Ratchets",
	"Probability", "Gate", "Accumulator", "Settings",
}

// metropolixAccumSubPageNames is the display name for each accumulator
// sub-page. Indexed by spec.AccumSubPage.
var metropolixAccumSubPageNames = [...]string{"Value", "Reset", "Mode"}

// metropolixAccumModeNames is the display name for each accumulator mode.
// Indexed by stage.AccumMode (0..2).
var metropolixAccumModeNames = [...]string{"Reset", "PingPong", "Hold"}

// metropolixGateLengthNames is the display name for each gate length index.
// Indexed by stage.GateLength (0..5).
var metropolixGateLengthNames = [...]string{"Trg", "1/16", "1/8", "1/4", "1/2", "Full"}

// metropolixKey handles a keystroke on the Metropolix TUI view.
//
// Caller (HandleKey in tui.go) holds track.Lock and calls MarkTrackDirty
// after this returns.
//
// Keybindings preserved (modulo conflicts with the new global hotkeys —
// space/+/=/-/1-8/,/S/tab are all consumed by tui.go before we see them):
//
//	h / left             select previous stage
//	l / right            select next stage
//	j / down             pitch down (Note--; rolls into prev octave at 0)
//	k / up               pitch up   (Note++; rolls into next octave at 7)
//	g                    toggle gate on the selected stage (was space)
//	s                    toggle slide on the selected stage
//	r / R                ratchets -/+    (clamped 1..8)
//	p / P                probability -/+10  (clamped 0..100)
//	a / A                accumulator -/+    (clamped -4..+3)
//	u / U                pulse count -/+    (clamped 1..8)
//	o / O                octave      -/+    (clamped 0..7)
//	[  /  ]              length -/+
//	<  or .              next scale (wrap)         (',' is a global hotkey)
//	>                    next scale  — alias for '.'
//	z                    root note --             ('-' is a global hotkey)
//	x                    root note ++
//	m                    cycle playback mode
//	{  /  }              prev / next editing pattern
//	f                    next page (wraps 0..7)
//	F                    prev page
//	0..7 (not keys 1..8) — direct page select via SHIFT+number:
//	                       '!','@','#','$','%','^','&','*' → pages 0..7
//	i / I                next / prev accumulator sub-page (pages 6 only)
//	G                    next gate length index on selected stage (page 5)
//	B                    cycle AccumMode on selected stage (page 6)
//	y / Y                gate-length -- on selected stage (inverse of G)
//	c                    clear-pattern (opens confirm dialog)
//	n                    confirm dialog: no / cancel
//	Y (yes, uppercase)   confirm dialog: yes
//	esc                  close confirm dialog
//
// Notes on divergences from the old UI:
//   - 'g' toggles gate (was space; space is a global transport key in the
//     new TUI).
//   - '<' / '>' select scale (was 'q'); '{' / '}' switch patterns.
//   - '-'/'+' can't be used for root note (globals); we use z/x (matches old).
//   - page switching gets letter keys 'f'/'F' plus shift+number alternatives
//     because 1..8 are the global track-focus hotkeys.
func metropolixKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Confirm dialog consumes keys exclusively while open.
	if spec.ConfirmKind != devices.MetropolixConfirmNone {
		metropolixConfirmKey(spec, pat, key)
		return
	}

	// Clamp Selected defensively — Length may have shrunk since last call.
	if spec.Selected >= pat.Length {
		spec.Selected = pat.Length - 1
	}
	if spec.Selected < 0 {
		spec.Selected = 0
	}
	stage := &pat.Stages[spec.Selected]

	switch key {
	case "h", "left":
		if spec.Selected > 0 {
			spec.Selected--
		}
	case "l", "right":
		if spec.Selected < pat.Length-1 {
			spec.Selected++
		}
	case "j", "down":
		if stage.Note > 0 {
			stage.Note--
		} else if stage.Octave > 0 {
			stage.Octave--
			stage.Note = 7
		}
	case "k", "up":
		if stage.Note < 7 {
			stage.Note++
		} else if stage.Octave < 7 {
			stage.Octave++
			stage.Note = 0
		}
	case "g":
		stage.Gate = !stage.Gate
	case "s":
		stage.Slide = !stage.Slide
	case "r":
		if stage.Ratchets > 1 {
			stage.Ratchets--
		}
	case "R":
		if stage.Ratchets < 8 {
			stage.Ratchets++
		}
	case "p":
		stage.Probability -= 10
		if stage.Probability < 0 {
			stage.Probability = 0
		}
	case "P":
		stage.Probability += 10
		if stage.Probability > 100 {
			stage.Probability = 100
		}
	case "a":
		if stage.Accumulator > -4 {
			stage.Accumulator--
		}
	case "A":
		if stage.Accumulator < 3 {
			stage.Accumulator++
		}
	case "u":
		if stage.PulseCount > 1 {
			stage.PulseCount--
		}
	case "U":
		if stage.PulseCount < 8 {
			stage.PulseCount++
		}
	case "o":
		if stage.Octave > 0 {
			stage.Octave--
		}
	case "O":
		if stage.Octave < 7 {
			stage.Octave++
		}
	case "m":
		pat.Mode = devices.PlaybackMode((int(pat.Mode) + 1) % 4)
	case "[":
		if pat.Length > 1 {
			pat.Length--
			if spec.Selected >= pat.Length {
				spec.Selected = pat.Length - 1
			}
		}
	case "]":
		if pat.Length < 8 {
			pat.Length++
		}
	case "<":
		pat.Scale = (pat.Scale - 1 + devices.ScaleCount) % devices.ScaleCount
	case ">", ".":
		pat.Scale = (pat.Scale + 1) % devices.ScaleCount
	case "z":
		if pat.RootNote > 0 {
			pat.RootNote--
		}
	case "x":
		if pat.RootNote < 127 {
			pat.RootNote++
		}
	case "{":
		if spec.EditingPatternIdx > 0 {
			spec.EditingPatternIdx--
		}
	case "}":
		if spec.EditingPatternIdx < devices.NumPatterns-1 {
			spec.EditingPatternIdx++
		}
	case "f":
		spec.Page = (spec.Page + 1) % 8
	case "F":
		spec.Page = (spec.Page + 7) % 8
	case "!":
		spec.Page = 0
	case "@":
		spec.Page = 1
	case "#":
		spec.Page = 2
	case "$":
		spec.Page = 3
	case "%":
		spec.Page = 4
	case "^":
		spec.Page = 5
	case "&":
		spec.Page = 6
	case "*":
		spec.Page = 7
	case "i":
		if spec.AccumSubPage < 2 {
			spec.AccumSubPage++
		}
	case "I":
		if spec.AccumSubPage > 0 {
			spec.AccumSubPage--
		}
	case "G":
		if stage.GateLength < 5 {
			stage.GateLength++
		}
	case "y":
		if stage.GateLength > 0 {
			stage.GateLength--
		}
	case "B":
		stage.AccumMode = (stage.AccumMode + 1) % 3
	case "c":
		spec.ConfirmKind = devices.MetropolixConfirmClearPattern
		spec.ConfirmMsg = fmt.Sprintf("Clear pattern %d?", spec.EditingPatternIdx+1)
	}
}

// metropolixConfirmKey handles keys while the confirm dialog is open.
// 'Y' confirms, 'n'/'N'/'esc' cancels. (Note: 'y' lowercase is used by the
// main-screen gate-length -- binding above, so the confirm uses uppercase Y.)
func metropolixConfirmKey(spec *devices.Metropolix, pat *devices.MetropolixPattern, key string) {
	switch key {
	case "Y":
		switch spec.ConfirmKind {
		case devices.MetropolixConfirmClearPattern:
			metropolixClearPattern(pat)
		}
		spec.ConfirmKind = devices.MetropolixConfirmNone
		spec.ConfirmMsg = ""
	case "n", "N", "esc":
		spec.ConfirmKind = devices.MetropolixConfirmNone
		spec.ConfirmMsg = ""
	}
}

// metropolixClearPattern resets a MetropolixPattern to the default
// (8 stages, gate on, pulse=1, ratchets=1, probability=100, notes walking
// 0..7, octave 4, scale Major, root C4, mode Forward, length 8, slide time 3).
func metropolixClearPattern(pat *devices.MetropolixPattern) {
	pat.Length = 8
	pat.Mode = devices.ModeForward
	pat.Scale = devices.ScaleMajor
	pat.RootNote = 60
	pat.SlideTime = 3
	for i := 0; i < 8; i++ {
		pat.Stages[i] = devices.MetropolixStage{
			Octave:      4,
			Note:        i,
			Gate:        true,
			PulseCount:  1,
			Ratchets:    1,
			Probability: 100,
		}
	}
}

// metropolixRender returns the TUI view for the Metropolix device.
//
// Layout:
//
//	header:  pattern, mode, scale, root, length, selected stage, page
//	table:   one column per stage (1..Length), one row per parameter
//	         (Pitch, Gate, Pulse, Ratch, Prob, Slide, Accum, AReset, AMode,
//	         GateLen). Selected stage marked with '>' above the column.
//	footer:  key-help summary, plus accumulator sub-page indicator on page 6.
//
// Styling is theme-driven: header, row labels, and the selected-stage column
// are tinted; other cells are plain. When no theme is available everything
// renders as plain text. If a confirm dialog is active we replace the normal
// view with a simple Y/n prompt.
func metropolixRender(track *model.Track, project *model.Project, th *theme.Theme) string {
	if track == nil || track.Metropolix == nil {
		return ""
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
		return ""
	}

	if spec.Selected >= pat.Length {
		spec.Selected = pat.Length - 1
	}
	if spec.Selected < 0 {
		spec.Selected = 0
	}

	if spec.ConfirmKind != devices.MetropolixConfirmNone {
		return metropolixRenderConfirm(spec, th)
	}

	headerStyle := themeStyle(th, roleFG).Bold(true)
	rowLblStyle := themeStyle(th, roleMuted)
	selPointerStyle := themeStyle(th, roleAccent).Bold(true)
	stageNumStyle := themeStyle(th, roleFG)
	selStageNumStyle := themeStyle(th, roleCursor).Bold(true)

	var b strings.Builder

	playInfo := ""
	if spec.EditingPatternIdx != spec.Schedule.Playing {
		playInfo = fmt.Sprintf(" (playing %d)", spec.Schedule.Playing+1)
	}
	header := fmt.Sprintf("Metropolix  Pattern %d/%d%s  Mode %s  Scale %s  Root %s  Len %d  Stage %d  Page %d:%s",
		spec.EditingPatternIdx+1, devices.NumPatterns, playInfo,
		metropolixModeName(pat.Mode),
		metropolixScaleName(pat.Scale),
		metropolixPitchName(int(pat.RootNote)),
		pat.Length,
		spec.Selected+1,
		spec.Page,
		metropolixPageName(spec.Page),
	)
	if spec.Page == 6 {
		header += fmt.Sprintf("/%s", metropolixAccumSubPageName(spec.AccumSubPage))
	}
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n\n")

	// Selection-pointer row: '>' above the selected stage column.
	b.WriteString("           ")
	for i := 0; i < pat.Length; i++ {
		if i == spec.Selected {
			b.WriteString(selPointerStyle.Render("  >  "))
		} else {
			b.WriteString("     ")
		}
	}
	b.WriteString("\n")

	// Column header: stage numbers. Selected column highlighted.
	b.WriteString(rowLblStyle.Render("Stage      "))
	for i := 0; i < pat.Length; i++ {
		cell := fmt.Sprintf("  %d  ", i+1)
		if i == spec.Selected {
			b.WriteString(selStageNumStyle.Render(cell))
		} else {
			b.WriteString(stageNumStyle.Render(cell))
		}
	}
	b.WriteString("\n")

	selCellStyle := themeStyle(th, roleCursor).Bold(true)
	// writeRow emits a tinted row label + a per-stage cell. cell(i) returns
	// the formatted text for stage i; we wrap the selected stage's cell in
	// selCellStyle so the full row visibly tracks the selection.
	writeRow := func(label string, cell func(i int) string) {
		b.WriteString(rowLblStyle.Render(label))
		for i := 0; i < pat.Length; i++ {
			s := cell(i)
			if i == spec.Selected {
				b.WriteString(selCellStyle.Render(s))
			} else {
				b.WriteString(s)
			}
		}
		b.WriteString("\n")
	}

	// Pitch row — scale-degree / octave label.
	writeRow("Pitch      ", func(i int) string {
		return fmt.Sprintf(" %-4s", metropolixStagePitchLabel(&pat.Stages[i]))
	})

	// Gate row.
	writeRow("Gate       ", func(i int) string {
		ch := "."
		if pat.Stages[i].Gate {
			ch = "X"
		}
		return fmt.Sprintf("  %s  ", ch)
	})

	// Pulse count.
	writeRow("Pulse      ", func(i int) string {
		return fmt.Sprintf("  %d  ", pat.Stages[i].PulseCount)
	})

	// Ratchets.
	writeRow("Ratch      ", func(i int) string {
		return fmt.Sprintf("  %d  ", pat.Stages[i].Ratchets)
	})

	// Probability (0..100).
	writeRow("Prob       ", func(i int) string {
		return fmt.Sprintf(" %3d ", pat.Stages[i].Probability)
	})

	// Slide.
	writeRow("Slide      ", func(i int) string {
		ch := "."
		if pat.Stages[i].Slide {
			ch = "~"
		}
		return fmt.Sprintf("  %s  ", ch)
	})

	// Accumulator (signed -4..+3).
	writeRow("Accum      ", func(i int) string {
		return fmt.Sprintf(" %+2d  ", pat.Stages[i].Accumulator)
	})

	// AccumReset.
	writeRow("AReset     ", func(i int) string {
		return fmt.Sprintf("  %d  ", pat.Stages[i].AccumReset)
	})

	// AccumMode (short name: Rst/P-P/Hld).
	writeRow("AMode      ", func(i int) string {
		return fmt.Sprintf(" %-4s", metropolixAccumModeShort(pat.Stages[i].AccumMode))
	})

	// GateLength.
	writeRow("GateLen    ", func(i int) string {
		return fmt.Sprintf(" %-4s", metropolixGateLengthName(pat.Stages[i].GateLength))
	})

	// Footer / key help.
	b.WriteString("\n")
	b.WriteString("  h/l select   j/k pitch   g gate   s slide   r/R ratch   p/P prob   a/A accum\n")
	b.WriteString("  u/U pulse    o/O octave  G/y gateLen  B accumMode   i/I accumSubPage\n")
	b.WriteString("  [/] length   </> scale   z/x root    m mode   {/} prev/next pattern\n")
	b.WriteString("  f/F page cycle   !@#$%^&* (shift+1..8) direct page   c clear pattern\n")

	return b.String()
}

// metropolixRenderConfirm renders the modal clear-pattern confirm dialog.
// The view is intentionally simple so there's no ambiguity about which
// prompt the user is answering.
func metropolixRenderConfirm(spec *devices.Metropolix, th *theme.Theme) string {
	titleStyle := themeStyle(th, roleWarning).Bold(true)
	msgStyle := themeStyle(th, roleFG).Bold(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Metropolix — confirm"))
	b.WriteString("\n\n  ")
	b.WriteString(msgStyle.Render(spec.ConfirmMsg))
	b.WriteString("\n\n")
	b.WriteString("  [Y] Yes    [n] No    [esc] cancel\n")
	return b.String()
}

// metropolixModeName returns the display name for a PlaybackMode, or "?" if
// the value is out of range.
func metropolixModeName(m devices.PlaybackMode) string {
	if m < 0 || int(m) >= len(metropolixModeNames) {
		return "?"
	}
	return metropolixModeNames[m]
}

// metropolixScaleName returns the display name for a ScaleType, or "?" if
// the value is out of range.
func metropolixScaleName(s devices.ScaleType) string {
	if s < 0 || int(s) >= len(metropolixScaleNames) {
		return "?"
	}
	return metropolixScaleNames[s]
}

// metropolixPageName returns the display name for a page index (0..7), or
// "?" if out of range.
func metropolixPageName(p int) string {
	if p < 0 || p >= len(metropolixPageNames) {
		return "?"
	}
	return metropolixPageNames[p]
}

// metropolixAccumSubPageName returns the display name for a sub-page
// index (0..2), or "?" if out of range.
func metropolixAccumSubPageName(sp int) string {
	if sp < 0 || sp >= len(metropolixAccumSubPageNames) {
		return "?"
	}
	return metropolixAccumSubPageNames[sp]
}

// metropolixAccumModeShort returns a short 3-letter name for an AccumMode
// value (0..2), or "?" if out of range.
func metropolixAccumModeShort(m int) string {
	switch m {
	case 0:
		return "Rst"
	case 1:
		return "P-P"
	case 2:
		return "Hld"
	}
	return "?"
}

// metropolixGateLengthName returns the display name for a gate length index
// (0..5), or "?" if out of range.
func metropolixGateLengthName(gl int) string {
	if gl < 0 || gl >= len(metropolixGateLengthNames) {
		return "?"
	}
	return metropolixGateLengthNames[gl]
}

// metropolixPitchName returns a short MIDI-pitch label ("C4", "G#3", ...).
func metropolixPitchName(pitch int) string {
	if pitch < 0 {
		pitch = 0
	}
	if pitch > 127 {
		pitch = 127
	}
	octave := pitch/12 - 1
	return fmt.Sprintf("%s%d", metropolixNoteNames[pitch%12], octave)
}

// metropolixStagePitchLabel returns a compact label for a stage's editable
// pitch fields — "<scale-degree>/<octave>", e.g. "3/4". We deliberately
// don't resolve the full MIDI pitch here (that requires the scale-intervals
// table which lives in the controller package); the raw editable values
// are clearer for the UI.
func metropolixStagePitchLabel(stage *devices.MetropolixStage) string {
	return fmt.Sprintf("%d/%d", stage.Note, stage.Octave)
}
