package midi

type EventType uint8

// MIDI message types
const (
	NoteOn    EventType = 0x90
	NoteOff   EventType = 0x80
	CC        EventType = 0xB0
	PitchBend EventType = 0xE0
	Trigger   EventType = 0xFF // Internal: manager sends NoteOn + immediate NoteOff
)

// Event represents a MIDI event in the sequencer
type Event struct {
	Tick      int64     // Absolute tick when this event should fire
	Type      EventType // NoteOn, NoteOff, CC, PitchBend
	Channel   uint8     // internal channel (device index)
	Note      uint8
	Velocity  uint8
	BendValue int16 // -8192 to +8191 for PitchBend
}
