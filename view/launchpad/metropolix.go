package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Metropolix page constants — mirror controller/devices/metropolix.
const (
	mxPageAccumulator = 0
	mxPageProbability = 1
	mxPageGate        = 2
	mxPageRatchets    = 3
	mxPagePulseCount  = 4
	mxPageNotes       = 5
	mxPageOctave      = 6
	mxPageSettings    = 7
)

const (
	mxAccumSubValue = 0
	mxAccumSubReset = 1
	mxAccumSubMode  = 2
)

// metropolixPad dispatches a pad press/release to the page-specific handler.
// Releases (down == false) are no-ops.
//
// Ported from controller/devices/metropolix.HandlePad.
func metropolixPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix

	// Top row (row 8): accumulator sub-page navigation (only on the accum page).
	if row == 8 {
		if spec.Page == mxPageAccumulator {
			if col == 1 && spec.AccumSubPage > 0 {
				spec.AccumSubPage--
			} else if col == 2 && spec.AccumSubPage < 2 {
				spec.AccumSubPage++
			}
		}
		return
	}

	// Right column (col 8): page select (scene buttons).
	if col == 8 {
		if row >= 0 && row <= 7 {
			spec.Page = row
		}
		return
	}

	// Main grid: page-specific handler.
	pat := spec.Patterns[spec.EditingPatternIdx]
	switch spec.Page {
	case mxPageSettings:
		metropolixSettingsPad(spec, pat, row, col)
	case mxPageOctave:
		if col < pat.Length {
			pat.Stages[col].Octave = row
		}
	case mxPageNotes:
		if col < pat.Length {
			pat.Stages[col].Note = row
		}
	case mxPagePulseCount:
		if col < pat.Length {
			pat.Stages[col].PulseCount = row + 1
		}
	case mxPageRatchets:
		if col < pat.Length {
			pat.Stages[col].Ratchets = row + 1
		}
	case mxPageProbability:
		if col < pat.Length {
			p := row * 100 / 7
			if p > 100 {
				p = 100
			}
			pat.Stages[col].Probability = p
		}
	case mxPageGate:
		if col < pat.Length {
			switch {
			case row >= 2 && row <= 7:
				pat.Stages[col].GateLength = row - 2
			case row == 1:
				pat.Stages[col].Gate = !pat.Stages[col].Gate
			case row == 0:
				pat.Stages[col].Slide = !pat.Stages[col].Slide
			}
		}
	case mxPageAccumulator:
		metropolixAccumulatorPad(spec, pat, row, col)
	}
}

// metropolixLEDs returns the LED frame for the Metropolix view. Stub.
func metropolixLEDs(track *model.Track, project *model.Project) []LED { return nil }

// metropolixSettingsPad covers the PageSettings grid (mode, scale, length,
// root, slide time).
func metropolixSettingsPad(spec *devices.Metropolix, pat *devices.MetropolixPattern, row, col int) {
	switch row {
	case 7: // Mode
		if col >= 0 && col < 4 {
			pat.Mode = devices.PlaybackMode(col)
		}
	case 6: // Scale
		if col >= 0 && col < 4 {
			pat.Scale = devices.ScaleType(col)
		}
	case 5: // Length
		if col >= 0 && col < 8 {
			pat.Length = col + 1
			if spec.Selected >= pat.Length {
				spec.Selected = pat.Length - 1
			}
		}
	case 4: // Root note (within current octave)
		if col >= 0 && col < 8 {
			currentOctave := int(pat.RootNote) / 12
			root := currentOctave*12 + col
			if root < 0 {
				root = 0
			}
			if root > 127 {
				root = 127
			}
			pat.RootNote = uint8(root)
		}
	case 3: // Slide time
		if col >= 0 && col < 8 {
			pat.SlideTime = col + 1
		}
	}
}

// metropolixAccumulatorPad handles the three accumulator sub-pages: value,
// reset-count, and mode.
func metropolixAccumulatorPad(spec *devices.Metropolix, pat *devices.MetropolixPattern, row, col int) {
	if col < 0 || col >= pat.Length {
		return
	}
	stage := &pat.Stages[col]

	switch spec.AccumSubPage {
	case mxAccumSubValue:
		stage.Accumulator = row - 4
	case mxAccumSubReset:
		stage.AccumReset = row
	case mxAccumSubMode:
		if row >= 0 && row < 3 {
			stage.AccumMode = row
		}
	}
}
