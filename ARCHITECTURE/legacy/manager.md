# Manager

Orchestrates everything. Owns the clock, ticks devices, routes MIDI to multiple ports.

## State

```go
type Manager struct {
    devices  [8]Device
    session  *SessionDevice
    settings *SettingsDevice

    // Multi-port MIDI output
    defaultPort string
    senders     map[string]func(gomidi.Message) error
    sendersMu   sync.RWMutex

    controller midi.Controller  // Launchpad

    stopChan chan struct{}
    mu       sync.Mutex

    focused Device  // which device gets UI/input

    UpdateChan chan struct{}  // notify TUI of changes
}
```

## Tick Loop

Called at tempo. Ticks all devices, sends MIDI per-device to their configured port, updates LEDs.

```go
func (m *Manager) tickLoop() {
    for {
        // 1. Tick all devices → send MIDI events per-device port
        for i, dev := range m.devices {
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

            // Send events with track's channel (1-16 → 0-15 for MIDI)
            for _, e := range events {
                midiCh := ts.Channel - 1
                sender(gomidi.NoteOn(midiCh, e.Note, e.Velocity))
                // note-off scheduled async
            }
        }

        // 2. Update LEDs on focused device
        for _, led := range m.focused.RenderLEDs() {
            m.controller.SetLEDRGB(led.Row, led.Col, led.Color, led.Channel)
        }

        // 3. Notify TUI
        // 4. Advance step
        S.Step = (S.Step + 1) % 16
    }
}
```

## Multi-Port Output

Each track can route to a different MIDI port. Ports are opened lazily on first use.

```go
func (m *Manager) getSender(portName string) func(gomidi.Message) error {
    // Check cache
    // If not open, find port and open it
    // Store in m.senders map
}
```

Routing config lives in the state singleton:
- `S.Tracks[i].Channel` - MIDI channel (1-16, user-facing)
- `S.Tracks[i].PortName` - MIDI output port ("" = default)
- `S.Tracks[i].Muted` - skip this track

## Methods

```go
func NewManager() *Manager
func (m *Manager) Play()
func (m *Manager) Stop()
func (m *Manager) SetTempo(bpm int)
func (m *Manager) GetState() (step, playing, tempo)

// Device management
func (m *Manager) SetDevice(idx int, d Device)
func (m *Manager) GetDevice(idx int) Device
func (m *Manager) CreateDrumDevice(trackIdx int) Device
func (m *Manager) CreatePianoDevice(trackIdx int) Device
func (m *Manager) CreateEmptyDevice(trackIdx int) Device

// Focus
func (m *Manager) GetFocused() Device
func (m *Manager) SetFocused(d Device)
func (m *Manager) FocusSession()
func (m *Manager) FocusDevice(idx int)
func (m *Manager) FocusSettings()

// Input routing (to focused device)
func (m *Manager) HandleKey(key string)
func (m *Manager) HandlePad(row, col int)

// UI (from focused device)
func (m *Manager) View() string
```

## Architecture

```
S (singleton)         Manager              Hardware
┌──────────────┐     ┌──────────────┐
│ Tracks[0-7]  │────>│ devices[0-7] │────> Port A (IAC → Bitwig)
│  .Channel    │     │              │────> Port B (hardware synth)
│  .PortName   │     │ senders map  │────> Port C (...)
│  .Muted      │     └──────────────┘
│  .Drum/Piano │
└──────────────┘
```

Devices read their pattern data from `S.Tracks[i].Drum` or `S.Tracks[i].Piano`.
Manager reads routing config from `S.Tracks[i]` at tick time.
