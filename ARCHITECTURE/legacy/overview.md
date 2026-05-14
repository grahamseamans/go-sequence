# about docs
Docs are genrally just - types, state, func singnatures. 
Rarely you can have a little exposition
and in docs like this (overview) we just explain.

# go-sequence Overview

MIDI sequencer/arranger. Not a DAW. A MIDI brain.

## What It Does

- Sequences patterns on multiple devices (drum, piano roll, etc.)
- Launches clips/patterns via SessionDevice
- Outputs MIDI to hardware via interface
- Controlled by keyboard (vim-style) and Launchpad

## Architecture

For a deeper, code-aligned explanation of the runtime loops, tick/time model, and the device queue contract, see:

- [Architecture](architecture.md)

```
┌─────────────────────────────────────────────────────────┐
│                        Manager                          │
│  - Owns clock, ticks all devices                        │
│  - Routes MIDI (internal → external)                    │
│  - Manages focus (which device gets UI/input)           │
└─────────────────────────────────────────────────────────┘
        │
        ├── SessionDevice (clip launcher, device 0)
        │
        ├── Device 1 (e.g., DrumDevice)
        ├── Device 2 (e.g., PianoRollDevice)
        └── ...
                │
                v
        ┌───────────────┐
        │  MIDI Output  │ (translate internal → external channels)
        └───────────────┘
                │
                v
        Hardware synths, Pd, etc.
```

## MIDI Routing

Each track has its own MIDI channel and output port, configured in the Settings device (`,` key).

```
Track 1 → Channel 1 → IAC Driver (Bitwig)
Track 2 → Channel 2 → IAC Driver (Bitwig)
Track 3 → Channel 10 → Hardware Interface (drum machine)
...
```

Routing config lives in the state singleton `S.Tracks[i]`:
- `.Channel` - MIDI channel (1-16)
- `.PortName` - MIDI output port ("" = default)
- `.Muted` - skip this track

## Devices

Devices are self-contained sequencer engines. Each device:

- Stores its own patterns
- Handles its own playback logic
- Returns render data (TUI string + LED state)
- Handles its own input (keyboard + pads)
- Outputs MIDI events (with internal channel)

See individual device docs for details:
- [SessionDevice](session_device.md) - clip launcher
- [DrumDevice](drum_device.md) - step sequencer
- [PianoRollDevice](pianoroll_device.md) - note events
- etc.

## Focus

Manager tracks which device is focused. Focused device:
- Gets keyboard input
- Gets Launchpad pad input
- View() shown in TUI
- RenderLEDs() sent to Launchpad

SessionDevice is focused by default (clip launcher view). Switch focus to edit a specific device.

## Files

```
go-sequence/
├── main.go
├── sequencer/
│   ├── state.go      # State singleton (S), TrackState, DrumState, PianoState
│   ├── device.go     # Device interface
│   ├── manager.go    # Manager, tick loop, multi-port routing
│   ├── session.go    # SessionDevice (clip launcher)
│   ├── settings.go   # SettingsDevice (track config UI)
│   ├── drum.go       # DrumDevice
│   ├── pianoroll.go  # PianoRollDevice
│   └── empty.go      # EmptyDevice
├── midi/
│   ├── controller.go # Launchpad interface
│   └── launchpad.go  # Launchpad X implementation
├── tui/
│   └── model.go      # bubbletea, routes to focused device
└── config/
    └── config.go     # YAML config loading
```
