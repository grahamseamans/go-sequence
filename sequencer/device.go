package sequencer

import (
	"go-sequence/midi"
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
	DeviceTypeNone       DeviceType = ""
	DeviceTypeDrum       DeviceType = "Drum"
	DeviceTypePiano      DeviceType = "Piano"
	DeviceTypeMetropolix DeviceType = "Metropolix"
)

// Device is a musical device that can produce MIDI events
type Device interface {
	// Queue-based playback
	// Devices maintain their own event queue. Manager calls FillUntil to ensure
	// the queue has events up to a certain tick, then peeks/pops to dispatch.
	FillUntil(tick int64)        // Fill queue with events up to tick
	PeekNextEvent() *midi.Event  // Get next event without removing (nil if empty)
	PopNextEvent() *midi.Event   // Remove and return next event (nil if empty)
	ClearQueue()                 // Clear all queued events (for stop/restart)

	// Pattern control - Ableton-style quantized switching
	QueuePattern(p int, atTick int64) // Queue pattern switch at boundary after atTick
	CurrentPattern() int              // Currently playing pattern
	NextPattern() int                 // Queued pattern (-1 if none)
	ContentMask() []bool              // Which patterns have content

	// Live input (bypasses queue - immediate echo + record)
	HandleMIDI(event midi.Event)

	// Recording control
	ToggleRecording()
	TogglePreview()
	IsRecording() bool
	IsPreviewing() bool

	// UI - device returns render data, Manager handles output
	View() string
	RenderLEDs() []LEDState
	HandleKey(key string)
	HandlePad(row, col int)
}

// LEDState describes the state of a single LED
type LEDState struct {
	Row, Col int
	Color    [3]uint8 // RGB color - controller maps to its palette
	Channel  uint8    // 0=static, 2=pulse
}
