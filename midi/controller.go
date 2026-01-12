package midi

// ControllerType identifies the kind of controller
type ControllerType int

const (
	ControllerUnknown ControllerType = iota
	ControllerLaunchpad
	ControllerKeyboard
)

// PadEvent is sent when a pad/button is pressed on a grid controller
type PadEvent struct {
	Row, Col int
	Velocity uint8
}

// NoteEvent is sent when a note is played on a keyboard
type NoteEvent struct {
	Note     uint8
	Velocity uint8
	Channel  uint8
}

// Controller is the interface for MIDI input devices
type Controller interface {
	ID() string
	Type() ControllerType

	// Input events from the controller
	PadEvents() <-chan PadEvent   // For grid controllers (Launchpad)
	NoteEvents() <-chan NoteEvent // For keyboards

	// Output to the controller
	SetLED(row, col int, color uint8, channel uint8) error
	ClearLEDs() error

	// Lifecycle
	Close() error
}

// Launchpad color palette indices
const (
	ColorOff         uint8 = 0
	ColorRed         uint8 = 5
	ColorGreen       uint8 = 13
	ColorBrightGreen uint8 = 19
	ColorBlue        uint8 = 45
	ColorYellow      uint8 = 69
	ColorOrange      uint8 = 9
	ColorWhite       uint8 = 127
	ColorDim         uint8 = 1
)
