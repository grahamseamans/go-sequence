package sequencer

import (
	"sync"
	"time"

	"go-sequence/midi"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// Manager orchestrates sequencer playback and device management
type Manager struct {
	session    *SessionDevice
	devices    []Device
	channelMap []uint8 // internal channel → external MIDI channel

	outPort drivers.Out
	send    func(msg gomidi.Message) error

	controller midi.Controller

	step     int
	tempo    int
	playing  bool
	stopChan chan struct{}
	mu       sync.Mutex

	focused Device // which device gets UI/input

	// Notify TUI of updates
	UpdateChan chan struct{}
}

// NewManager creates a new sequencer manager
func NewManager() *Manager {
	m := &Manager{
		tempo:      120,
		channelMap: make([]uint8, 16),
		UpdateChan: make(chan struct{}, 1),
	}

	// Default channel map: internal = external
	for i := range m.channelMap {
		m.channelMap[i] = uint8(i)
	}

	return m
}

// SetController sets the MIDI controller for LED feedback
func (m *Manager) SetController(c midi.Controller) {
	m.controller = c
	if m.controller != nil && m.focused != nil {
		m.controller.ClearLEDs()
		for _, led := range m.focused.RenderLEDs() {
			m.controller.SetLED(led.Row, led.Col, led.Color, led.Channel)
		}
	}
}

// AddDevice adds a musical device to the sequencer
func (m *Manager) AddDevice(d Device, externalChannel uint8) {
	m.devices = append(m.devices, d)
	idx := len(m.devices)
	if idx < len(m.channelMap) {
		m.channelMap[idx] = externalChannel
	}
}

// SetSession sets the session device
func (m *Manager) SetSession(s *SessionDevice) {
	m.session = s
	m.focused = s // Session is focused by default
}

// OpenMIDI opens a MIDI output port for note output
func (m *Manager) OpenMIDI(portIndex int) error {
	outs := gomidi.GetOutPorts()
	if portIndex < 0 || portIndex >= len(outs) {
		return nil
	}
	m.outPort = outs[portIndex]
	send, err := gomidi.SendTo(m.outPort)
	if err != nil {
		return err
	}
	m.send = send
	return nil
}

// Play starts playback
func (m *Manager) Play() {
	m.mu.Lock()
	if m.playing {
		m.mu.Unlock()
		return
	}
	m.playing = true
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	go m.tickLoop()
}

// Stop stops playback
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.playing {
		return
	}
	m.playing = false
	close(m.stopChan)
}

func (m *Manager) tickLoop() {
	for {
		m.mu.Lock()
		if !m.playing {
			m.mu.Unlock()
			return
		}
		tempo := m.tempo
		step := m.step
		m.mu.Unlock()

		// Calculate step duration (16th notes)
		stepDuration := time.Duration(float64(time.Second) * 60.0 / float64(tempo) / 4.0)

		// 1. Tick all devices → collect MIDI events
		var events []midi.Event
		for i, dev := range m.devices {
			devEvents := dev.Tick(step)
			for _, e := range devEvents {
				e.Channel = uint8(i + 1) // internal channel
				events = append(events, e)
			}
		}

		// 2. Translate internal → external channels and send
		if m.send != nil {
			for _, e := range events {
				extChan := m.channelMap[e.Channel]
				switch e.Type {
				case midi.NoteOn:
					m.send(gomidi.NoteOn(extChan, e.Note, e.Velocity))
					// Schedule note off
					go func(ch, note uint8) {
						time.Sleep(stepDuration * 80 / 100)
						m.send(gomidi.NoteOff(ch, note))
					}(extChan, e.Note)
				case midi.NoteOff:
					m.send(gomidi.NoteOff(extChan, e.Note))
				}
			}
		}

		// 3. Update LEDs on focused device
		if m.focused != nil && m.controller != nil {
			for _, led := range m.focused.RenderLEDs() {
				m.controller.SetLED(led.Row, led.Col, led.Color, led.Channel)
			}
		}

		// 4. Notify TUI
		select {
		case m.UpdateChan <- struct{}{}:
		default:
		}

		// 5. Wait for next step or stop
		select {
		case <-m.stopChan:
			return
		case <-time.After(stepDuration):
		}

		// 6. Advance step
		m.mu.Lock()
		m.step = (m.step + 1) % 16
		m.mu.Unlock()
	}
}

// SetTempo sets the BPM
func (m *Manager) SetTempo(bpm int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if bpm < 20 {
		bpm = 20
	}
	if bpm > 300 {
		bpm = 300
	}
	m.tempo = bpm
}

// GetState returns the current sequencer state
func (m *Manager) GetState() (step int, playing bool, tempo int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.step, m.playing, m.tempo
}

// Focus management

// GetFocused returns the currently focused device
func (m *Manager) GetFocused() Device {
	return m.focused
}

// SetFocused sets the focused device
func (m *Manager) SetFocused(d Device) {
	m.focused = d
	if m.focused != nil && m.controller != nil {
		m.controller.ClearLEDs()
		for _, led := range m.focused.RenderLEDs() {
			m.controller.SetLED(led.Row, led.Col, led.Color, led.Channel)
		}
	}
}

// FocusSession focuses the session device
func (m *Manager) FocusSession() {
	m.SetFocused(m.session)
}

// FocusDevice focuses a device by index
func (m *Manager) FocusDevice(idx int) {
	if idx >= 0 && idx < len(m.devices) {
		m.SetFocused(m.devices[idx])
	}
}

// Input routing (to focused device)

// HandleKey routes a key press to the focused device
func (m *Manager) HandleKey(key string) {
	if m.focused != nil {
		m.focused.HandleKey(key)
	}
}

// HandlePad routes a pad press to the focused device
func (m *Manager) HandlePad(row, col int) {
	if m.focused != nil {
		m.focused.HandlePad(row, col)
	}
}

// View returns the view of the focused device
func (m *Manager) View() string {
	if m.focused != nil {
		return m.focused.View()
	}
	return ""
}

// Devices returns the list of devices
func (m *Manager) Devices() []Device {
	return m.devices
}
