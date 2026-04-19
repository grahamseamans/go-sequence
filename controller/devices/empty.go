package devices

import (
	"go-sequence/controller/surface"
	"go-sequence/midi"
	"go-sequence/model"
)

// Empty is the no-op device for tracks with Type == DeviceNone.
type Empty struct{}

// HandlePad / HandleKey / HandleMIDI — no-ops. The track has no device to drive.
func (Empty) HandlePad(project *model.Project, out midi.ToExternal, row, col int, down bool) {}
func (Empty) HandleKey(project *model.Project, out midi.ToExternal, key string)             {}
func (Empty) HandleMIDI(project *model.Project, out midi.ToExternal, ev midi.Event)         {}

// Render returns empty content — nothing to draw.
// TODO: step 5 wires view/, this may need a "no device" placeholder message.
func (Empty) Render(project *model.Project) string            { return "" }
func (Empty) RenderLEDs(project *model.Project) []surface.LED { return nil }
