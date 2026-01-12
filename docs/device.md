# Device Interface

All sequencer engines implement this interface.

## Interface

```go
const NumPatterns = 128

type Device interface {
    // Called by Manager every step
    Tick(step int) []MIDIEvent

    // Pattern control (called by SessionDevice)
    QueuePattern(p int) (pattern, next int)  // queue pattern, returns current state
    GetState() (pattern, next int)           // read state without changing
    ContentMask() []bool                     // which patterns have content

    // External MIDI input (keyboard for recording, etc.)
    HandleMIDI(event MIDIEvent)

    // UI - device returns render data, Manager handles output
    View() string
    RenderLEDs() []LEDMsg
    HandleKey(key string)
    HandlePad(row, col int)
}

type LEDMsg struct {
    Row, Col int
    Color    uint8
    Channel  uint8  // 0=static, 2=pulse
}
```

## Tick(step int) []MIDIEvent

Called by Manager every step.

Returns MIDI events to send. Channel is set by Manager.

Device handles pattern switching at its own loop boundary:
```go
func (d *SomeDevice) Tick(step int) []MIDIEvent {
    if step == 0 {
        d.pattern = d.next
    }
    // ... generate events for current step
}
```

## Pattern Looping

- `next` defaults to `pattern` (loops forever)
- At loop boundary, `pattern = next`
- Call `QueuePattern(n)` to change what plays next

## QueuePattern / GetState

SessionDevice uses these directly (not MIDI):
- `QueuePattern(3)` → queues pattern 3, returns `(currentPattern, 3)`
- `GetState()` → returns `(pattern, next)` for display

## HandleMIDI

For external MIDI input (keyboard recording, etc.). Not used by SessionDevice.

## UI Methods

Device renders itself:
- `View()` - TUI string
- `HandleKey(key)` - keyboard
- `HandlePad(row, col)` - Launchpad

Device holds Controller reference, updates LEDs directly.
