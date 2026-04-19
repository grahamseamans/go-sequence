package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// settingsPad handles pad input on the settings view.
//
// Intentional no-op: track focus is driven by keyboard shortcuts in the
// top-level HandleKey, not by pad presses inside Settings.
func settingsPad(project *model.Project, out midi.ToExternal, focusedTrack, row, col int, down bool) {
}

// settingsLEDs returns the LED frame for the settings view. Stub.
func settingsLEDs(project *model.Project) []LED { return nil }
