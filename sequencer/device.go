package sequencer

import (
	"go-sequence/midi"
	"go-sequence/widgets"
)

const NumPatterns = 128

// Device is a musical device that can produce MIDI events
type Device interface {
	// Called by Manager every step
	Tick(step int) []midi.Event

	// Pattern control (called by SessionDevice)
	QueuePattern(p int) (pattern, next int) // queue pattern, returns current state
	GetState() (pattern, next int)          // read state without changing
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
	Color    uint8
	Channel  uint8 // 0=static, 2=pulse
}
