# go-sequence Design

## What This Is

A MIDI sequencer/arranger. Not a DAW. Not a mixer. A MIDI brain.

- Input: Keyboard (vim-style) + Launchpad (pages)
- Output: MIDI to hardware via interface
- Downstream: Pd, hardware synths, whatever you want

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        Manager                          │
│  - "track 2: play pattern 3"                           │
│  - "all: play pattern 1"                               │
└─────────────────────────────────────────────────────────┘
        │           │           │           │
        v           v           v           v
┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
│ Track 1  │ │ Track 2  │ │ Track 3  │ │ Track 4  │ ... (8 tracks)
│ Device:  │ │ Device:  │ │ Device:  │ │ Device:  │
│  Drum    │ │ PianoRoll│ │Metropolix│ │  Drum    │
│ Output:  │ │ Output:  │ │ Output:  │ │ Output:  │
│ MIDI 1   │ │ MIDI 2   │ │ MIDI 3   │ │ MIDI 4   │
└──────────┘ └──────────┘ └──────────┘ └──────────┘
        │           │           │           │
        v           v           v           v
┌─────────────────────────────────────────────────────────┐
│                    MIDI Interface                        │
│              (iConnectivity mioXM/XL)                   │
└─────────────────────────────────────────────────────────┘
        │           │           │           │
        v           v           v           v
    Hardware    Hardware    Hardware      Pd
     Synth 1    Synth 2     Synth 3    (soft synths)
```

## Core Concepts

### Manager
- Controls all tracks
- Commands: "track N play pattern M" or "all play pattern M"
- Handles global transport (play/stop/tempo)
- Routes Launchpad input to correct track/page

### Track (8 total)
- Has one Device instance (can be whatever type you want)
- Has one MIDI output assignment
- Just a wrapper - device does the work

### Device (4 types for now)
- The sequencer engine
- Stores its own patterns (pattern 1, 2, 3, ...)
- Knows how to play/stop/edit
- Each type has different pattern data structure
- Each type has its own Launchpad page layout
- Each type has its own TUI view

### Device Types

#### 1. Drum (step sequencer)
- Pattern = 16 steps
- Per step: note, velocity, active, delay (positive to negative)
- What we built today

#### 2. Piano Roll
- Pattern = list of note events
- Per event: start time, duration, pitch, velocity
- Polyphonic

#### 3. Metropolix-style (do more reasearch with grok for more info later)
- Pattern = 8 stages
- Per stage: pitch, gate length, probability, ratchet, slide, accumulator
- Pitch manipulation between stages

#### 4. Melodicer (do more reasearch with grok for more info later)
- Pattern = rules/constraints
- Scale, root, density, range
- Generates notes based on rules

#### Future: Euclidean, Arp, etc.

## Launchpad Pages

## Clip Launcher View

```
        T1   T2   T3   T4   T5   T6   T7   T8
Pat 1  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]
Pat 2  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]
Pat 3  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]
Pat 4  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]  [ ]
...
```

- Tap cell = that track plays that pattern
- Tap row = all tracks play that row (scene launch)
- LED shows currently playing pattern per track

## devices

shift + pat opens that idx's device (row 1 launch + shift opens devices 1 page, also some vim style bs on the main page)
idk if we need to be able to set the device per track from launchpad, seems like that should be a computer problem to me.

each device has as much useful stuff on controllers as we can get it
at least a launchpad will be used
drums are easily covered in probalby one page with some key commands as well
but
we'll probalby need a few screen on the launchpad for the other sequencers, and idk if we're going to really be able to get anything
useful on the launchpd for the piano roll sequencer - ah we will definilty use a midi keyboard input for this as well
- maybe just like - overdub record and clear buttons on the launchpad for the pianoroll device. idk


as a stretch goal it would be really cool is you could use multiple launchpads - descyn their naviagtion from the terminal and eachother
that way multiple people could all be editing stuff at once, or you could have drums open on one and metropolix on another, would be so slick.


## TUI

Terminal = detail view
- Shows current track/device state
- Vim-style editing for notes, parameters
- Keyboard for stuff that's awkward on pads


### Pad/Key Defibutuibs

Unified pattern for defining controls:

```go
// Launchpad pads - group pads with same behavior
type PadDef struct {
    Pads    [][2]int  // list of {row, col}
    Color   string
    Tooltip string
}

// Keyboard shortcuts
type KeyDef struct {
    Keys    []string  // {"h", "l"} or {"space"}
    Desc    string
}

// Mapping between note numbers, indices, coords
type PadMap interface {
    NoteToIndex(note uint8) int
    IndexToNote(idx int) uint8
    NoteToCoord(note uint8) (row, col int)
    CoordToNote(row, col int) uint8
}
```

Each page defines its pads + keys. Component renders them.
Mini Launchpad has mouse hover for tooltips.


### Help/Legend System

Every page shows both Launchpad and keyboard controls. Self-documenting.

```
──────────────────────────────────────────
DRUM - Track 1 - Pattern 2
──────────────────────────────────────────

·C·C··C···C·····   <- the sequence

┌────────────────┐
│ █ █ · █ · · · ·│  <- mini Launchpad
│ · · · · · · · ·│     mirrors LED state
│ ...            │     mouse hover = tooltip
└────────────────┘

KEYBOARD
──────────────────────────────────────────
h/l     cursor left/right
j/k     note down/up
space   toggle step
p       play/stop
──────────────────────────────────────────
```

### Render Registration (hover tooltips)

Components register themselves during render. Hover over anything = learn what it is.

```go
// cleared at start of each render cycle
var screenMap []RenderedRegion

type RenderedRegion struct {
    Row, Col  int
    Width     int
    Tooltip   string
}

// during render, components register their regions
func (lp *LaunchpadView) Render() string {
    startRow := currentRow
    output := renderGrid()

    // register each pad
    for _, pad := range pads {
        screenMap = append(screenMap, RenderedRegion{
            Row:     startRow + pad.Row,
            Col:     pad.Col * 2,
            Width:   2,
            Tooltip: pad.Tooltip,
        })
    }
    return output
}

// on mouse move
func FindTooltip(row, col int) string {
    for _, region := range screenMap {
        if row == region.Row && col >= region.Col && col < region.Col+region.Width {
            return region.Tooltip
        }
    }
    return ""
}
```

No docs. No cheat sheets. Hover over anything to learn what it does.
Everything teaches itself.

### Multiple Controllers (stretch goal)

Each controller instance has its own page state. - navigates where it wants.... (maybe we do a split term for viewing?)
Desync navigation = multi-person jamming.

## File Structure (planned)

```
go-sequence/
├── main.go
├── manager.go         # orchestrates everything
├── track.go           # track wrapper
├── device.go          # Device interface
├── devices/
│   ├── drum.go        # step sequencer
│   ├── pianoroll.go   # note events
│   ├── metropolix.go  # stage-based
│   └── melodicer.go   # generative
├── launchpad.go       # LED output
├── launchpad_input.go # pad input
├── launchpad_pages/
│   ├── page.go        # Page interface
│   ├── clip_launcher.go
│   ├── drum_edit.go
│   ├── pianoroll_edit.go
│   └── ...
├── tui.go             # terminal UI
└── midi.go            # MIDI output handling
```

## What We Have Now

- [x] Basic drum step sequencer
- [x] Launchpad LED output
- [x] Launchpad pad input
- [x] TUI with vim-style nav
- [x] Bubbletea + gomidi stack working

## Next Steps

1. Refactor to Device interface
2. Add Manager
3. Add Track wrapper
4. Add page system for Launchpad
5. Add clip launcher page
6. Add more device types
