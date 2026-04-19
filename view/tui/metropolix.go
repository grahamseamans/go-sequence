package tui

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// metropolixKey mutates the editing stage / pattern via keyboard shortcuts.
//
// Ported from controller/devices/metropolix.HandleKey.
func metropolixKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]

	// Clamp selected defensively in case Length shrunk on load.
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
	case " ":
		stage.Gate = !stage.Gate
	case "r":
		if stage.Ratchets > 1 {
			stage.Ratchets--
		}
	case "R":
		if stage.Ratchets < 8 {
			stage.Ratchets++
		}
	case "s":
		stage.Slide = !stage.Slide
	case "a":
		if stage.Accumulator > -4 {
			stage.Accumulator--
		}
	case "A":
		if stage.Accumulator < 3 {
			stage.Accumulator++
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
		pat.Mode = (pat.Mode + 1) % 4
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
		if spec.EditingPatternIdx > 0 {
			spec.EditingPatternIdx--
		}
	case ">", ".":
		if spec.EditingPatternIdx < devices.NumPatterns-1 {
			spec.EditingPatternIdx++
		}
	case "q":
		pat.Scale = (pat.Scale + 1) % devices.ScaleCount
	case "z":
		if pat.RootNote > 0 {
			pat.RootNote--
		}
	case "x":
		if pat.RootNote < 127 {
			pat.RootNote++
		}
	}
}

// metropolixRender returns the TUI view for the Metropolix device. Stub.
func metropolixRender(track *model.Track, project *model.Project) string { return "" }
