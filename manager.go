package main

import (
	"sync"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type Manager struct {
	session    *SessionDevice
	devices    []Device
	channelMap []uint8 // internal channel → external MIDI channel

	outPort drivers.Out
	send    func(msg midi.Message) error

	controller *Controller

	step     int
	tempo    int
	playing  bool
	stopChan chan struct{}
	mu       sync.Mutex

	focused Device // which device gets UI/input

	// Notify TUI of updates
	UpdateChan chan struct{}
}

func NewManager(controller *Controller) *Manager {
	m := &Manager{
		controller: controller,
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

func (m *Manager) AddDevice(d Device, externalChannel uint8) {
	m.devices = append(m.devices, d)
	idx := len(m.devices)
	if idx < len(m.channelMap) {
		m.channelMap[idx] = externalChannel
	}
}

func (m *Manager) SetSession(s *SessionDevice) {
	m.session = s
	m.focused = s // Session is focused by default
}

func (m *Manager) OpenMIDI(portIndex int) error {
	outs := midi.GetOutPorts()
	if portIndex < 0 || portIndex >= len(outs) {
		return nil
	}
	m.outPort = outs[portIndex]
	send, err := midi.SendTo(m.outPort)
	if err != nil {
		return err
	}
	m.send = send
	return nil
}

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
		var events []MIDIEvent
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
				case NoteOn:
					m.send(midi.NoteOn(extChan, e.Note, e.Velocity))
					// Schedule note off
					go func(ch, note uint8) {
						time.Sleep(stepDuration * 80 / 100)
						m.send(midi.NoteOff(ch, note))
					}(extChan, e.Note)
				case NoteOff:
					m.send(midi.NoteOff(extChan, e.Note))
				}
			}
		}

		// 3. Update LEDs on focused device
		if m.focused != nil {
			m.focused.UpdateLEDs()
		}

		// 4. Notify TUI
		select {
		case m.UpdateChan <- struct{}{}:
		default:
		}

		// 4. Wait for next step or stop
		select {
		case <-m.stopChan:
			return
		case <-time.After(stepDuration):
		}

		// 5. Advance step
		m.mu.Lock()
		m.step = (m.step + 1) % 16
		m.mu.Unlock()
	}
}

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

func (m *Manager) GetState() (step int, playing bool, tempo int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.step, m.playing, m.tempo
}

// Focus management

func (m *Manager) GetFocused() Device {
	return m.focused
}

func (m *Manager) SetFocused(d Device) {
	m.focused = d
	if m.focused != nil {
		m.controller.ClearGrid()
		m.focused.UpdateLEDs()
	}
}

func (m *Manager) FocusSession() {
	m.SetFocused(m.session)
}

func (m *Manager) FocusDevice(idx int) {
	if idx >= 0 && idx < len(m.devices) {
		m.SetFocused(m.devices[idx])
	}
}

// Input routing (to focused device)

func (m *Manager) HandleKey(key string) {
	if m.focused != nil {
		m.focused.HandleKey(key)
	}
}

func (m *Manager) HandlePad(row, col int) {
	if m.focused != nil {
		m.focused.HandlePad(row, col)
	}
}

// View (from focused device)

func (m *Manager) View() string {
	if m.focused != nil {
		return m.focused.View()
	}
	return ""
}
