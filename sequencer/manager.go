package sequencer

import (
	"runtime"
	"sync"
	"time"

	"go-sequence/midi"

	gomidi "gitlab.com/gomidi/midi/v2"
)

// Manager orchestrates sequencer playback and device management
type Manager struct {
	devices  [8]Device
	session  *SessionDevice
	settings *SettingsDevice

	// Multi-port MIDI output
	defaultPort string
	senders     map[string]func(gomidi.Message) error
	sendersMu   sync.RWMutex

	controller midi.Controller

	stopChan chan struct{}
	mu       sync.Mutex

	focused Device // which device gets UI/input

	// Notify TUI of updates
	UpdateChan chan struct{}
}

// NewManager creates a new sequencer manager
func NewManager() *Manager {
	return &Manager{
		senders:    make(map[string]func(gomidi.Message) error),
		UpdateChan: make(chan struct{}, 1),
	}
}

// SetDevice assigns a device to a slot
func (m *Manager) SetDevice(idx int, d Device) {
	if idx >= 0 && idx < 8 {
		m.devices[idx] = d
	}
}

// GetDevice returns the device at a slot
func (m *Manager) GetDevice(idx int) Device {
	if idx >= 0 && idx < 8 {
		return m.devices[idx]
	}
	return nil
}

// Devices returns the devices array
func (m *Manager) Devices() [8]Device {
	return m.devices
}

// CreateDrumDevice creates a DrumDevice wired to the given track's state
func (m *Manager) CreateDrumDevice(trackIdx int) Device {
	if trackIdx < 0 || trackIdx >= 8 {
		return nil
	}
	ts := S.Tracks[trackIdx]
	if ts.Drum == nil {
		ts.Drum = NewDrumState()
	}
	ts.Type = DeviceTypeDrum
	ts.Piano = nil // clear other device state
	return NewDrumDevice(ts.Drum)
}

// CreatePianoDevice creates a PianoRollDevice wired to the given track's state
func (m *Manager) CreatePianoDevice(trackIdx int) Device {
	if trackIdx < 0 || trackIdx >= 8 {
		return nil
	}
	ts := S.Tracks[trackIdx]
	if ts.Piano == nil {
		ts.Piano = NewPianoState()
	}
	ts.Type = DeviceTypePiano
	ts.Drum = nil // clear other device state
	return NewPianoRollDevice(ts.Piano)
}

// CreateEmptyDevice creates an EmptyDevice for the given track
func (m *Manager) CreateEmptyDevice(trackIdx int) Device {
	if trackIdx < 0 || trackIdx >= 8 {
		return nil
	}
	ts := S.Tracks[trackIdx]
	ts.Type = DeviceTypeNone
	ts.Drum = nil
	ts.Piano = nil
	return NewEmptyDevice(trackIdx + 1)
}

// SetDefaultPort sets the default MIDI output port name
func (m *Manager) SetDefaultPort(portName string) {
	m.defaultPort = portName
}

// getSender returns a sender for the given port name, lazily opening it
func (m *Manager) getSender(portName string) func(gomidi.Message) error {
	if portName == "" {
		return nil
	}

	m.sendersMu.RLock()
	if sender, ok := m.senders[portName]; ok {
		m.sendersMu.RUnlock()
		return sender
	}
	m.sendersMu.RUnlock()

	// Open port
	m.sendersMu.Lock()
	defer m.sendersMu.Unlock()

	// Double-check after acquiring write lock
	if sender, ok := m.senders[portName]; ok {
		return sender
	}

	// Find and open port
	for _, port := range gomidi.GetOutPorts() {
		if port.String() == portName {
			sender, err := gomidi.SendTo(port)
			if err != nil {
				return nil
			}
			m.senders[portName] = sender
			return sender
		}
	}
	return nil
}

// SetController sets the MIDI controller for LED feedback
func (m *Manager) SetController(c midi.Controller) {
	m.controller = c
	if m.controller != nil && m.focused != nil {
		m.controller.ClearLEDs()
		for _, led := range m.focused.RenderLEDs() {
			m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
		}
	}
}

// SetSession sets the session device
func (m *Manager) SetSession(s *SessionDevice) {
	m.session = s
	m.focused = s // Session is focused by default
}

// SetSettings sets the settings device
func (m *Manager) SetSettings(s *SettingsDevice) {
	m.settings = s
}

// GetSettings returns the settings device
func (m *Manager) GetSettings() *SettingsDevice {
	return m.settings
}

// FocusSettings focuses the settings device
func (m *Manager) FocusSettings() {
	if m.settings != nil {
		m.SetFocused(m.settings)
	}
}

// Play starts playback
func (m *Manager) Play() {
	m.mu.Lock()
	if S.Playing {
		m.mu.Unlock()
		return
	}
	S.Playing = true
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	go m.tickLoop()
}

// Stop stops playback
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !S.Playing {
		return
	}
	S.Playing = false
	close(m.stopChan)
}

func (m *Manager) tickLoop() {
	runtime.LockOSThread() // Pin to OS thread for consistent scheduling
	defer runtime.UnlockOSThread()

	// Capture tempo at start (changes apply on next play)
	m.mu.Lock()
	tempo := S.Tempo
	m.mu.Unlock()

	// Calculate step duration (16th notes)
	stepDuration := time.Duration(float64(time.Second) * 60.0 / float64(tempo) / 4.0)

	ticker := time.NewTicker(stepDuration)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.mu.Lock()
			if !S.Playing {
				m.mu.Unlock()
				return
			}
			step := S.Step
			m.mu.Unlock()

			// 1. Tick all devices → send MIDI events per-device port
			for i, dev := range m.devices {
				if dev == nil {
					continue
				}
				ts := S.Tracks[i]
				if ts.Muted {
					continue
				}

				events := dev.Tick(step)
				if len(events) == 0 {
					continue
				}

				// Determine which port to use
				portName := ts.PortName
				if portName == "" {
					portName = m.defaultPort
				}
				sender := m.getSender(portName)
				if sender == nil {
					continue
				}

				// Send events
				for _, e := range events {
					// ts.Channel is 1-16 (user-facing), MIDI protocol uses 0-15
					midiCh := ts.Channel - 1
					switch e.Type {
					case midi.NoteOn:
						sender(gomidi.NoteOn(midiCh, e.Note, e.Velocity))
						// Schedule note off
						go func(s func(gomidi.Message) error, ch, note uint8) {
							time.Sleep(stepDuration * 80 / 100)
							s(gomidi.NoteOff(ch, note))
						}(sender, midiCh, e.Note)
					case midi.NoteOff:
						sender(gomidi.NoteOff(midiCh, e.Note))
					}
				}
			}

			// 2. Update LEDs on focused device
			if m.focused != nil && m.controller != nil {
				for _, led := range m.focused.RenderLEDs() {
					m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
				}
			}

			// 3. Notify TUI
			select {
			case m.UpdateChan <- struct{}{}:
			default:
			}

			// 4. Advance step
			m.mu.Lock()
			S.Step = (S.Step + 1) % 16
			m.mu.Unlock()
		}
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
	S.Tempo = bpm
}

// GetState returns the current sequencer state
func (m *Manager) GetState() (step int, playing bool, tempo int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return S.Step, S.Playing, S.Tempo
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
			m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
		}
	}
}

// FocusSession focuses the session device
func (m *Manager) FocusSession() {
	m.SetFocused(m.session)
}

// FocusDevice focuses a device by index
func (m *Manager) FocusDevice(idx int) {
	if idx >= 0 && idx < 8 && m.devices[idx] != nil {
		m.SetFocused(m.devices[idx])
	}
}

// Input routing (to focused device)

// HandleKey routes a key press to the focused device
func (m *Manager) HandleKey(key string) {
	if m.focused != nil {
		m.focused.HandleKey(key)
		m.notifyUpdate()
	}
}

// HandlePad routes a pad press to the focused device
func (m *Manager) HandlePad(row, col int) {
	if m.focused != nil {
		m.focused.HandlePad(row, col)
		m.notifyUpdate()
	}
}

// notifyUpdate refreshes LEDs and notifies TUI
func (m *Manager) notifyUpdate() {
	// Update LEDs
	if m.controller != nil && m.focused != nil {
		for _, led := range m.focused.RenderLEDs() {
			m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
		}
	}
	// Notify TUI
	select {
	case m.UpdateChan <- struct{}{}:
	default:
	}
}

// View returns the view of the focused device
func (m *Manager) View() string {
	if m.focused != nil {
		return m.focused.View()
	}
	return ""
}
