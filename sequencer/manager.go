package sequencer

import (
	"runtime"
	"sync"
	"time"

	"go-sequence/midi"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// Manager orchestrates sequencer playback and device management
type Manager struct {
	session  *SessionDevice
	settings *SettingsDevice
	tracks   []*Track // tracks own devices and MIDI channel assignments

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
	return &Manager{
		tempo:      120,
		tracks:     make([]*Track, 0),
		UpdateChan: make(chan struct{}, 1),
	}
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

// AddTrack creates a new track with the given name and MIDI channel.
func (m *Manager) AddTrack(name string, channel uint8) *Track {
	t := NewTrack(name, channel)
	m.tracks = append(m.tracks, t)
	return t
}

// SetTrackDevice assigns a device to a track by index.
func (m *Manager) SetTrackDevice(idx int, d Device) {
	if idx >= 0 && idx < len(m.tracks) {
		m.tracks[idx].Device = d
	}
}

// Tracks returns the list of tracks.
func (m *Manager) Tracks() []*Track {
	return m.tracks
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
	runtime.LockOSThread() // Pin to OS thread for consistent scheduling
	defer runtime.UnlockOSThread()

	// Capture tempo at start (changes apply on next play)
	m.mu.Lock()
	tempo := m.tempo
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
			if !m.playing {
				m.mu.Unlock()
				return
			}
			step := m.step
			m.mu.Unlock()

			// 1. Tick all tracks → collect MIDI events with track's channel
			var events []midi.Event
			for _, track := range m.tracks {
				if track.Device == nil || track.Muted {
					continue
				}
				devEvents := track.Device.Tick(step)
				for _, e := range devEvents {
					e.Channel = track.Channel // use track's MIDI channel
					events = append(events, e)
				}
			}

			// 2. Send MIDI events (channel already set from track)
			if m.send != nil {
				for _, e := range events {
					switch e.Type {
					case midi.NoteOn:
						m.send(gomidi.NoteOn(e.Channel, e.Note, e.Velocity))
						// Schedule note off
						go func(ch, note uint8) {
							time.Sleep(stepDuration * 80 / 100)
							m.send(gomidi.NoteOff(ch, note))
						}(e.Channel, e.Note)
					case midi.NoteOff:
						m.send(gomidi.NoteOff(e.Channel, e.Note))
					}
				}
			}

			// 3. Update LEDs on focused device
			if m.focused != nil && m.controller != nil {
				for _, led := range m.focused.RenderLEDs() {
					m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
				}
			}

			// 4. Notify TUI
			select {
			case m.UpdateChan <- struct{}{}:
			default:
			}

			// 5. Advance step
			m.mu.Lock()
			m.step = (m.step + 1) % 16
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
			m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
		}
	}
}

// FocusSession focuses the session device
func (m *Manager) FocusSession() {
	m.SetFocused(m.session)
}

// FocusDevice focuses a device by track index (works for any device including empty)
func (m *Manager) FocusDevice(idx int) {
	if idx >= 0 && idx < len(m.tracks) && m.tracks[idx].Device != nil {
		m.SetFocused(m.tracks[idx].Device)
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

// Devices returns all non-nil devices from tracks (for backward compatibility)
func (m *Manager) Devices() []Device {
	var devices []Device
	for _, t := range m.tracks {
		if t.Device != nil {
			devices = append(devices, t.Device)
		}
	}
	return devices
}
