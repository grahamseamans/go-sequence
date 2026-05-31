package midi

// MockToExternal records every Send call for test assertions.
type MockToExternal struct {
	Sends []struct {
		PortName string
		Channel  uint8
		Event    Event
	}
}

func (m *MockToExternal) Send(portName string, channel uint8, ev Event) error {
	m.Sends = append(m.Sends, struct {
		PortName string
		Channel  uint8
		Event    Event
	}{portName, channel, ev})
	return nil
}

// MockOutputLifecycle records EnsureOutput and CloseAllOutputs calls.
type MockOutputLifecycle struct {
	OpenedPorts []string
	NumClose    int
}

func (m *MockOutputLifecycle) EnsureOutput(portName string) error {
	m.OpenedPorts = append(m.OpenedPorts, portName)
	return nil
}

func (m *MockOutputLifecycle) CloseAllOutputs() {
	m.NumClose++
}

// compile-time checks
var _ ToExternal = (*MockToExternal)(nil)
var _ OutputLifecycle = (*MockOutputLifecycle)(nil)