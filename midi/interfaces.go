package midi

type ToExternal interface {
	Send(portName string, channel uint8, ev Event) error
}

type FromExternal interface {
	Subscribe(portName string, channel uint8) (<-chan Event, error)
}

// OutputLifecycle manages the lifecycle of MIDI output ports — open on play,
// close on stop. Ports is the sole production implementation.
type OutputLifecycle interface {
	EnsureOutput(portName string) error
	CloseAllOutputs()
}
