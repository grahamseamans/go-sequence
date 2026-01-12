package main

const NumPatterns = 128

type Device interface {
	// Called by Manager every step
	Tick(step int) []MIDIEvent

	// Pattern control (called by SessionDevice)
	QueuePattern(p int) (pattern, next int) // queue pattern, returns current state
	GetState() (pattern, next int)          // read state without changing
	ContentMask() []bool                    // [NumPatterns]bool - which patterns have content

	// External MIDI input (keyboard for recording, etc.)
	HandleMIDI(event MIDIEvent)

	// UI - device renders itself
	View() string
	HandleKey(key string)
	HandlePad(row, col int)
	UpdateLEDs()
}
