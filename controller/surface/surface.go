package surface

// Surface is a control surface — a grid of pads with RGB LEDs.
// Launchpad X is the reference implementation. Mock exists for tests.
type Surface interface {
	PadEvents() <-chan PadEvent
	SetLEDs(leds []LED)
	Close() error
}

type PadEvent struct {
	Row, Col int
	Down     bool // true on press, false on release
}

type LED struct {
	Row, Col int
	R, G, B  uint8
}
