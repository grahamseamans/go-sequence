package sequencer

import "go-sequence/midi"

// Track represents a column in the session view / MIDI output channel slot.
// A Track can be empty (Device == nil) or have a sequencer device assigned.
type Track struct {
	Name    string
	Channel uint8  // MIDI output channel (1-16)
	Device  Device // nil = empty track
	Muted   bool
	Solo    bool
}

// NewTrack creates a new empty track with the given name and MIDI channel.
func NewTrack(name string, channel uint8) *Track {
	return &Track{
		Name:    name,
		Channel: channel,
	}
}

// HasDevice returns true if this track has a device assigned.
func (t *Track) HasDevice() bool {
	return t.Device != nil
}

// Tick forwards to the device if present, respects mute.
func (t *Track) Tick(step int) []midi.Event {
	if t.Device == nil || t.Muted {
		return nil
	}
	return t.Device.Tick(step)
}

// QueuePattern forwards to the device if present.
func (t *Track) QueuePattern(p int) (pattern, next int) {
	if t.Device == nil {
		return 0, 0
	}
	return t.Device.QueuePattern(p)
}

// GetState forwards to the device if present.
func (t *Track) GetState() (pattern, next int) {
	if t.Device == nil {
		return 0, 0
	}
	return t.Device.GetState()
}

// ContentMask forwards to the device if present.
func (t *Track) ContentMask() []bool {
	if t.Device == nil {
		return make([]bool, NumPatterns)
	}
	return t.Device.ContentMask()
}
