package midi

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
