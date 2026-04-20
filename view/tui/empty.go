package tui

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// emptyKey handles keystrokes for a no-device track. Always a no-op —
// there's nothing to edit until the user assigns a device from the Settings
// page.
func emptyKey(project *model.Project, out midi.ToExternal, key string) {}

// emptyRender is shown when the focused track has no device assigned. Hints
// at the Settings keybind so the user can configure the track.
func emptyRender(project *model.Project) string {
	return "(no device assigned — press ',' to open Settings)\n"
}
