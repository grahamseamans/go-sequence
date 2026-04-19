package midi

type ToExternal interface {
	Send(portName string, channel uint8, ev Event) error
}

type FromExternal interface {
	Subscribe(portName string, channel uint8) (<-chan Event, error)
}
