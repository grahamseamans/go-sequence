# go-sequence

MIDI sequencer/arranger. Not a DAW. A MIDI brain.

## ☠️ DEAD PROJECT (2026-06-02)

This project is deprecated. It should have just been a few Ableton scripts this whole time.

Almost everything here — session view, scenes, clip launching, the looper, pattern state, the endless grid-orientation and save-vs-runtime-state wrangling — is stuff Ableton Live already does. Reimplementing a DAW's guts in Go to drive MIDI was the wrong altitude. The thing to build is a thin layer on top of Live (a Control Surface script, or driving Live over OSC), not a parallel DAW underneath it.

Kept around for reference only. No further development. New work happens in a fresh repo targeting Ableton.

## Status (ded)

Working: Drum sequencer (16 sounds, 32 steps, variable length), looper (record/playback MIDI input), clip launcher, Launchpad X with LED feedback. Track-based architecture with per-track MIDI channels.

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
- [x] UI to change track channel/device/output (Settings device, press `,`)
- [ ] Scene launch (whole row at once)
- [ ] Stop clip on device

### Drum Device
- [x] Toggle steps
- [x] Basic playback
- [x] 1-32 steps per track (variable length, `[`/`]` to adjust)
- [x] 16 sounds/notes per pattern
- [x] Drum kit mapping (GM, RD-8, TR-8S, ER-1) - patterns store slot indices, kit maps to MIDI notes
- [x] Velocity per step (data exists, no per-step UI yet)
- [x] Clear track (`c`) / clear pattern (`C`)
- [x] Launchpad layout (top: steps, bottom-left: track select, bottom-right: commands)
- [x] Preview mode - audition sounds from track pads
- [x] Record mode - record steps from MIDI input
- [ ] Nudge notes forward/backward (data structure exists)
- [ ] Copy/paste pattern

### Looper Device
- [x] Record MIDI input to pattern (tick-relative event capture)
- [x] Loop playback of recorded events
- [x] Per-pattern length control
- [x] 16 patterns per track
- [ ] Overdub
- [ ] Undo

### Transport
- [x] Play/stop
- [x] Tempo control
- [ ] Tap tempo

### MIDI
- [x] Note-off tracking (piano roll tracks held notes)
- [x] Per-track MIDI channel output
- [x] Multiple MIDI output ports (per-track routing in Settings)
- [x] Channel mapping UI (Settings device)

### Save/Load
- [x] Project folders with timestamped saves (`~/.config/go-sequence/projects/`)
- [x] Quick save (Shift+S)
- [x] Save device for browsing/loading (Shift+D)
- [x] Create/rename/delete projects and saves


## Controls

### Global
- `Q` - quit (Shift+Q)
- `P` - play/stop (Shift+P)
- `+`/`-` - tempo ±5 BPM
- `S` - quick save to current project (Shift+S)
- `D` - focus save device (Shift+D)
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

**Launchpad commands** (bottom-right 4x4):
- Preview toggle - audition sounds when tapping track pads
- Record toggle - write steps when tapping track pads during playback
- Clear track/pattern, length +/-

### Looper Device
- `r` - toggle record (wipes previous recording on arm)
- `c` - clear recorded events
- `<` / `>` - previous/next pattern
- `[` / `]` - shorten/lengthen loop

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

