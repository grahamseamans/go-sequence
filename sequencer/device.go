package sequencer

import (
	"go-sequence/midi"
	"go-sequence/widgets"
)

const NumPatterns = 128

// Piano roll view modes
const (
	ViewSmushed = 12 // fewer rows, notes closer together
	ViewSpread  = 24 // more rows, notes spread out
)

// DeviceType identifies what kind of sequencer device
type DeviceType string

const (
	DeviceTypeNone     DeviceType = ""
	DeviceTypeDrum     DeviceType = "Drum"
	DeviceTypePiano    DeviceType = "Piano"
	// Future: DeviceTypeMetropolix, DeviceTypeEuclidean, etc.
)

// Device is a musical device that can produce MIDI events
type Device interface {
	// Called by Manager every step
	Tick(step int) []midi.Event

	// Pattern control (called by SessionDevice)
	QueuePattern(p int) (pattern, next int) // queue pattern, returns current state
	ContentMask() []bool                    // which patterns have content

	// External MIDI input (keyboard for recording, etc.)
	HandleMIDI(event midi.Event)

	// UI - device returns render data, Manager handles output
	View() string
	RenderLEDs() []LEDState
	HandleKey(key string)
	HandlePad(row, col int)

	// Help widget layout (device-specific)
	HelpLayout() widgets.LaunchpadLayout
}

// LEDState describes the state of a single LED
type LEDState struct {
	Row, Col int
	Color    [3]uint8 // RGB color - controller maps to its palette
	Channel  uint8    // 0=static, 2=pulse
}
