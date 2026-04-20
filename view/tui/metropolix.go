package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
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

// metropolixKey handles a keystroke on the Metropolix TUI view.
//
// Caller (HandleKey in tui.go) holds track.Lock and calls MarkTrackDirty
// after this returns. We just mutate spec.
//
// Keybindings (per task spec):
//
//	h / left             select previous stage
//	l / right            select next stage
//	j / down             pitch down (Note--; rolls to prev octave at 0)
//	k / up               pitch up   (Note++; rolls to next octave at 7)
//	g                    toggle gate on the selected stage
//	s                    toggle slide on the selected stage
//	[                    length --
//	]                    length ++
//	<  or ,              previous scale  (wrap)
//	>  or .              next scale      (wrap)
//	-                    root note --
//	+  or =              root note ++
//	p                    probability -10 (clamped 0..100)
//	P                    probability +10
//	r                    ratchets --    (clamped 1..8)
//	R                    ratchets ++
//	m                    cycle playback mode (Forward / Reverse / Pendulum / Random)
//	{  or n              previous editing pattern
//	}  or N              next editing pattern
//
// Notes on divergences from the old UI:
//   - 'g' toggles gate (was space; space is a global transport key in the new
//     TUI, so we use 'g' instead).
//   - '<' and '>' select scale (was 'q'); '<'/'>' are more discoverable as
//     "previous/next" and the old '<>' pattern-switch now lives on '{/}'.
//   - '-'/'+' set root note (was 'z'/'x').
func metropolixKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
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
	case "<", ",":
		pat.Scale = (pat.Scale - 1 + devices.ScaleCount) % devices.ScaleCount
	case ">", ".":
		pat.Scale = (pat.Scale + 1) % devices.ScaleCount
	case "-":
		if pat.RootNote > 0 {
			pat.RootNote--
		}
	case "+", "=":
		if pat.RootNote < 127 {
			pat.RootNote++
		}
	case "{", "n":
		if spec.EditingPatternIdx > 0 {
			spec.EditingPatternIdx--
		}
	case "}", "N":
		if spec.EditingPatternIdx < devices.NumPatterns-1 {
			spec.EditingPatternIdx++
		}
	}
}

// metropolixRender returns the TUI view for the Metropolix device.
//
// Layout:
//
//	header:  pattern X/Y (playing Z), mode, scale, root, length
//	table:   one column per stage (1..Length), rows per parameter
//	         (Pitch, Gate, Pulse, Ratch, Prob, Slide). Selected stage
//	         marked with '>' above the column.
//	footer:  key-help summary.
//
// Plain text; no styling (theme layer is a later pass).
func metropolixRender(track *model.Track, project *model.Project) string {
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

	var b strings.Builder

	playInfo := ""
	if spec.EditingPatternIdx != spec.Schedule.Playing {
		playInfo = fmt.Sprintf(" (playing %d)", spec.Schedule.Playing+1)
	}
	fmt.Fprintf(&b, "Metropolix  Pattern %d/%d%s  Mode %s  Scale %s  Root %s  Len %d  Stage %d\n\n",
		spec.EditingPatternIdx+1, devices.NumPatterns, playInfo,
		metropolixModeName(pat.Mode),
		metropolixScaleName(pat.Scale),
		metropolixPitchName(int(pat.RootNote)),
		pat.Length,
		spec.Selected+1,
	)

	// Selection-pointer row: '>' above the selected stage column.
	b.WriteString("         ")
	for i := 0; i < pat.Length; i++ {
		if i == spec.Selected {
			b.WriteString("  >  ")
		} else {
			b.WriteString("     ")
		}
	}
	b.WriteString("\n")

	// Column header: stage numbers.
	b.WriteString("Stage    ")
	for i := 0; i < pat.Length; i++ {
		fmt.Fprintf(&b, "  %d  ", i+1)
	}
	b.WriteString("\n")

	// Pitch row (scale-degree name at current Octave/Note, without
	// accumulator — the UI shows the static stage pitch, not the runtime
	// accumulated pitch).
	b.WriteString("Pitch    ")
	for i := 0; i < pat.Length; i++ {
		stage := &pat.Stages[i]
		fmt.Fprintf(&b, " %-4s", metropolixStagePitchLabel(pat, stage))
	}
	b.WriteString("\n")

	// Gate row.
	b.WriteString("Gate     ")
	for i := 0; i < pat.Length; i++ {
		ch := "."
		if pat.Stages[i].Gate {
			ch = "X"
		}
		fmt.Fprintf(&b, "  %s  ", ch)
	}
	b.WriteString("\n")

	// Pulse count.
	b.WriteString("Pulse    ")
	for i := 0; i < pat.Length; i++ {
		fmt.Fprintf(&b, "  %d  ", pat.Stages[i].PulseCount)
	}
	b.WriteString("\n")

	// Ratchets.
	b.WriteString("Ratch    ")
	for i := 0; i < pat.Length; i++ {
		fmt.Fprintf(&b, "  %d  ", pat.Stages[i].Ratchets)
	}
	b.WriteString("\n")

	// Probability (0..100 — show as 2-3 digit number).
	b.WriteString("Prob     ")
	for i := 0; i < pat.Length; i++ {
		fmt.Fprintf(&b, " %3d ", pat.Stages[i].Probability)
	}
	b.WriteString("\n")

	// Slide.
	b.WriteString("Slide    ")
	for i := 0; i < pat.Length; i++ {
		ch := "."
		if pat.Stages[i].Slide {
			ch = "~"
		}
		fmt.Fprintf(&b, "  %s  ", ch)
	}
	b.WriteString("\n")

	// Footer / key help.
	b.WriteString("\n")
	b.WriteString("  h/l select   j/k pitch   g gate   s slide   r/R ratchets   p/P prob\n")
	b.WriteString("  [/] length   </> scale   -/+ root   m mode   {/} prev/next pattern\n")

	return b.String()
}

// metropolixModeName returns the display name for a PlaybackMode, or a
// "?" sentinel if the value is out of range.
func metropolixModeName(m devices.PlaybackMode) string {
	if m < 0 || int(m) >= len(metropolixModeNames) {
		return "?"
	}
	return metropolixModeNames[m]
}

// metropolixScaleName returns the display name for a ScaleType, or a
// "?" sentinel if the value is out of range.
func metropolixScaleName(s devices.ScaleType) string {
	if s < 0 || int(s) >= len(metropolixScaleNames) {
		return "?"
	}
	return metropolixScaleNames[s]
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

// metropolixStagePitchLabel returns a compact label for a stage's pitch —
// "<scale-degree>/<octave>" e.g. "3/4". We deliberately DON'T resolve the
// full MIDI pitch here (that would require the scale-intervals table which
// lives in the controller package); this is the UI, so showing the raw
// editable fields is clearer.
func metropolixStagePitchLabel(pat *devices.MetropolixPattern, stage *devices.MetropolixStage) string {
	return fmt.Sprintf("%d/%d", stage.Note, stage.Octave)
}
