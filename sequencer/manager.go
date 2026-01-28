package sequencer

import (
	"runtime"
	"sync"
	"time"

	"go-sequence/debug"
	"go-sequence/midi"

	gomidi "gitlab.com/gomidi/midi/v2"
)

// Manager orchestrates sequencer playback and device management
type Manager struct {
	devices  [8]Device
	session  *SessionDevice
	settings *SettingsDevice
	save     *SaveDevice

	// Multi-port MIDI output
	defaultPort string
	senders     map[string]func(gomidi.Message) error
	sendersMu   sync.RWMutex

	controller midi.Controller

	stopChan      chan struct{}
	interruptChan chan struct{} // signal dispatch loop to recalculate (queue changed)
	mu            sync.RWMutex  // RWMutex for concurrent reads in midiOutputLoop

	focused Device // which device gets UI/input

	// MIDI input
	midiInputChan     chan midi.NoteEvent
	midiInputStopChan chan struct{}

	// LED rendering at fixed FPS
	ledDirty    bool                // true if LEDs need refresh
	prevLEDs    map[[2]int]LEDState // for diffing
	ledStopChan chan struct{}       // stop the LED loop

	// Notify TUI of updates
	UpdateChan chan struct{}
}

// LED refresh rate
const ledFPS = 30

// NewManager creates a new sequencer manager
func NewManager() *Manager {
	m := &Manager{
		senders:     make(map[string]func(gomidi.Message) error),
		prevLEDs:    make(map[[2]int]LEDState),
		ledStopChan: make(chan struct{}),
		UpdateChan:  make(chan struct{}, 1),
	}
	return m
}

// StartRuntime starts all runtime goroutines (called once at startup)
func (m *Manager) StartRuntime() {
	// Initialize channels
	m.midiInputChan = make(chan midi.NoteEvent, 32)
	m.midiInputStopChan = make(chan struct{})
	m.stopChan = make(chan struct{})
	m.interruptChan = make(chan struct{}, 1)

	// Start all 5 goroutines
	go m.ledLoop()          // LED updates
	go m.midiInputLoop()    // MIDI keyboard input
	go m.queueManagerLoop() // Queue filling
	go m.midiOutputLoop()   // MIDI output
}

// SetDevice assigns a device to a slot and wires up callbacks
func (m *Manager) SetDevice(idx int, d Device) {
	if idx >= 0 && idx < 8 {
		m.devices[idx] = d
		m.wireDeviceCallbacks(d)
	}
}

// wireDeviceCallbacks sets up the onQueueChange callback for a device
func (m *Manager) wireDeviceCallbacks(d Device) {
	if d == nil {
		return
	}
	// Type assert to set callback - each device type has SetOnQueueChange
	switch dev := d.(type) {
	case *DrumDevice:
		dev.SetOnQueueChange(m.interrupt)
	case *PianoRollDevice:
		dev.SetOnQueueChange(m.interrupt)
	case *MetropolixDevice:
		dev.SetOnQueueChange(m.interrupt)
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
	if ts.Kit == "" {
		ts.Kit = DefaultKit
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
	ts.Metropolix = nil
	return NewEmptyDevice(trackIdx + 1)
}

// CreateMetropolixDevice creates a MetropolixDevice wired to the given track's state
func (m *Manager) CreateMetropolixDevice(trackIdx int) Device {
	if trackIdx < 0 || trackIdx >= 8 {
		return nil
	}
	ts := S.Tracks[trackIdx]
	if ts.Metropolix == nil {
		ts.Metropolix = NewMetropolixState()
	}
	ts.Type = DeviceTypeMetropolix
	ts.Drum = nil // clear other device state
	ts.Piano = nil
	return NewMetropolixDevice(ts.Metropolix)
}

// recreateDevicesFromState rebuilds all devices from the loaded state
func (m *Manager) recreateDevicesFromState() {
	for i := 0; i < 8; i++ {
		ts := S.Tracks[i]
		var dev Device
		switch ts.Type {
		case DeviceTypeDrum:
			dev = NewDrumDevice(ts.Drum)
		case DeviceTypePiano:
			dev = NewPianoRollDevice(ts.Piano)
		case DeviceTypeMetropolix:
			dev = NewMetropolixDevice(ts.Metropolix)
		default:
			dev = NewEmptyDevice(i + 1)
		}
		m.SetDevice(i, dev) // Use SetDevice to wire callbacks
	}
	// Focus session after loading
	m.SetFocused(m.session)
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
	debug.Log("ctrl", "SetController called, resetting diff state")
	m.controller = c
	if m.controller != nil && m.focused != nil {
		m.prevLEDs = make(map[[2]int]LEDState) // reset state - diff will handle clearing
		m.markLEDsDirty()
	}
}

// markLEDsDirty flags that LEDs need refresh (called from various places)
func (m *Manager) markLEDsDirty() {
	m.mu.Lock()
	m.ledDirty = true
	m.mu.Unlock()
}

// ledLoop runs at fixed FPS and flushes LED updates
func (m *Manager) ledLoop() {
	ticker := time.NewTicker(time.Second / ledFPS)
	defer ticker.Stop()

	for {
		select {
		case <-m.ledStopChan:
			return
		case <-ticker.C:
			m.mu.Lock()
			dirty := m.ledDirty
			m.ledDirty = false
			m.mu.Unlock()

			if dirty {
				m.flushLEDs()
			}
		}
	}
}

// flushLEDs sends only changed LEDs to the controller (diffing + batching)
func (m *Manager) flushLEDs() {
	if m.focused == nil || m.controller == nil {
		return
	}

	newLEDs := m.focused.RenderLEDs()
	newMap := make(map[[2]int]LEDState, len(newLEDs))

	var updates []midi.LEDUpdate

	for _, led := range newLEDs {
		key := [2]int{led.Row, led.Col}
		newMap[key] = led

		// Only send if changed
		if prev, ok := m.prevLEDs[key]; !ok || prev != led {
			updates = append(updates, midi.LEDUpdate{
				Row:     led.Row,
				Col:     led.Col,
				Color:   led.Color,
				Channel: led.Channel,
			})
		}
	}

	// Clear LEDs that are no longer present
	for key := range m.prevLEDs {
		if _, ok := newMap[key]; !ok {
			updates = append(updates, midi.LEDUpdate{
				Row:   key[0],
				Col:   key[1],
				Color: [3]uint8{0, 0, 0},
			})
		}
	}

	if len(updates) > 0 {
		debug.Log("led", "flushLEDs: batch=%d prev=%d", len(updates), len(m.prevLEDs))
		m.controller.SetLEDBatch(updates)
	}

	m.prevLEDs = newMap
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

// SetSave sets the save device
func (m *Manager) SetSave(s *SaveDevice) {
	m.save = s
}

// GetSave returns the save device
func (m *Manager) GetSave() *SaveDevice {
	return m.save
}

// FocusSave focuses the save device
func (m *Manager) FocusSave() {
	if m.save != nil {
		m.save.Refresh() // refresh project list when focusing
		m.SetFocused(m.save)
	}
}

// Look-ahead for queue filling (in ticks) - about 100ms worth at 120 BPM
const lookAheadTicks = PPQ / 2

// Play starts playback
func (m *Manager) Play() {
	m.mu.Lock()
	if S.Playing {
		m.mu.Unlock()
		return
	}

	// Initialize timing
	S.Playing = true
	S.T0 = time.Now()
	S.Tick = 0

	// Clear and initialize all device queues
	for _, dev := range m.devices {
		if dev != nil {
			dev.ClearQueue()
		}
	}
	m.mu.Unlock()

	// Goroutines already running, just signal to start filling
	m.interrupt()
}

// Stop stops playback
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !S.Playing {
		return
	}
	S.Playing = false

	// Clear all device queues
	for _, dev := range m.devices {
		if dev != nil {
			dev.ClearQueue()
		}
	}
	// Don't stop goroutines - they keep running, just no playback
}

// interrupt signals the dispatch loop to recalculate (called when queues change)
func (m *Manager) interrupt() {
	select {
	case m.interruptChan <- struct{}{}:
	default:
	}
}

// midiInputLoop consumes MIDI keyboard input and routes to devices
func (m *Manager) midiInputLoop() {
	for {
		select {
		case <-m.midiInputStopChan:
			return
		case evt := <-m.midiInputChan:
			// HandleNote does immediate echo + routes to device
			m.HandleNote(evt.Note, evt.Velocity)
		}
	}
}

// SetMIDIInput sets the MIDI keyboard input source
func (m *Manager) SetMIDIInput(ctrl midi.Controller) {
	if ctrl == nil {
		// Stop existing loop
		if m.midiInputStopChan != nil {
			close(m.midiInputStopChan)
			m.midiInputStopChan = make(chan struct{})
		}
		return
	}

	// Start consuming from controller
	go func() {
		for evt := range ctrl.NoteEvents() {
			select {
			case m.midiInputChan <- evt:
			default:
				// Drop if channel full
			}
		}
	}()
}

// fillQueues fills all device queues up to horizon
func (m *Manager) fillQueues() {
	m.mu.Lock()
	now := time.Now()
	currentTick := S.TimeToTick(now)
	targetTick := currentTick + lookAheadTicks
	m.mu.Unlock()

	// Fill all device queues
	for _, dev := range m.devices {
		if dev != nil {
			dev.FillUntil(targetTick)
		}
	}
}

// queueManagerLoop ensures device queues are filled ahead of playhead
func (m *Manager) queueManagerLoop() {
	ticker := time.NewTicker(time.Millisecond * 50) // Every 50ms
	uiTicker := time.NewTicker(time.Second / 30)    // 30 FPS
	defer ticker.Stop()
	defer uiTicker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-m.interruptChan:
			// Queue changed, recalculate immediately
			m.fillQueues()
		case <-ticker.C:
			// Periodic fill
			m.fillQueues()
		case <-uiTicker.C:
			// Update UI state
			m.mu.Lock()
			S.Tick = S.TimeToTick(time.Now())
			m.mu.Unlock()
			m.markLEDsDirty()
			select {
			case m.UpdateChan <- struct{}{}:
			default:
			}
		}
	}
}

// midiOutputLoop reads from device queues and sends MIDI messages
func (m *Manager) midiOutputLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		select {
		case <-m.stopChan:
			return
		default:
			// Find earliest event across all devices
			var nextEvent *midi.Event
			var nextDeviceIdx int = -1

			m.mu.RLock()
			for i, dev := range m.devices {
				if dev == nil {
					continue
				}
				ts := S.Tracks[i]
				if ts.Muted {
					continue
				}
				evt := dev.PeekNextEvent()
				if evt != nil && (nextEvent == nil || evt.Tick < nextEvent.Tick) {
					nextEvent = evt
					nextDeviceIdx = i
				}
			}
			m.mu.RUnlock()

			if nextEvent == nil {
				// No events, sleep briefly
				time.Sleep(time.Millisecond)
				continue
			}

			// Check if we're playing
			m.mu.RLock()
			if !S.Playing {
				m.mu.RUnlock()
				time.Sleep(time.Millisecond)
				continue
			}
			eventTime := S.TickToTime(nextEvent.Tick)
			m.mu.RUnlock()
			waitDuration := eventTime.Sub(time.Now())

			if waitDuration > 0 {
				timer := time.NewTimer(waitDuration)
				select {
				case <-m.stopChan:
					timer.Stop()
					return
				case <-timer.C:
					// Ready
				}
			}

			// Pop and send
			dev := m.devices[nextDeviceIdx]
			evt := dev.PopNextEvent()
			if evt == nil {
				continue
			}

			m.mu.RLock()
			ts := S.Tracks[nextDeviceIdx]
			m.mu.RUnlock()

			// Translate drum slot → MIDI note if needed
			if ts.Type == DeviceTypeDrum {
				kit := GetKit(ts.Kit)
				if evt.Note < 16 {
					evt.Note = kit.Notes[evt.Note]
				}
			}

			// Send MIDI
			portName := ts.PortName
			if portName == "" {
				portName = m.defaultPort
			}
			sender := m.getSender(portName)
			if sender != nil {
				midiCh := ts.Channel - 1
				switch evt.Type {
				case midi.NoteOn:
					sender(gomidi.NoteOn(midiCh, evt.Note, evt.Velocity))
				case midi.NoteOff:
					sender(gomidi.NoteOff(midiCh, evt.Note))
				case midi.Trigger:
					sender(gomidi.NoteOn(midiCh, evt.Note, evt.Velocity))
					sender(gomidi.NoteOff(midiCh, evt.Note))
				case midi.PitchBend:
					sender(gomidi.Pitchbend(midiCh, evt.BendValue))
				}
				debug.Log("dispatch", "track=%d port=%s ch=%d tick=%d type=%d note=%d", nextDeviceIdx, portName, midiCh+1, evt.Tick, evt.Type, evt.Note)
			}
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
	return S.Step(), S.Playing, S.Tempo
}

// Focus management

// GetFocused returns the currently focused device
func (m *Manager) GetFocused() Device {
	return m.focused
}

// SetFocused sets the focused device
func (m *Manager) SetFocused(d Device) {
	debug.Log("focus", "SetFocused called, resetting diff state")
	m.focused = d
	if m.focused != nil && m.controller != nil {
		m.prevLEDs = make(map[[2]int]LEDState) // reset - diff will handle clearing
		m.markLEDsDirty()
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

		// Check for preview events from DrumDevice
		m.handlePreviewEvents()

		m.notifyUpdate()
	}
}

// handlePreviewEvents drains preview channels from drum devices and sends MIDI
func (m *Manager) handlePreviewEvents() {
	for i, dev := range m.devices {
		if dev == nil {
			continue
		}
		drumDev, ok := dev.(*DrumDevice)
		if !ok {
			continue
		}

		ts := S.Tracks[i]
		kit := GetKit(ts.Kit)

		// Drain all pending preview events
		for {
			select {
			case slotIdx := <-drumDev.PreviewChan():
				if slotIdx < 0 || slotIdx >= 16 {
					continue
				}
				note := kit.Notes[slotIdx]

				// Get sender for this track's port
				portName := ts.PortName
				if portName == "" {
					portName = m.defaultPort
				}
				sender := m.getSender(portName)
				if sender == nil {
					continue
				}

				// Send note on/off
				midiCh := ts.Channel - 1
				sender(gomidi.NoteOn(midiCh, note, 100))
				go func(s func(gomidi.Message) error, ch, n uint8) {
					time.Sleep(100 * time.Millisecond)
					s(gomidi.NoteOff(ch, n))
				}(sender, midiCh, note)
			default:
				// Channel empty
				return
			}
		}
	}
}

// HandleNote handles live MIDI input: echo immediately, then record
func (m *Manager) HandleNote(note uint8, velocity uint8) {
	eventType := midi.NoteOn
	if velocity == 0 {
		eventType = midi.NoteOff
	}

	// Calculate tick from wall clock
	tick := int64(0)
	if S.Playing {
		tick = S.TimeToTick(time.Now())
	}

	// Echo immediately to MIDI out (bypass queue for low latency)
	// Find which track is focused and use its output settings
	focusedIdx := m.getFocusedTrackIdx()
	if focusedIdx >= 0 {
		ts := S.Tracks[focusedIdx]
		portName := ts.PortName
		if portName == "" {
			portName = m.defaultPort
		}
		sender := m.getSender(portName)
		if sender != nil {
			midiCh := ts.Channel - 1
			if eventType == midi.NoteOn {
				sender(gomidi.NoteOn(midiCh, note, velocity))
			} else {
				sender(gomidi.NoteOff(midiCh, note))
			}
		}
	}

	// Send to device for recording (with tick)
	if m.focused != nil {
		m.focused.HandleMIDI(midi.Event{
			Tick:     tick,
			Type:     eventType,
			Note:     note,
			Velocity: velocity,
		})
		m.notifyUpdate()
	}
}

// getFocusedTrackIdx returns the track index of the focused device (-1 if none)
func (m *Manager) getFocusedTrackIdx() int {
	for i, dev := range m.devices {
		if dev == m.focused {
			return i
		}
	}
	return -1
}

// notifyUpdate refreshes LEDs and notifies TUI
func (m *Manager) notifyUpdate() {
	// Update LEDs (diffed)
	m.markLEDsDirty()
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

// ToggleRecording toggles recording on the focused device
func (m *Manager) ToggleRecording() {
	if m.focused != nil {
		m.focused.ToggleRecording()
		m.notifyUpdate()
	}
}

// TogglePreview toggles preview/thru on the focused device
func (m *Manager) TogglePreview() {
	if m.focused != nil {
		m.focused.TogglePreview()
		m.notifyUpdate()
	}
}
