package launchpad

import (
	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/system"
)

// projectPad handles pad input on the project (save/load) browser view.
//
// Ported from controller/devices/save.HandlePad. The old browser drove a
// pad grid for project/save selection; with the simplified flat "list of
// saves" model that's redundant with the keyboard cursor. Left as a no-op
// and will be revisited if a touch-first UX is needed.
func projectPad(sp *system.Project, project *model.Project, ops save.SaveOps, out midi.ToExternal, row, col int, down bool) {
	// Intentional no-op.
}

// projectLEDs returns the LED frame for the project browser view. Stub.
func projectLEDs(sp *system.Project, project *model.Project, ops save.SaveOps) []LED { return nil }
