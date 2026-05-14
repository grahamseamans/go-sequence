package midi

import gomidi "gitlab.com/gomidi/midi/v2"

// Ports manages OS MIDI port I/O. Implements ToExternal and FromExternal.
//
// Skeleton in Step 1 — real gomidi wiring (port open/close, SysEx init,
// input demultiplexing) lands when controller/playback.go + input.go
// need it in later steps.
type Ports struct {
	// fields to be fleshed out when wiring lands
}

func NewPorts() (*Ports, error) {
	return &Ports{}, nil
}

func (p *Ports) Send(portName string, channel uint8, ev Event) error {
	return nil
}

func (p *Ports) Subscribe(portName string, channel uint8) (<-chan Event, error) {
	ch := make(chan Event)
	return ch, nil
}

func (p *Ports) Close() error {
	return nil
}

// OutputPortNames returns the OS-level MIDI output port names visible to
// gomidi right now. Used by the settings picker to populate the per-track
// output list. Re-queries on every call so newly-attached USB devices show
// up without a restart.
func OutputPortNames() []string {
	ports := gomidi.GetOutPorts()
	names := make([]string, 0, len(ports))
	for _, p := range ports {
		names = append(names, p.String())
	}
	return names
}

// InputPortNames returns the OS-level MIDI input port names visible to
// gomidi right now. Used by the settings page MIDI device list.
func InputPortNames() []string {
	ports := gomidi.GetInPorts()
	names := make([]string, 0, len(ports))
	for _, p := range ports {
		names = append(names, p.String())
	}
	return names
}
