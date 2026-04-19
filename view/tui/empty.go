package tui

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// emptyKey handles keystrokes for a no-device track. Always a no-op.
func emptyKey(project *model.Project, out midi.ToExternal, key string) {}

// emptyRender returns empty content — nothing to draw.
func emptyRender(project *model.Project) string { return "" }
