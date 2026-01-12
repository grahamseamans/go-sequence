# DrumDevice

16-track drum machine. Each track is a different sound (kick, snare, hat, etc.) with up to 32 steps.

## State

```go
type DrumStep struct {
    Active   bool
    Velocity uint8
    Nudge    int8   // -64 to +63, timing offset per step
}

type DrumTrack struct {
    Steps  [32]DrumStep
    Length int    // 1-32, defaults to 16
    Note   uint8  // MIDI note for this sound (36=kick, 38=snare, etc.)
}

type DrumPattern struct {
    Tracks [16]DrumTrack
}

type DrumDevice struct {
    patterns []*DrumPattern
    pattern  int  // currently playing
    next     int  // queued (defaults to pattern)
    step     int  // global step counter
    selected int  // which track (0-15) we're editing
}
```

## Methods

```go
func NewDrumDevice() *DrumDevice
func (p *DrumPattern) MasterLength() int  // max of all track lengths
```

## Tick

Pattern loops at `MasterLength()`. Each track only plays up to its own `Length`.

```go
func (d *DrumDevice) Tick(step int) []MIDIEvent {
    // pattern switch at own loop boundary
    if d.step == 0 {
        d.pattern = d.next
    }

    pat := d.patterns[d.pattern]
    masterLen := pat.MasterLength()

    var events []MIDIEvent
    for i := 0; i < 16; i++ {
        track := &pat.Tracks[i]
        if d.step < track.Length {
            s := track.Steps[d.step]
            if s.Active {
                events = append(events, MIDIEvent{
                    Type:     NoteOn,
                    Note:     track.Note,
                    Velocity: s.Velocity,
                    // TODO: apply s.Nudge to timing
                })
            }
        }
    }

    d.step = (d.step + 1) % masterLen
    return events
}
```

## Launchpad Layout (8x8)

```
+---+---+---+---+---+---+---+---+
| rows 4-7: Steps for selected  |
| track (8x4 = 32 steps)        |
|   row 7: steps 0-7            |
|   row 6: steps 8-15           |
|   row 5: steps 16-23          |
|   row 4: steps 24-31          |
+---+---+---+---+---+---+---+---+
| rows 0-3, cols 0-3:           | rows 0-3, cols 4-7:
| Track/Sound Select (16 pads)  | Commands
|   0-3, 4-7, 8-11, 12-15       |   clear, nudge L/R, length...
+---+---+---+---+---+---+---+---+
```

Track select: bottom-left 4x4 (rows 0-3, cols 0-3)
- Lit = has content, bright = selected
- Press to select which track you're editing

Commands: bottom-right 4x4 (rows 0-3, cols 4-7)
- Clear track
- Nudge left/right
- Hold + tap step = set length

Steps: top 4 rows (rows 4-7)
- Shows steps 0-31 for currently selected track
- Active steps lit green
- Playhead yellow pulse
- Steps beyond track.Length dimmed

## TUI View

Shows selected track's steps, current pattern/step, all track activity.
