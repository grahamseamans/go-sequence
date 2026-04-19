# go-sequence

A free, software clone of expensive hardware MIDI sequencers (Cirklon, Hapax, Pyramid, Metropolix, Torso T-1), driven by a Novation Launchpad. Not a DAW — a MIDI brain that sits between your controller and your synths.

This document describes the target architecture. Source of truth for design decisions. Check git history for older versions.

---

## 1. The whole system, in five lines

- **Model** — state. Human patterns (what you edit), machine patterns (what plays), schedule (which patterns in what order), transport (playing/stopped/tempo).
- **Controller: devices** — pure compilers. Human pattern → machine pattern.
- **Controller: input manager** — receives events (pad/key/MIDI in). Mutates state. Calls the device compiler when a human pattern changed.
- **Controller: playback engine** — reads state, dispatches MIDI.
- **View** — renders state.

Everything else is implementation detail.

---

## 2. Architecture

```
                        ┌────────────────┐
                        │     model/     │   data only
                        │ Project, Drum, │   human patterns
                        │ Piano, ...     │   machine patterns (runtime)
                        └────────────────┘
                          ▲             ▲
                  reads   │             │ reads + mutates
                          │             │
                ┌─────────┴───┐  ┌──────┴────────────────────────┐
                │    view/    │  │         controller/           │
                │  TUI now,   │  │  playback.go (playback engine)│
                │  Wails next │  │  input.go    (input manager)  │
                │             │  │  project.go  (save/load)      │
                │             │  │  ┌──────────────────────────┐ │
                │             │  │  │ devices/  Drum, Piano,   │ │
                │             │  │  │           Metropolix,    │ │
                │             │  │  │           Session, ...   │ │
                │             │  │  └──────────────────────────┘ │
                │             │  │  ┌──────────────────────────┐ │
                │             │  │  │ surface/  Launchpad,     │ │
                │             │  │  │           Mock           │ │
                │             │  │  └──────────────────────────┘ │
                │             │  │  ┌──────────────────────────┐ │
                │             │  │  │ midi/     OS port I/O    │ │
                │             │  │  └──────────────────────────┘ │
                └─────────────┘  └───────────────────────────────┘
                       │                       │
                       │ keypress              │ pad events ↔ LED
                       ▼                       ▼ MIDI in ↔ MIDI out
                    keyboard               Launchpad, MIDI hardware
```

MIDI I/O, Control Surface, and Save/Load all live inside Controller. They're capabilities the Controller uses to do its job, not peer chunks.

---

## 3. Principles

1. **Model is pure data.** No behavior, no I/O. Just structs, JSON tags, and `Validate()` methods that clamp fields to valid ranges.

2. **Device types have no fields and no lock awareness.** `var Drum devices.Drum` is a value-singleton with only methods. The methods are of two kinds:
   - **Compile** is pure: `Compile(human *model.DrumHuman) *model.MachinePattern`. Same input → same output. No side effects.
   - **HandlePad / HandleKey / HandleMIDI** are impure: they mutate the state slice passed to them, may send preview MIDI, may call Compile. They never acquire locks themselves.

3. **InputManager owns all writer-side locking.** It's the sole writer in the system, so it owns the lock. Before calling any device handler, it acquires the appropriate per-track mutex; after the handler returns, it releases. Devices stay focused on *what* to do; InputManager controls *when it's safe to write*.

4. **Playback engine is a tape head.** Every tick: read transport → read schedule → read cursor → read current pattern's machine form → dispatch events. All reads.

5. **Input manager routes input.** Receives pad/key/MIDI events. For global commands (play, stop, tempo, save, load), mutates state directly. For device-specific inputs (step toggle, metropolix edit, session pad), routes to the focused device's `HandlePad`/`HandleKey`.

6. **Patterns store both forms.** `DrumPattern` holds a human part (persisted) and a machine part (`json:"-"`, runtime). A per-track `sync.RWMutex` guards both. Tick grabs an `Events` slice header under RLock, then walks without the lock — the slice backing array is immutable because input replaces the slice rather than mutating in place.

7. **One code path per concept.** The tick loop runs identical logic every tick regardless of whether anything just changed. Modular-wrap on the cursor handles length changes and pattern swaps for free.

8. **Naming: package path conveys role, no role suffixes.** `model.Drum` is the data, `controller/devices.Drum` is the behavior. Same domain word in both packages.

---

## 4. Data flows

### 4.1 Tick (the tape head)

```
clock advances → PlaybackEngine.tick(globalTick):
    if not state.Transport.Playing: return
    delta = globalTick - lastGlobalTick
    for each track:
        track.mu.RLock()
        sched   = track.<Kind>.Schedule
        events  = track.<Kind>.Patterns[sched.Playing].Machine.Events   // grab slice header
        plen    = patternLength(track.<Kind>.Patterns[sched.Playing])
        track.mu.RUnlock()
        cursor.Tick %= plen                                     // handles prior length changes
        newTick = (cursor.Tick + delta) % plen
        wrapped = newTick < cursor.Tick
        for each event in events:                    // walking without lock; backing array immutable
            if event.Tick in (cursor.Tick, newTick] (split if wrapped):
                if rollProbability(event.Probability):
                    midi.Send(track.Port, track.Channel, event.Event)
        cursor.Tick = newTick
        if wrapped and sched.Queued != -1:
            track.mu.Lock()                          // brief write lock for Schedule promotion
            sched.Playing = sched.Queued
            sched.Queued  = -1
            track.mu.Unlock()
```

No compilation. No regeneration. Pure walk + dispatch.

### 4.2 Pattern edit (compile-triggering input)

```
pad press → surface → InputManager.HandlePad(row, col)
  → InputManager acquires state.mu.Lock()           // writer owns the lock
  → InputManager calls focused device: devices.Drum.HandlePad(state, project, ports, row, col)
    → device mutates state.Patterns[N].Human (e.g., Notes[lane].Steps[step].Active = true)
    → device recompiles: state.Patterns[N].Machine = devices.Drum.Compile(&state.Patterns[N].Human)
    → return                                        // device never touches the lock
  → InputManager releases state.mu.Unlock()
```

PlaybackEngine reads the new machine pattern from the next tick onward (RLock, grab events slice header, RUnlock, walk).

Note: preview-only handler calls (no state mutation) still get wrapped in Lock/Unlock for uniformity. Cost is sub-microsecond. If this ever shows up as a hot path, devices can split into separate `HandlePadMutating` / `HandlePadPreview` methods so InputManager only locks the mutating path.

### 4.3 Transport / schedule change (no compile)

```
key press → InputManager.HandleKey(key)
  → InputManager recognizes "space" as play/pause → toggles state.Transport.Playing
  → return

pad press in Session view → Session.HandlePad(state, project, ports, row, col)
  → device mutates state.Tracks[row].Drum.Schedule.Queued = col
  → LED render next frame: pattern-col pad flashes (reads Schedule.Queued)
  → return
```

No compile called. PlaybackEngine reads `Schedule.Queued` and promotes it to `Schedule.Playing` at the next pattern boundary — which also causes the flashing pad to go solid.

### 4.4 Preview MIDI (no state, no compile)

```
pad press in Drum view, not recording → Drum.HandlePad(state, project, ports, row, col)
  → device checks state.Drum.Recording == false
  → device calls ports.Send(track.PortName, track.Channel, midi.Event{note, vel})
  → return
```

Direct send. Input handlers have access to `*midi.Ports` the same way the manager does. No state change, no intent machinery.

### 4.5 Save / Load

```
save:
    user presses save key in Save view
    Save.HandleKey calls controller.SaveProject(project, name)
    controller.SaveProject: bytes = json.Marshal(project); write to disk

load:
    user picks save file
    Save.HandleKey:
        state.Transport.Playing = false     // loading stops playback
        controller.LoadProject(path)
    controller.LoadProject:
        read file, unmarshal into *model.Project
        call project.Validate()
        recompile all patterns (walk, call each device's Compile for each pattern)
        return *model.Project
    InputManager replaces the shared project state. PlaybackEngine is stopped so no tick-time races; next time user hits play, it reads the new state fresh.
```

Save/load lives in `controller/project.go`, not in `model/`. Model is pure data. **Loading always stops playback** — no mid-play load.

---

## 5. Per-package reference

### 5.1 model/

Pure data. No I/O. No behavior beyond `Validate()`.

```go
package model

type Project struct {
    Tempo      int                  `json:"tempo"`
    Tracks     [8]*Track            `json:"tracks"`
    Transport  Transport            `json:"-"`  // runtime
    Tick       int64                `json:"-"`  // runtime
}

type Transport struct {
    Playing bool
    T0      time.Time
}

type Track struct {
    Name     string                 `json:"name"`
    Channel  uint8                  `json:"channel"`
    PortName string                 `json:"portName,omitempty"`
    Type     DeviceKind             `json:"type"`
    Drum       *Drum                `json:"drum,omitempty"`
    Piano      *Piano               `json:"piano,omitempty"`
    Metropolix *Metropolix          `json:"metropolix,omitempty"`
}

type Drum struct {
    mu       sync.RWMutex               `json:"-"`  // protects Patterns + Schedule writes
    Patterns [16]DrumPattern             `json:"patterns"`
    Schedule Schedule                    `json:"schedule"`
    EditingPatternIdx int                `json:"editingPattern"`
    Recording bool                       `json:"-"`
}

type DrumPattern struct {
    Human   DrumHuman       `json:"human"`
    Machine MachinePattern  `json:"-"`       // derived from Human, runtime only — replaced (not mutated) under Drum.mu
}

type DrumHuman struct {
    Notes [16]NoteLane
}

type MachinePattern struct {
    Events []TimedEvent
}

type Schedule struct {
    Playing int `json:"playing"`  // currently playing pattern (0-15)
    Queued  int `json:"queued"`   // -1 if none; pattern to promote at next boundary
}
// Future: a Chain type (named, reusable sequence of pattern indices) that
// Schedule.Playing/Queued can also reference via a small kind enum. Deferred to v2.

type TimedEvent struct {
    Tick        int64
    Event       midi.Event
    StageID     int     // for grouping (e.g., Metropolix ratchets share a probability gate)
    Probability uint8   // 0-100, dispatch rolls
}

func (p *Project) Validate()  // clamps all fields recursively
```

Piano and Metropolix follow the same shape (Human + Machine per pattern, atomic.Pointer per pattern array).

### 5.2 view/

Reads from Model (via Manager accessors), sends keyboard events to Manager. Bubbletea TUI now, Wails later. Doesn't import `model` directly.

### 5.3 controller/

#### controller/playback.go

```go
package controller

type PlaybackEngine struct {
    project *model.Project
    cursors [8]Cursor
    ports   *midi.Ports
}

type Cursor struct {
    Tick int64   // playback position within the currently-playing pattern
}

func NewPlaybackEngine(project *model.Project, ports *midi.Ports) *PlaybackEngine
func (pe *PlaybackEngine) Run(ctx context.Context)             // owns the clock + tick goroutine
func (pe *PlaybackEngine) ResetCursors()                        // called after project swap
```

Single goroutine: reads `project` state atomically, walks events, dispatches MIDI via `ports`. Writes only its own cursors + `Project.Tick`.

#### controller/input.go

```go
package controller

type InputManager struct {
    project *model.Project
    ports   *midi.Ports       // for preview MIDI in input handlers
    surface surface.Surface
    focused FocusTarget
}

type FocusTarget struct {
    Kind  FocusKind   // Track, Settings, Save, Session
    Track int         // valid when Kind == Track
}

func NewInputManager(project *model.Project, ports *midi.Ports, surface surface.Surface) *InputManager
func (im *InputManager) Run(ctx context.Context)                // listens to surface.PadEvents()
func (im *InputManager) HandleKey(key string)                   // called by view
func (im *InputManager) HandlePad(row, col int)                 // also callable directly (tests)
```

Receives input events, mutates state (directly or by routing to focused device). Writes spec fields, schedule, transport, and triggers recompilation via device `Compile`.

#### controller/project.go

```go
package controller

func SaveProject(p *model.Project, path string) error
func LoadProject(path string) (*model.Project, error)   // reads, unmarshals, Validate, recompiles all patterns
```

#### controller/devices/

Each device is an empty struct with value-receiver methods. No fields.

```go
package devices

type Drum struct{}

// pure compile
func (Drum) Compile(human *model.DrumHuman) *model.MachinePattern

// input handlers — mutate state directly; recompile by calling own Compile
func (Drum) HandlePad(state *model.Drum, project *model.Project, ports *midi.Ports, row, col int)
func (Drum) HandleKey(state *model.Drum, project *model.Project, ports *midi.Ports, key string)
func (Drum) HandleMIDI(state *model.Drum, ports *midi.Ports, ev midi.Event)

// view helpers
func (Drum) Render(state *model.Drum, project *model.Project) string
func (Drum) RenderLEDs(state *model.Drum, project *model.Project) []surface.LED
```

Same shape for `Piano`, `Metropolix`, `Session`, `Settings`, `Empty`. Session/Settings don't have `Compile` (they don't sequence patterns).

#### controller/midi/

```go
package midi

type Event struct {
    Type     EventType
    Note     uint8
    Velocity uint8
    Channel  uint8
    // ...
}

type Ports struct { ... }

func NewPorts() (*Ports, error)
func (p *Ports) Send(portName string, channel uint8, ev Event) error
func (p *Ports) Subscribe(portName string) (<-chan Event, error)
func (p *Ports) List() []PortInfo
```

#### controller/surface/

```go
package surface

type Surface interface {
    PadEvents() <-chan PadEvent
    SetLEDs(leds []LED)
    Close() error
}

type PadEvent struct {
    Row, Col int
    Down     bool
}

type LED struct {
    Row, Col int
    R, G, B  uint8  // semantic; surface maps to hardware
}

type Launchpad struct { ... }
type Mock struct { ... }
```

---

## 6. Concurrency model

Goroutines share `*model.Project`:

| Goroutine                       | Reads                                      | Writes                                           |
|---------------------------------|--------------------------------------------|--------------------------------------------------|
| PlaybackEngine.Run              | pattern.Machine (atomic), Schedule, Tracks, Transport | cursors (own), Project.Tick, Schedule.Playing/Queued at boundary |
| InputManager.Run                | surface pad events                         | patterns (atomic swap via device Compile), spec, Schedule.Queued, transport |
| InputManager.HandleKey (view)   | everything                                 | same as InputManager.Run                         |

- **Per-track `sync.RWMutex`** on each device state (Drum, Piano, Metropolix). Guards Patterns (both Human and Machine.Events) and Schedule. **All Lock/Unlock calls are made by InputManager (writer side) or PlaybackEngine (boundary promotion only).** Devices themselves are lock-unaware.
- **Tick read path**: PlaybackEngine takes RLock briefly each iteration to grab a consistent snapshot of the events slice header + schedule values; walks events without lock (slice backing array immutable — input replaces, never mutates in place).
- **Input write path**: InputManager Lock → call device handler (mutates state, may call Compile) → Unlock. Devices never touch the lock.
- **Schedule boundary promotion**: PlaybackEngine takes a brief Lock to promote `Queued → Playing`. The only place PlaybackEngine writes anything other than its own cursors. User queueing (via InputManager) takes the same Lock to set `Queued`. Whoever gets the lock first runs; user's queue is always honored on the next boundary after they set it.
- **Cursors**: owned by PlaybackEngine. Not shared, no lock needed.
- **Project.Tick, Project.Transport.Playing**: `atomic.Int64` and `atomic.Bool`.
- **Project pointer swap (load)**: only happens when Transport.Playing == false, so no concurrent tick reads. Simple pointer assignment.

---

## 7. Open questions

1. **Device interface typing** — each device has its own state type. A unified `Device` interface needs `any` + runtime assertions, or we drop the interface and use type switches in the manager. Leaning type switches.
2. **Persisting machine patterns** — currently `json:"-"`, regenerated on load. Alternative: persist them, skip regen. Default: regenerate on load.
3. **Save format compatibility** — the restructure breaks JSON tags (e.g., new `human` nesting). Pre-1.0; break is acceptable.
4. **Clock goroutine back-pressure** — channel-based tick can drop ticks. Consider a timer-in-tickLoop design instead.

---

## 8. Out of scope (future work)

- **Named pattern chains** ("meta-patterns") — reusable sequences of patterns that queue as a single unit (e.g., "Chorus" = [D, E, F]). Schedule extends cleanly to reference either a pattern or a chain via a small kind enum. Deferred to v2.
- Multi-controller support (`Surface` interface admits this).
- VST hosting (not a DAW).
- Audio output (MIDI only; route to softsynth for sound).
- MIDI clock out (eventually).
