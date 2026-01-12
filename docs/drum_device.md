# DrumDevice

Step sequencer. 16 steps per pattern.

## State

```go
type DrumDevice struct {
    controller *Controller
    patterns   []*DrumPattern
    pattern    int  // currently playing
    next       int  // queued (defaults to pattern)
    step       int  // internal step counter
}

type DrumPattern struct {
    steps  [16]Step
    length int  // 1-16, defaults to 16
}

type Step struct {
    Active   bool
    Note     uint8
    Velocity uint8
    Delay    int8  // -50 to +50, for swing
}
```

## Methods

```go
func NewDrumDevice(controller *Controller) *DrumDevice

// Device interface
func (d *DrumDevice) Tick(step int) []MIDIEvent
func (d *DrumDevice) QueuePattern(p int) (pattern, next int)
func (d *DrumDevice) GetState() (pattern, next int)
func (d *DrumDevice) HandleMIDI(event MIDIEvent)
func (d *DrumDevice) View() string
func (d *DrumDevice) HandleKey(key string)
func (d *DrumDevice) HandlePad(row, col int)
```

## Tick

```go
func (d *DrumDevice) Tick(step int) []MIDIEvent {
    // pattern switch at own loop boundary
    if d.step == 0 {
        d.pattern = d.next
    }

    pat := d.patterns[d.pattern]
    s := pat.steps[d.step]

    var events []MIDIEvent
    if s.Active {
        events = append(events, MIDIEvent{
            Type:     NoteOn,
            Note:     s.Note,
            Velocity: s.Velocity,
        })
    }

    d.step = (d.step + 1) % pat.length
    return events
}
```

## UI

Row 0: 16 steps (active = lit)
Rows 1-7: note select, velocity, etc.
