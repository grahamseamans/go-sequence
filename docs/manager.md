# Manager

Orchestrates everything. Owns the clock, ticks devices, routes MIDI.

## State

```go
type Manager struct {
    session    *SessionDevice
    devices    []Device
    channelMap []uint8  // internal channel → external MIDI channel

    midiOut    *MIDIOut
    controller *Controller  // Launchpad

    step       int
    tempo      int
    playing    bool

    focused    Device  // which device gets UI/input
}
```

## Tick Loop

Called at tempo. Ticks all devices, collects MIDI, sends to hardware, updates LEDs.

```go
func (m *Manager) tick() {
    // 1. Tick all devices → collect MIDI events
    var events []MIDIEvent
    for i, dev := range m.devices {
        devEvents := dev.Tick(m.step)
        for _, e := range devEvents {
            e.Channel = uint8(i + 1)  // internal channel
            events = append(events, e)
        }
    }

    // 2. Translate internal → external channels
    for i := range events {
        events[i].Channel = m.channelMap[events[i].Channel]
    }

    // 3. Send MIDI (tight loop, all at once)
    for _, e := range events {
        m.midiOut.Send(e)
    }

    // 4. Update LEDs on focused device
    for _, led := range m.focused.RenderLEDs() {
        m.controller.SetPad(led.Row, led.Col, led.Color, led.Channel)
    }

    // 5. Advance step
    m.step = (m.step + 1) % 16
}
```

Note: Session controls devices directly via QueuePattern(), not through Manager tick loop.

## Methods

```go
func NewManager(midiOut *MIDIOut, controller *Controller) *Manager
func (m *Manager) Play()
func (m *Manager) Stop()
func (m *Manager) SetTempo(bpm int)

// Focus
func (m *Manager) GetFocused() Device
func (m *Manager) SetFocused(d Device)

// Input routing (to focused device)
func (m *Manager) HandleKey(key string)
func (m *Manager) HandlePad(row, col int)

// UI (from focused device)
func (m *Manager) View() string
```

## Channel Mapping

Internal channel = device index. Always.

```
Internal 0 = SessionDevice (no external output)
Internal 1 = Device 1 → channelMap[1] = external MIDI channel
Internal 2 = Device 2 → channelMap[2] = external MIDI channel
...
```

Session (channel 0) outputs are routed internally to devices, not to hardware.
