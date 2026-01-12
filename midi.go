package main

const (
	NoteOn  uint8 = 0x90
	NoteOff uint8 = 0x80
	CC      uint8 = 0xB0
)

type MIDIEvent struct {
	Type     uint8 // NoteOn, NoteOff, CC
	Channel  uint8 // internal channel (device index)
	Note     uint8
	Velocity uint8
}
