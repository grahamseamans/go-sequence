# go-sequence

A free, software clone of expensive hardware MIDI sequencers (Cirklon, Hapax, Pyramid, Metropolix, Torso T-1), driven by a Novation Launchpad. Not a DAW — a MIDI brain that sits between your controller and your synths.

This document describes the target architecture. It's the source of truth for design decisions and the reference for implementation. It is expected to evolve; check git history for older versions.

---

## 1. Architecture

go-sequence is structured as MVC, with all hardware I/O living inside Controller:

```
                        ┌────────────────┐
                        │     model/     │   data only, no behavior
                        │  Project, Drum,│   one source of truth
                        │  Piano, ...    │
                        └────────────────┘
                          ▲             ▲
                  reads   │             │ reads + mutates
                          │             │
                ┌─────────┴───┐  ┌──────┴────────────────────────┐
                │    view/    │  │         controller/           │
                │  TUI now,   │  │  manager.go (tick + dispatch) │
                │  Wails next │  │  ┌──────────────────────────┐ │
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

**MIDI I/O and Control Surface live inside Controller** because they're capabilities the Controller uses to talk to the outside world, not peer chunks.

### 1.1 Principles

1. **Model is the single source of truth.** All persisted state lives in `model/`. Save = serialize the package. Load = deserialize back. No parallel save/runtime types. Devices and views *read* from Model and (in Controller's case) *mutate* it. They never own state.

2. **Devices are stateless Controllers.** No fields, no cached pointers, no channels. Each device is a value-singleton (`var Drum devices.Drum`) whose methods take state as a parameter. This eliminates the orphaned-pointer footgun in load: swapping `*model.Project` is enough; nothing has to be re-wired.

3. **Devices return Intents, not callbacks.** When a pad press needs to trigger a MIDI preview or queue a pattern on another track, the device returns an Intent value describing what should happen. The Manager interprets and applies. Devices never call back into anything.

4. **Patterns precompute their own events.** Each pattern in Model carries both spec (notes, lengths, velocities) and rendered events (`[]TimedEvent`). Spec is the source of truth and persists; events are runtime-only (`json:"-"`) and regenerate at edit time. Tick loop never computes events — it just walks them.

5. **One code path per concept.** No "swap recovery." No "post-edit handler." The tick loop runs the same logic every tick, regardless of whether anything just changed. The cursor's modular wrap handles every "weird state" by definition.

6. **Naming: package path conveys role, no role suffixes.** `model.Drum` is the data, `controller/devices.Drum` is the behavior. Same domain word in both packages. Go-idiomatic anti-stutter.

---

## 2. Data flows

### 2.1 Tick (the heartbeat)

The tick goroutine runs every ~1ms. It is pure walk and dispatch — no event computation, no regeneration.

```
wall clock advances → Manager.tick(globalTick):
    delta = globalTick - lastGlobalTick
    for each track:
        pattern  = currentPattern(track)             // schedule[cursor.scheduleIdx], atomic load
        plen     = patternLength(pattern)
        cursor.Tick %= plen                          // handles any prior length change
        newTick  = (cursor.Tick + delta) % plen
        wrapped  = newTick < cursor.Tick
        for each event in pattern.Events:
            if event.Tick is in (cursor.Tick, newTick] (split if wrapped):
                if rollProbability(event.Probability):
                    midi.Send(track.Port, track.Channel, event.Event)
        cursor.Tick = newTick
        if wrapped:
            cursor.scheduleIdx = (cursor.scheduleIdx + 1) % len(schedule)
    surface.SetLEDBatch(devices.<focused>.RenderLEDs(state, project))
```

Key properties:
- **No regeneration in tick.** Events are precomputed by the input goroutine. Tick is read-only as far as pattern state.
- **Atomic pointer load on the pattern.** Tick reads whatever pattern pointer is current. Input may have swapped a pointer between this tick and the last; tick doesn't care.
- **Cursor stores tick, not event index.** Robust to event-list swaps and pattern length changes.
- **Modular wrap is unconditional.** No special case for "we just shrank a pattern." Every tick wraps if needed; usually it's a no-op.

### 2.2 Pad press (input → action)

```
hardware pad → surface.Launchpad reads MIDI bytes
  → translates to (row, col)
  → dispatches PadEvent to Manager
    → Manager looks up focused device + state slice
      → intents = devices.<focused>.HandlePad(state, project, row, col)
        → device may mutate spec (state.Patterns[N].Notes[...])
        → device regenerates events for any mutated pattern (atomic pointer swap)
        → device returns []Intent describing other side effects
    → Manager applies each Intent:
        PreviewNote{track, ev}     → midi.Send(port, channel, ev)
        QueuePattern{track, p}     → state.<Kind>.Schedule = append(...)
        SaveProject{name}          → model.SaveProject(project, name)
        ...
```

Key properties:
- **Devices are leaves of the call graph.** They mutate state passed in, regenerate affected pattern events, return Intents. They never call into Manager or other devices.
- **Pattern regeneration happens at edit time, not at tick time.** The device that received the input edits the spec and immediately rebuilds the events for that pattern.
- **Copy-on-write for events.** Device builds a new `DrumPattern` value, regenerates its `Events`, then `state.Patterns[N].Store(&newPattern)`. Tick goroutine sees old or new — never partial.

### 2.3 Pattern edit and regeneration

```
HandleKey/HandlePad call (input goroutine)
    ↓
device borrows current pattern pointer:    old := state.Patterns[N].Load()
device copies it:                          new := *old
device mutates spec in copy:               new.Notes[lane].Steps[step].Active = true
device regenerates events:                 new.Events = generateDrumEvents(&new)
device atomically swaps in:                state.Patterns[N].Store(&new)
old pattern is garbage-collected when no live readers hold it
```

The tick goroutine is never blocked. Worst case: it sees the old pattern for one tick after a swap, the new pattern from the next tick. ~1ms of staleness, well below human perception.

### 2.4 Save / Load

```
save (input goroutine):
    user presses save key
    SaveDevice.HandleKey returns intent: SaveProject{name}
    Manager: bytes = json.Marshal(*Manager.project); write to disk

load (input goroutine):
    user picks save file
    SaveDevice.HandleKey returns intent: LoadProject{path}
    Manager:
        project, err := model.LoadProject(path)
        project.Validate()           // clamp loaded fields to valid ranges
        Manager.project = project    // single pointer swap
        regenerate all pattern events // walk every pattern, rebuild Events
        reset all cursors             // playhead back to start of each schedule
    next tick uses new state
```

No device recreation. No `recreateDevicesFromState`. Devices have no cached pointers — swapping `*model.Project` and regenerating events is the entire load operation.

---

## 3. Per-package reference

### 3.1 model/

Pure data. No behavior beyond `Validate()` (clamps fields to valid ranges) and JSON round-trip helpers. Imports nothing except standard library and `controller/midi` (for `midi.Event`).

```go
package model

type Project struct {
    Tempo         int               `json:"tempo"`
    Tracks        [8]*Track         `json:"tracks"`
    NoteInputPort string            `json:"noteInputPort,omitempty"`
    ProjectName   string            `json:"-"`  // runtime only
    Tick          int64             `json:"-"`  // runtime: global playback tick
    Playing       bool              `json:"-"`  // runtime: playback active
    T0            time.Time         `json:"-"`  // runtime: wall-clock reference
}

type Track struct {
    Name       string                       `json:"name"`
    Channel    uint8                        `json:"channel"`
    Muted      bool                         `json:"muted"`
    Solo       bool                         `json:"solo"`
    PortName   string                       `json:"portName,omitempty"`
    Type       DeviceKind                   `json:"type"`
    Kit        string                       `json:"kit,omitempty"`
    Drum       *Drum                        `json:"drum,omitempty"`
    Piano      *Piano                       `json:"piano,omitempty"`
    Metropolix *Metropolix                  `json:"metropolix,omitempty"`
}

type Drum struct {
    Patterns          [16]atomic.Pointer[DrumPattern]  // copy-on-write
    Schedule          Schedule                          `json:"schedule"`
    PlayingPatternIdx int                               `json:"playingPattern"`
    EditingPatternIdx int                               `json:"editingPattern"`
    SelectedNoteIdx   int                               `json:"selectedNote"`
    Cursor            int                               `json:"cursor"`
    Recording         bool                              `json:"-"`
    Preview           bool                              `json:"-"`
}

type DrumPattern struct {
    Notes  [16]NoteLane    `json:"notes"`
    Events []TimedEvent    `json:"-"`     // derived from Notes, runtime only
}

// Piano, Metropolix similar shape — spec field(s) + Events field, atomic pointers in parent

type Schedule struct {
    StartTick int64 `json:"startTick"`
    Patterns  []int `json:"patterns"`     // pattern indices, in play order
}

type TimedEvent struct {
    Tick        int64       // pattern-relative tick
    Event       midi.Event  // the MIDI bytes
    StageID     int         // for grouping (Metropolix probability gates ratchets)
    Probability uint8       // 0-100, dispatch rolls
}

func LoadProject(path string) (*Project, error) { ... }
func SaveProject(p *Project, path string) error { ... }
func (p *Project) Validate() { ... }       // clamps all fields recursively
```

`atomic.Pointer[DrumPattern]` arrays don't auto-marshal — `Drum`/`Piano`/`Metropolix` need custom `MarshalJSON`/`UnmarshalJSON` that load/store the pointers.

### 3.2 view/

The user-facing rendering layer. Reads from Model (via Manager accessors), sends keyboard events to Manager. Currently Bubbletea TUI; planned to swap to Wails (Go backend + HTML/JS in system webview) without touching anything else.

```go
package view

type TUI struct {
    Manager *controller.Manager
}

func (t *TUI) View() string                    // current screen
func (t *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd)  // route input to Manager
```

View never imports `model` directly. Everything goes through `controller.Manager` accessors. This keeps the Wails migration to a single boundary.

### 3.3 controller/

#### controller/manager.go

The engine. Owns the project pointer, the tick loop, the cursors, the goroutines, the MIDI dispatch.

```go
package controller

type Manager struct {
    project *model.Project
    cursors [8]Cursor
    surface surface.Surface
    ports   *midi.Ports
    focused FocusTarget        // which device gets keyboard input
    
    // goroutine plumbing
    intents chan Intent        // from input handlers to applier
    ticks   chan int64         // from clock to tick loop
}

type Cursor struct {
    ScheduleIdx int    // which slot of Schedule we're playing
    Tick        int64  // playhead within current pattern
}

type FocusTarget struct {
    Kind  FocusKind  // Track, Settings, Save, Session
    Track int        // valid when Kind == Track
}

func New(project *model.Project, surface surface.Surface, ports *midi.Ports) *Manager
func (m *Manager) Run(ctx context.Context)               // starts goroutines
func (m *Manager) HandleKey(key string)                  // from view
func (m *Manager) HandlePad(row, col int)                // from surface
func (m *Manager) State() *model.Project                 // accessor for view
func (m *Manager) Apply(i Intent)                        // applies an intent
```

Goroutines (set up in `Run`):
- **clock**: wakes every ~1ms, sends globalTick onto `m.ticks`
- **tickLoop**: receives ticks, walks events, dispatches MIDI
- **inputApplier**: receives intents, applies them (mutates state, sends MIDI for previews)
- **midiInput**: receives MIDI from external sources, fans to focused device
- **ledRender**: throttled (60fps), reads model + focused device, calls surface.SetLEDBatch

#### controller/devices/

Device implementations. Each is an empty struct with value-receiver methods. No state.

```go
package devices

type Drum struct{}

func (Drum) Generate(pattern *model.DrumPattern) []TimedEvent  // pure function
func (Drum) HandlePad(state *model.Drum, project *model.Project, row, col int) []Intent
func (Drum) HandleKey(state *model.Drum, project *model.Project, key string) []Intent
func (Drum) HandleMIDI(state *model.Drum, ev midi.Event) []Intent  // for recording / live input
func (Drum) Render(state *model.Drum, project *model.Project) string         // TUI string
func (Drum) RenderLEDs(state *model.Drum, project *model.Project) []LEDState // for surface
```

Same shape for `Piano`, `Metropolix`, `Session`, `Settings`, `Empty`. `Session` and `Settings` don't have `Generate` (they don't sequence patterns); their queue methods are no-ops or absent.

Two interfaces, both satisfied by each device type:

```go
type Sequencer interface {
    Generate(pattern any) []model.TimedEvent  // (any here is sketchy; see open question 3)
    HandleMIDI(state any, ev midi.Event) []Intent
}

type UI interface {
    Render(state any, project *model.Project) string
    RenderLEDs(state any, project *model.Project) []LEDState
    HandleKey(state any, project *model.Project, key string) []Intent
    HandlePad(state any, project *model.Project, row, col int) []Intent
}
```

(See open question on how to handle the `any` typing in interfaces given each device has a different state type.)

#### controller/midi/

OS MIDI port management. Send/receive raw MIDI bytes via gomidi.

```go
package midi

type Event struct {
    Type     EventType  // NoteOn, NoteOff, CC, ProgramChange, ...
    Note     uint8
    Velocity uint8
    Channel  uint8      // 1-16
    // ...
}

type Ports struct { ... }

func NewPorts() (*Ports, error)
func (p *Ports) Send(portName string, ev Event) error
func (p *Ports) Subscribe(portName string) (<-chan Event, error)
func (p *Ports) ListPorts() []PortInfo
```

#### controller/surface/

Hardware control surfaces. Currently just Launchpad X. Interface lets us mock for tests.

```go
package surface

type Surface interface {
    PadEvents() <-chan PadEvent  // pad presses streamed here
    SetLEDBatch(leds []LEDState) // set all 64 pad LEDs
    Close() error
}

type PadEvent struct {
    Row, Col int
    Down     bool  // true on press, false on release
}

type LEDState struct {
    Row, Col   int
    R, G, B    uint8  // semantic RGB; surface maps to hardware
}

type Launchpad struct { ... }     // implements Surface for Novation Launchpad X
type Mock struct { ... }          // for tests; records LED batches, exposes injection of pad events
```

---

## 4. Intent vocabulary

Intents are values returned by devices that describe side effects the manager should apply. Adding a new kind of side effect = adding a new Intent type and a case in the applier.

```go
package controller

type Intent interface{ isIntent() }  // sealed interface (marker method)

// audio side effects
type PreviewNote struct {
    Track int
    Event midi.Event
}

// schedule mutations
type QueuePattern struct {
    Track   int
    Pattern int
    AtTick  int64  // 0 = next pattern boundary
}
type ClearQueue struct {
    Track int
}

// project lifecycle
type SaveProject struct { Name string }
type LoadProject struct { Path string }
type NewProject  struct { Name string }

// playback
type Play  struct{}
type Stop  struct{}
type SetTempo struct { BPM int }
```

What's NOT an Intent:
- **Focus changes** — the Manager handles these directly from key dispatch. Devices don't request focus.
- **Pattern spec edits** — devices mutate spec in place (under copy-on-write); they don't return intents for "I edited this." The atomic swap is the side effect.
- **MIDI dispatch from sequencer events** — happens in tick loop, not via intents.

---

## 5. Concurrency model

Three main goroutines share `*model.Project`:

| Goroutine     | Reads               | Writes                                                |
|---------------|---------------------|-------------------------------------------------------|
| tickLoop      | pattern.Events (atomic), Schedule, Tracks | cursors (own), Tick                                  |
| inputApplier  | everything          | spec fields, pattern.Events (atomic swap), Schedule  |
| midiInput     | focused device state| forwards events to inputApplier via intents         |

Synchronization:
- **`pattern.Events`**: atomic.Pointer[DrumPattern] etc. Lock-free reads; atomic swap on write.
- **Spec fields** (Notes, Schedule, etc.): mutated only by inputApplier. tickLoop reads these — needs `sync.RWMutex` or "read snapshot" pattern. Practically: a single `sync.RWMutex` on Project, with inputApplier taking write lock briefly per intent. Tick takes read lock per tick.
- **Cursors**: owned by Manager (in tickLoop). No shared access.
- **`Project.Tick`, `Project.Playing`**: written by tickLoop, read by everyone. `atomic.Int64` and `atomic.Bool`.

**Single-writer principle**: pattern events are written only by inputApplier, never by tickLoop. Cursor state is written only by tickLoop, never by inputApplier. Schedule is written only by inputApplier.

---

## 6. Open questions

These need decisions before implementation lands the corresponding piece.

1. **Device interface typing** — Each device has its own state type (`*model.Drum`, `*model.Piano`, ...). A unified `Sequencer`/`UI` interface needs `any` and runtime type assertions, OR we drop the unified interfaces and use type switches in the manager (manager knows which device kind goes with which state type, so it can dispatch directly). The latter is uglier-looking but type-safe. Worth deciding.

2. **Persisting the pattern's compiled `Events`** — currently spec'd as `json:"-"` so events regenerate on load. Alternative: persist them and skip regeneration on load (faster, but risks stale events if the generator algorithm ever changes). Default: `json:"-"`, regenerate on load. OK?

3. **Save format compatibility** — restructure breaks JSON tags by definition (e.g., `DrumState` → `Drum` changes nothing, but `Schedule` is new and `DrumSchedule` was never persisted). Acceptable to break? Pre-1.0, probably yes.

4. **Goroutine ownership of ticks** — clock-as-goroutine sending on `m.ticks` channel means the tick channel can drop ticks under back-pressure (bad). Alternative: clock fires a callback synchronously in the tickLoop goroutine via a timer. Worth thinking about.

5. **Wails timing** — restructure first, then Wails. Wails swap is a separate effort that touches only the View boundary.

---

## 7. Out of scope (future work)

- **Multi-controller support** — multiple Launchpads, or other surfaces (Push, MK3). The `Surface` interface admits this; no work today.
- **VST hosting** — explicitly not a DAW; out of scope.
- **Audio output** — go-sequence is MIDI only. Hook to a softsynth (FluidSynth, Bitwig, etc.) for sound.
- **Pattern chaining beyond Schedule** — Schedule is the chain mechanism. Polyrhythmic per-track schedules are inherent (each track has its own).
- **Network sync (MIDI clock out)** — eventually, not now.
