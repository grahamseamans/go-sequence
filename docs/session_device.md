# SessionDevice

Clip launcher. Device 0. Controls other devices via direct method calls.

## State

```go
type SessionDevice struct {
    controller *Controller
    devices    []Device
    cursorRow  int
    cursorCol  int
    viewRows   int  // visible rows (default 8)
    viewOffset int  // scroll position
}
```

## Methods

```go
func NewSessionDevice(controller *Controller, devices []Device) *SessionDevice
```

## Pattern Control

Session calls device methods directly:
```go
func (s *SessionDevice) HandlePad(row, col int) {
    patternRow := s.viewOffset + (7 - row)
    s.devices[col].QueuePattern(patternRow)
}

func (s *SessionDevice) View() string {
    for i, dev := range s.devices {
        pattern, next := dev.GetState()
        mask := dev.ContentMask()
        // render column i with content info
    }
}
```

## UI

Grid: columns = devices, rows = patterns (scrollable, 128 total).

LEDs:
- Playing = green
- Queued = yellow pulse
- Has content = dim
- Empty = off
