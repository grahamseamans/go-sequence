# go-sequence

MIDI sequencer/arranger. Not a DAW. A MIDI brain.

## Status

Basic architecture working. Two drum devices, clip launcher, Launchpad I/O.

## Feature Checklist

### Universal
- [ ] Pattern selection per device (shift + button to select which pattern you're editing)
- [ ] Device reports pattern content (empty vs has data) for clip launcher display
- [ ] Undo/redo
- [ ] Multiple Launchpads (independent navigation)

### ui tools
- [ ] Mini Launchpad in TUI
- [ ] Hover tooltips on all ui 

### Session Device (clip launcher)
- [x] Launch patterns on devices
- [x] Show playing vs queued
- [ ] Show empty vs has-content patterns
- [ ] Assign MIDI output channel per channel 
- [ ] Assign midi device per channel 
- [ ] Scene launch (whole row at once)
- [ ] Stop clip on device (just sending pattern -1)

### Drum Device
- [x] Toggle steps
- [x] Basic playback
- [ ] 1-32 steps (variable pattern length per sound) - whole hting loops on longest pattern
- [ ] 16 sounds/notes per pattern (it's a drum kit, not one sound)
- [ ] Velocity per step
- [ ] Nudge notes forward/backward (swing/delay)
- [ ] Clear pattern
- [ ] Record from MIDI input (quantize hits to steps)
- [ ] Good Launchpad layout (bottom left quarter is sound/note select, bottom right is commands like nudge left right, something you hold and hit a step to set sequence lenght, top half is steps (8x4))
- [ ] Copy/paste pattern?

### Piano Roll Device
- [ ] Record from MIDI keyboard
- [ ] Playback note events with timing
- [ ] hjkl around the notes to select and then edit notes (add/delete/start_move/lenght)
- [ ] Quantize

### Metropolix Device
- [ ] Stages with pitch, gate, probability
- [ ] Ratchets
- [ ] Slides
- [ ] Accumulators

### Transport
- [x] Play/stop
- [x] Tempo control
- [ ] Tap tempo

### MIDI
- [ ] Note off tracking (held notes)
- [ ] Multiple MIDI outputs
- [ ] Channel mapping UI

### Save/Load
- [ ] Save project to file
- [ ] Load project


## Controls

### Global
- `0` - focus session (clip launcher)
- `1-8` - focus device 1-8
- `p` - play/stop
- `+/-` - tempo

### Device (when focused)
- `h/l` - cursor left/right
- `j/k` - value down/up
- `space` - toggle

## Running

```bash
go run .
```

Select MIDI output port when prompted. Put Launchpad in Programmer Mode (hold Session + bottom Scene button).
