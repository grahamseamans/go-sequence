# go-sequence

MIDI sequencer/arranger. Not a DAW. A MIDI brain.

## Status

Working: Drum sequencer (16 sounds, 32 steps, variable length), piano roll (note editing, playback), clip launcher, Launchpad X with LED feedback. Track-based architecture with per-track MIDI channels.

## Feature Checklist

### Universal
- [x] Pattern selection per device (`<`/`>` to switch editing pattern)
- [x] Device reports pattern content (empty vs has data) for clip launcher display
- [ ] Undo/redo
- [ ] Multiple Launchpads (independent navigation)

### UI
- [x] Mini Launchpad in TUI (with color zones)
- [ ] Pattern select on Launchpad (all devices)

### Session Device (clip launcher)
- [x] Launch patterns on devices
- [x] Show playing vs queued
- [x] Show empty vs has-content patterns
- [x] Track-based with MIDI channel per track
- [ ] UI to change track channel/device assignment
- [ ] Scene launch (whole row at once)
- [ ] Stop clip on device

### Drum Device
- [x] Toggle steps
- [x] Basic playback
- [x] 1-32 steps per track (variable length, `[`/`]` to adjust)
- [x] 16 sounds/notes per pattern (GM drum kit defaults)
- [x] Velocity per step (data exists, no per-step UI yet)
- [x] Clear track (`c`)
- [x] Launchpad layout (top: steps, bottom-left: track select, bottom-right: commands)
- [ ] Nudge notes forward/backward (data structure exists)
- [ ] Record from MIDI input
- [ ] Copy/paste pattern

### Piano Roll Device
The piano roll is for **editing notes you play in** via MIDI keyboard - not for composing from scratch. Quick fixes: nudge timing, fix wrong notes, adjust velocity/length.

- [x] Playback note events with timing and note-off
- [x] Viewport-based rendering (center follows selection)
- [x] Select notes with `hjkl`, move with `yuio` (no mode toggle)
- [x] Note length with `n`/`m`
- [x] Add/delete notes (`space`/`x`)
- [x] Pattern length (`[`/`]`)
- [x] Horizontal zoom (8 levels, `q`/`w`)
- [x] Vertical zoom (smushed/spread, `a`/`s`)
- [x] Edit sensitivity (coarse/fine, `d`/`f` horiz, `e`/`r` vert)
- [x] Overlap visualization (overlapping notes shown with `═`)
- [ ] **Record from MIDI keyboard** ← priority
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
- [x] Note-off tracking (piano roll tracks held notes)
- [x] Per-track MIDI channel output
- [ ] Multiple MIDI output ports
- [ ] Channel mapping UI

### Save/Load
- [ ] Save project to file
- [ ] Load project


## Controls

### Global
- `Q` - quit (shift+q)
- `p` - play/stop
- `+`/`-` - tempo ±5 BPM
- `0` - focus session (clip launcher)
- `1-8` - focus device by track number
- `,` - focus settings

### Drum Device
- `h`/`l` - cursor left/right
- `j`/`k` - select track up/down
- `space` - toggle step
- `[`/`]` - track length -/+
- `c` - clear track
- `<`/`>` - previous/next pattern (editing)

### Piano Roll
**Select notes**
- `hjkl` - select notes (vim movement)

**Move selected note**
- `yuio` - move note (vim movement, one row up)
- `n`/`m` - shorter/longer

**Add/delete**
- `space` - add note at view center
- `x` - delete selected note

**View**
- `q`/`w` - zoom out/in
- `a`/`s` - smushed/spread (vertical)

**Grid sensitivity**
- `d`/`f` - horizontal coarse/fine
- `e`/`r` - vertical coarse/fine

**Pattern**
- `<`/`>` - previous/next pattern (editing)
- `[`/`]` - pattern length -/+
- `c` - clear pattern

### Session
- `h`/`l` - cursor left/right (tracks)
- `j`/`k` - cursor up/down (patterns)
- `space`/`enter` - launch clip

### Settings
- `h`/`l` - move between columns
- `j`/`k` - move between tracks
- `enter` - edit selected cell
- `r` - rescan MIDI devices

## Running

```bash
go run .
```

Launchpad X: put in Programmer Mode (hold Session + bottom-right Scene button on startup).


### Pattern Chaining (future)
- [ ] Chains: sequence of patterns that play in order
- [ ] Session device sees chains, not individual patterns
- [ ] Chain of length 1 = current behavior
- [ ] Allows song arrangement without duplicating pattern data

