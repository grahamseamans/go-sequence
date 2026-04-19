package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// emptyPad handles pad events on the no-device track. Always a no-op.
func emptyPad(project *model.Project, out midi.ToExternal, row, col int, down bool) {}

// emptyLEDs returns no LEDs — nothing to render for an empty track.
func emptyLEDs(project *model.Project) []LED { return nil }
