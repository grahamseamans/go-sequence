# SessionDevice

Clip launcher. Device 0. Controls other devices via direct method calls.

## State

```go
type SessionDevice struct {
    controller  *Controller
    devices     []Device  // references to all devices
    numPatterns int
}
```

## Methods

```go
func NewSessionDevice(controller *Controller, devices []Device, numPatterns int) *SessionDevice

// Device interface
func (s *SessionDevice) Tick(step int) []MIDIEvent  // returns nothing
func (s *SessionDevice) QueuePattern(p int) (pattern, next int)
func (s *SessionDevice) GetState() (pattern, next int)
func (s *SessionDevice) HandleMIDI(event MIDIEvent)
func (s *SessionDevice) View() string
func (s *SessionDevice) HandleKey(key string)
func (s *SessionDevice) HandlePad(row, col int)  // col=device, row=pattern
```

## Pattern Control

Session calls device methods directly:
```go
func (s *SessionDevice) HandlePad(row, col int) {
    s.devices[col].QueuePattern(row)
}

func (s *SessionDevice) View() string {
    for i, dev := range s.devices {
        pattern, next := dev.GetState()
        // render column i
    }
}
```

## UI

Grid: columns = devices, rows = patterns.

LEDs:
- Playing = green
- Queued = yellow pulse
- Empty = off
