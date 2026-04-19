package launchpad

import "sync"

// Mock is a Surface implementation for tests. PushPad pushes a pad event
// synchronously (blocks until consumed) for deterministic test ordering.
// LEDFrames records every SetLEDs call so tests can assert on what was sent.
type Mock struct {
	padCh     chan PadEvent
	mu        sync.Mutex
	LEDFrames [][]LED
}

func NewMock() *Mock {
	return &Mock{padCh: make(chan PadEvent)}
}

func (m *Mock) PadEvents() <-chan PadEvent { return m.padCh }

func (m *Mock) SetLEDs(leds []LED) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy the slice so caller can mutate after return.
	cp := make([]LED, len(leds))
	copy(cp, leds)
	m.LEDFrames = append(m.LEDFrames, cp)
}

func (m *Mock) Close() error {
	close(m.padCh)
	return nil
}

// PushPad sends a pad event into PadEvents(). Synchronous — blocks until
// consumed. Use this from tests.
func (m *Mock) PushPad(ev PadEvent) { m.padCh <- ev }
