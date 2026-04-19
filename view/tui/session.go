package tui

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// sessionKey handles keyboard input on the session view.
//
// Intentional no-op until session-cursor state is added to model.UIState.
// Dispatched for symmetry.
func sessionKey(project *model.Project, out midi.ToExternal, key string) {}

// sessionRender returns the TUI view for the session device. Stub.
func sessionRender(project *model.Project) string { return "" }
