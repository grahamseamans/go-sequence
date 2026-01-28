package midi

// MIDI message types
const (
	NoteOn    uint8 = 0x90
	NoteOff   uint8 = 0x80
	CC        uint8 = 0xB0
	PitchBend uint8 = 0xE0
	Trigger   uint8 = 0xFF // Internal type - manager sends NoteOn + immediate NoteOff
)

// Event represents a MIDI event in the sequencer
type Event struct {
	Tick      int64 // Absolute tick when this event should fire
	Type      uint8 // NoteOn, NoteOff, CC, PitchBend
	Channel   uint8 // internal channel (device index)
	Note      uint8
	Velocity  uint8
	BendValue int16 // -8192 to +8191 for PitchBend
}
