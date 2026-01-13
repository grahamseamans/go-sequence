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
	SetLEDRGB(row, col int, rgb [3]uint8, channel uint8) error // maps RGB to palette
	ClearLEDs() error

	// Lifecycle
	Close() error
}

// Launchpad X color palette (velocity values 0-127)
// See Programmer's Reference Manual for full palette
const (
	ColorOff          uint8 = 0
	ColorDimRed       uint8 = 7
	ColorRed          uint8 = 5
	ColorBrightRed    uint8 = 72
	ColorDimGreen     uint8 = 19
	ColorGreen        uint8 = 21
	ColorBrightGreen  uint8 = 87
	ColorDimYellow    uint8 = 97
	ColorYellow       uint8 = 13
	ColorBrightYellow uint8 = 62
	ColorDimOrange    uint8 = 11
	ColorOrange       uint8 = 9
	ColorBrightOrange uint8 = 84
	ColorDimBlue      uint8 = 43
	ColorBlue         uint8 = 45
	ColorBrightBlue   uint8 = 78
	ColorCyan         uint8 = 37
	ColorPurple       uint8 = 49
	ColorPink         uint8 = 53
	ColorWhite        uint8 = 3
	ColorBrightWhite  uint8 = 119

	// Channel modes for SetLED (use as 'channel' parameter)
	ChannelStatic uint8 = 0 // solid color
	ChannelFlash  uint8 = 1 // flashing A/B alternating
	ChannelPulse  uint8 = 2 // pulsing (fades)
)
