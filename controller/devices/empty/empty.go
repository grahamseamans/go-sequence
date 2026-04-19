package empty

import (
	"go-sequence/controller/surface"
	"go-sequence/midi"
	"go-sequence/model"
)

// Package empty is the no-op device for tracks with Type == DeviceNone.

// HandlePad / HandleKey / HandleMIDI — no-ops. The track has no device to drive.
func HandlePad(project *model.Project, out midi.ToExternal, row, col int, down bool) {}
func HandleKey(project *model.Project, out midi.ToExternal, key string)             {}
func HandleMIDI(project *model.Project, out midi.ToExternal, ev midi.Event)         {}

// Render returns empty content — nothing to draw.
// TODO: step 5 wires view/, this may need a "no device" placeholder message.
func Render(project *model.Project) string            { return "" }
func RenderLEDs(project *model.Project) []surface.LED { return nil }
