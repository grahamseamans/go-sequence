# go-sequence Overview

MIDI sequencer/arranger. Not a DAW. A MIDI brain.

## What It Does

- Sequences patterns on multiple devices (drum, piano roll, etc.)
- Launches clips/patterns via SessionDevice
- Outputs MIDI to hardware via interface
- Controlled by keyboard (vim-style) and Launchpad

## Architecture

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

## Internal MIDI Channels

All MIDI inside the system uses internal channels = device index.

```
Channel 0 = SessionDevice
Channel 1 = Device 1
Channel 2 = Device 2
...
Channel 8 = Device 8
```

External MIDI channels are configurable per device. Translation happens once, at the very end before sending to hardware.

## Devices

Devices are self-contained sequencer engines. Each device:

- Stores its own patterns
- Handles its own playback logic
- Renders its own UI (TUI + Launchpad LEDs)
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
- Shows its View() in TUI
- Controls Launchpad LEDs

SessionDevice is focused by default (clip launcher view). Switch focus to edit a specific device.

## Files

```
go-sequence/
├── main.go
├── manager.go        # Manager, tick loop, routing
├── device.go         # Device interface
├── devices/
│   ├── session.go
│   ├── drum.go
│   ├── pianoroll.go
│   └── ...
├── controller.go     # Launchpad MIDI I/O
├── midi.go           # MIDIEvent, MIDIOut
└── tui.go            # bubbletea, routes to focused device
```
