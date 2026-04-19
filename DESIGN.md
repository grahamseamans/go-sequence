# go-sequence

A free, software clone of expensive hardware MIDI sequencers (Cirklon, Hapax, Pyramid, Metropolix, Torso T-1), driven by a Novation Launchpad. Not a DAW — a MIDI brain that sits between your controller and your synths.

This document describes the target architecture. Source of truth for design decisions. Check git history for older versions.

---

## 1. The whole system

Three layers, organized by domain within each:

- **`model/`** — state. Pure data types. No behavior beyond `Validate()`.
- **`controller/`** — functions that operate on state. No types holding state, just functions.
- **`view/`** — renders state, captures user input gestures (pad presses, keys), calls controller functions in response.

Per-domain implementation is split across all three layers at the same path. Example:

```
model/devices/drum.go         — drum state struct
controller/devices/drum.go    — functions on drum state (Compile, ToggleStep, ...)
view/devices/drum.go          — drum UI: render the grid, capture pad presses
```

The "drum device" is a real conceptual entity (the user thinks about it that way), but it is **NOT a Go type**. It's a domain that has files in each of the three layers, all at parallel paths. Same shape for every domain.

Subdirs within each layer (`devices/`, `system/`) are organizational groupings for navigability — not semantic categories. They're folders, not types.

Beyond MVC: there's also `midi/` (data types + OS port I/O, lives at the repo root because it's a cross-cutting primitive used by everything).

---

## 2. Architecture

```
                                model/                  ← state types
                                ├── project.go          ← Project struct
                                ├── devices/            ← per-musical-domain state
                                │   ├── drum.go
                                │   ├── piano.go
                                │   └── metropolix.go
                                └── system/             ← per-system-domain state
                                    ├── project.go      ← project browser state
                                    ├── settings.go
                                    ├── session.go
                                    └── empty.go

  view/                                                                          controller/      ← functions on state
  ├── devices/                          reads model state                        ├── devices/
  │   ├── drum.go     ─── render + ─→  ←─ calls ────────────────────────────→   │   ├── drum.go     ← Compile, ToggleStep, ApplyMIDI, ...
  │   ├── piano.go        capture pads                                            │   ├── piano.go
  │   └── metropolix.go                                                           │   └── metropolix.go
  └── system/                                                                     ├── system/
      ├── project.go  ─── render + ─→                                             │   ├── project.go  ← SaveProject, LoadProject, ListSaves, ...
      ├── settings.go     capture pads                                            │   ├── settings.go
      ├── session.go                                                              │   ├── session.go
      └── empty.go                                                                │   └── empty.go
                                                                                  ├── playback.go     ← PlaybackEngine — tick loop, double-buffered events
                                                                                  ├── input.go        ← InputManager — dispatch, owns writes
                                                                                  └── compile.go      ← Compiler — background goroutine

                                midi/                   ← data types + OS port I/O
                                ├── event.go            ← Event, EventType
                                ├── interfaces.go       ← ToExternal, FromExternal
                                └── ports.go            ← Ports struct, Send/Subscribe
```

`midi/` lives at root because Model needs `midi.Event` for type definitions and Controller needs both types + I/O. It's a cross-cutting primitive.

---

## 3. Principles

1. **Domain entities are conceptual; implementation is split across three layers at parallel paths.** "Drum device" has files in `model/devices/drum.go`, `controller/devices/drum.go`, `view/devices/drum.go`. It is NOT a Go type bundling all three.

2. **Model owns ALL state.** Persisted (Project, Drum, etc.) and runtime (UI focus, playback cursors, compile channels, edit counters, transport flags). Runtime fields are marked `json:"-"`. The distinction "persisted vs runtime" is just a JSON tag, not a structural separation.

3. **Controller is bags of functions, no types.** Every controller file holds free functions that operate on state. No `PlaybackEngine struct`, no `Compiler struct`, no `InputManager struct` — just functions. `controller.RunPlayback(ctx, project)` and `controller.RunCompile(ctx, project)` are functions you start in goroutines from main, operating on `*model.Project`.
   - `controller/devices/drum.go`: `Compile(state *model.Drum, seed uint64) CompiledPattern`, `ToggleStep(state *model.Drum, lane, step int)`, etc.
   - `controller/system/project.go`: `SaveProject(p *model.Project, path string) error`, `LoadProject(path string) (*model.Project, error)`, etc.
   - Functions are universally callable: anyone can `import "go-sequence/controller/system"` and call `system.SaveProject(...)`.

4. **View is reactive — `view = f(model)`.** Renders model state. Captures user input gestures and interprets them, then calls controller functions to mutate state. View never holds state of its own. Controller never calls view.

5. **All UI state lives in model.** Focus (which device/widget is currently shown), edit cursors, browser cursors, modal flags — they're state, they live in `model/`. The view just reads them. Marked `json:"-"` if they shouldn't persist.

6. **View owns UI input loops** (pad presses, terminal keyboard) — knows the launchpad surface for both rendering and input. Reads `project.UI.Focus`, dispatches to `view/devices/<kind>.HandlePad(state, project, row, col)` which interprets the gesture and calls controller mutators.

7. **Controller owns external input loops** — `controller.RunMidiInput(ctx, project, midiPorts)` subscribes per-track to `(port, channel)` and dispatches MIDI events to controller mutators (e.g., `drum.RecordEvent`). MIDI input is external hardware, not UI gestures.

8. **Playback is a pure-walker function.** `controller.RunPlayback(ctx, project)` is the goroutine entry point. Reads `project.Playback.CurrentEvents`, walks events, dispatches MIDI. At wrap, atomic-swap with `project.Playback.NextEvents` and signal compile goroutine.

9. **Compile is the only producer of CompiledPatterns.** `controller.RunCompile(ctx, project)` is the goroutine entry point. Receives trackIdx from `project.Compile.Channel`, calls `controller/devices/<kind>.Compile(spec, seed)`, atomic-stores into `project.Playback.NextEvents[track]`.

9. **Mutators trigger compile.** When a `controller/devices/drum.ToggleStep(...)` (or similar) mutates state, it bumps that track's `editCounter` and signals compile (via channel send). Mutators know when state changed; they're the natural place to fire the signal.

10. **Fresh rolls every loop.** When playback wraps and swaps in `next_events`, signals compile to produce new `next_events` with a fresh seed. Live-feel: every loop sounds different.

11. **One code path per concept.** Tick loop is uniform. Modular cursor wrap handles length changes / pattern swaps for free. No special cases.

12. **Naming: package path conveys role + domain.** `model/devices/drum.go` is unambiguous. Files at parallel paths across layers. No role suffixes inside files (`Drum`, not `DrumState`).

---

## 4. Data flows

### 4.1 Tick (the walker)

Pure walk, no compile work. On wrap, atomic-swap and signal compile goroutine.

```
clock advances → PlaybackEngine.tick(globalTick):
    if not pe.playing.Load(): return
    delta = globalTick - lastGlobalTick
    if delta > MAX_DELTA_TICKS: delta = 1   // clamp on hiccup
    for each track:
        buf  = atomic.Load(current_events[track])   // CompiledPattern + length
        plen = buf.Length
        cursor.Tick %= plen
        newTick = (cursor.Tick + delta) % plen
        wrapped = newTick < cursor.Tick
        for each event in buf.Events:
            if event.Tick in (cursor.Tick, newTick] (split if wrapped):
                midi.Send(track.Port, track.Channel, event.Event)
        cursor.Tick = newTick
        if wrapped:
            handleWrap(track)

handleWrap(track):
    if sched.Queued != -1:
        sched.Playing = sched.Queued
        sched.Queued  = -1
    next_buf := atomic.Load(next_events[track])
    current_stale := buf.EditCounter < editCounters[track].Load()
    if next_buf != nil:
        atomic.Store(current_events[track], next_buf)
        atomic.Store(next_events[track], nil)
    elif current_stale:
        atomic.Store(current_events[track], syncCompile(track))   // sync fallback
    else:
        // current is fine; let it loop again
    compileCh <- track   // request next iteration
```

### 4.2 Pattern edit

```
pad press → surface → InputManager.HandlePad(row, col):
    InputManager identifies focused domain (e.g., focused on Drum track 0)
    calls view/devices/drum.HandlePad(drumState, project, row, col)
        view/drum decides what mutation to make (e.g., "user toggled step 3")
        calls controller/devices/drum.ToggleStep(drumState, lane, step)
            ToggleStep takes track lock, mutates state, releases
        returns
    InputManager increments editCounters[track]
    InputManager sends compileCh <- track
```

InputManager doesn't know about drum-specific operations. It knows "pad press at row/col on focused track." `view/devices/drum.HandlePad` knows what that means in drum context, calls the appropriate controller function.

### 4.3 Pattern queue (Session)

```
pad press in Session view → InputManager.HandlePad(row, col):
    Focus.Kind == Session, so:
    calls view/system/session.HandlePad(project, row, col)
        view/system/session decides "user queued pattern col on track row"
        calls controller/system/session.QueuePattern(project, row, col)
            QueuePattern takes track lock, sets Schedule.Queued = col, releases
        returns
    InputManager sends compileCh <- row   // queued pattern's events need rendering
```

### 4.4 Preview MIDI (no state change)

```
pad press in Drum view, drum not recording → view/devices/drum.HandlePad(state, ...):
    view checks state.Recording == false
    calls midi.Send(track.PortName, track.Channel, midi.Event{...})   // direct send
    no controller call, no state mutation
```

### 4.5 Save / Load

```
save:
    user presses save in project view
    view/system/project.HandleKey(project, key) interprets "save"
    calls controller/system/project.SaveProject(project, name)
        marshals JSON, writes to disk

load:
    user picks save file in project view
    view/system/project.HandlePad(project, row, col) interprets "load file at row/col"
    calls controller/system/project.LoadProject(path):
        playback.Stop()                    // stop playback first
        proj, _ := readAndUnmarshal(path)
        proj.Validate()
        InputManager replaces project pointer
        emits compileCh <- N for each track   // refresh all events for new state
```

### 4.6 Clock

Internal wall-clock timer drives playback. PPQ = 960 (compile-time constant).

```
tick interval (ns) = 60e9 / (Tempo * PPQ)

on play:                           // InputManager calls pe.Play()
    pe.playing.Store(true)
    pe.t0 = time.Now()
    pe.tick.Store(0)

on pause:                          // pe.Pause()
    pe.playing.Store(false)

on tempo change:
    pe.t0 = time.Now() - (pe.tick.Load() * newInterval)
    // events use ticks, not wall time. No recompile needed.

tick goroutine:
    for pe.playing.Load():
        sleep until next tick boundary
        globalTick = int64((time.Now() - pe.t0) / interval)
        if globalTick > pe.tick.Load():
            pe.tick(globalTick)
            pe.tick.Store(globalTick)
```

Transport (`playing`, `t0`, `tick`) lives in PlaybackEngine, not Model. Model is pure persisted data.

### 4.7 MIDI input routing

```
midi.Subscribe(portName, channel uint8) → <-chan Event
    // filtered; buffer=64; overflow drops oldest

InputManager on startup:
    for each track:
        go func(trackIdx int):
            ch, _ := midi.Subscribe(track.PortName, track.Channel)
            for ev := range ch:
                track := project.Tracks[trackIdx]
                track.mu.Lock()
                view/devices/<kind>.HandleMIDI(trackState, ev)
                    // view layer interprets the MIDI event in domain context
                    // calls e.g. controller/devices/drum.RecordEvent(state, ev)
                track.mu.Unlock()
                editCounters[trackIdx].Add(1)
                compileCh <- trackIdx
```

### 4.8 Focus

`InputManager.focused FocusTarget` selects the active domain. Mutated only by InputManager itself (no lock). Read by View via small value-copy accessor.

Triggers: number keys → track focus, `,` → Settings, `s` → Save (project view), tab → Session.

### 4.9 View rendering

View runs ~60Hz goroutine:

```
for each frame (~16ms):
    focused := InputManager.Focused()
    track.mu.RLock()
    switch focused.Kind:
    case Track:
        kind := project.Tracks[focused.Track].Type
        switch kind:
        case Drum:       tuiStr, leds = view/devices/drum.Render(...), view/devices/drum.RenderLEDs(...)
        case Piano:      ...
        case Metropolix: ...
    case Save:    tuiStr, leds = view/system/project.Render(...), view/system/project.RenderLEDs(...)
    case Settings: ...
    track.mu.RUnlock()
    print tuiStr; surface.SetLEDs(leds)
```

---

## 5. Per-package reference

### 5.1 `model/`

Pure data. No I/O. No behavior beyond `Validate()`.

```
model/
├── project.go          — Project, Track, Schedule, DeviceKind, Validate()
├── runtime.go          — UIState, PlaybackState, CompileState (json:"-" runtime fields)
├── devices/
│   ├── drum.go         — Drum, DrumPattern, NoteLane, DrumStep
│   ├── piano.go        — Piano, PianoPattern, NoteEvent
│   └── metropolix.go   — Metropolix, MetropolixPattern, MetropolixStage
└── system/
    ├── project.go      — project browser/UI state (current name, browser cursor, etc.)
    ├── settings.go     — settings UI state
    ├── session.go      — session UI state
    └── empty.go        — empty/default state (rare)
```

`Project.Tracks` holds `[8]*Track`. Each Track has a Kind plus pointer fields for `*devices.Drum`, `*devices.Piano`, `*devices.Metropolix`. Validate enforces "exactly one non-nil matches Kind."

Runtime state on `Project` (all `json:"-"`):

```go
type Project struct {
    // persisted
    Tempo  int
    Tracks [8]*Track
    
    // runtime
    UI       UIState        `json:"-"`
    Playback PlaybackState  `json:"-"`
    Compile  CompileState   `json:"-"`
}

type UIState struct {
    Focus FocusTarget   // which device/widget is shown
    // ...other ephemeral UI bits
}

type PlaybackState struct {
    Cursors        [8]Cursor                                 // per-track playback position
    CurrentEvents  [8]atomic.Pointer[CompiledPattern]        // what's playing
    NextEvents     [8]atomic.Pointer[CompiledPattern]        // pre-rendered
    EditCounters   [8]atomic.Uint64                          // bumped on every spec mutation
    Playing        atomic.Bool
    T0             time.Time
    Tick           atomic.Int64
}

type CompileState struct {
    Channel     chan int       // trackIdx; capacity ~16
    LoopCounter atomic.Uint64  // for seeding fresh rolls
}

type CompiledPattern struct {
    Events      []TimedEvent
    Length      int64
    EditCounter uint64   // stamped at compile time, used for stale-detection at wrap
}

type TimedEvent struct {
    Tick  int64
    Event midi.Event
}
```

Note `CompiledPattern` and `TimedEvent` are still in `model/` because they're state types. They're never persisted (CompiledPattern is in `Playback.CurrentEvents` which is `json:"-"`), but the type definitions belong with Model.

### 5.2 `midi/` (at repo root)

Data types + OS I/O. Imported by Model (for types) and by Controller (for types + I/O).

```go
package midi

type EventType uint8
type Event struct { Type EventType; Note, Velocity, Channel uint8; Value int16 }

type ToExternal interface { Send(portName string, channel uint8, ev Event) error }
type FromExternal interface { Subscribe(portName string, channel uint8) (<-chan Event, error) }

type Ports struct { ... }   // implements both
```

### 5.3 `view/`

Renders state, captures input. Calls controller functions to mutate.

```
view/
├── tui.go              — top-level Bubbletea Model; routes input to InputManager
├── devices/
│   ├── drum.go         — Render, RenderLEDs, HandlePad, HandleKey, HandleMIDI for drum
│   ├── piano.go
│   └── metropolix.go
└── system/
    ├── project.go      — project browser UI
    ├── settings.go
    ├── session.go
    └── empty.go
```

Per-domain file exports free functions like:
```go
func Render(state *model.Drum, project *model.Project) string
func RenderLEDs(state *model.Drum, project *model.Project) []surface.LED
func HandlePad(state *model.Drum, project *model.Project, row, col int)
func HandleKey(state *model.Drum, project *model.Project, key string)
func HandleMIDI(state *model.Drum, ev midi.Event)
```

`HandlePad`/`HandleKey`/`HandleMIDI` interpret the gesture in domain context and call the appropriate `controller/devices/drum.<func>` to mutate state.

### 5.4 `controller/`

Functions on state, organized by domain. PlaybackEngine, InputManager, Compiler at the top.

```
controller/
├── playback.go         — RunPlayback(ctx, project): tick loop goroutine
├── compile.go          — RunCompile(ctx, project): compile goroutine
├── midi_input.go       — RunMidiInput(ctx, project, ports): external MIDI input
├── transport.go        — Play(project), Pause(project), SetTempo(project, bpm)
├── devices/
│   ├── drum.go         — Compile, ToggleStep, RecordEvent, ...
│   ├── piano.go
│   └── metropolix.go
└── system/
    ├── project.go      — SaveProject, LoadProject, ListSaves, NewProject, ...
    ├── settings.go     — ApplySettings, ResubscribeMIDI, ...
    ├── session.go      — QueuePattern, ClearQueue, ...
    └── empty.go        — minimal
```

No structs. `RunPlayback`, `RunCompile`, `RunMidiInput` are just functions — they spawn goroutines internally or are started as goroutines from main. All state they operate on lives in `*model.Project`.

Per-domain controller file exports free functions:

```go
// controller/devices/drum.go
package drum  // or package devices, with files like drum.go inside

import "go-sequence/model/devices"

func Compile(state *devices.Drum, seed uint64) controller.CompiledPattern { ... }
func ToggleStep(state *devices.Drum, lane, step int) { ... }
func ClearPattern(state *devices.Drum, patternIdx int) { ... }
```

```go
// controller/system/project.go
package project  // or package system, with files like project.go inside

import "go-sequence/model"

func SaveProject(p *model.Project, path string) error { ... }
func LoadProject(path string) (*model.Project, error) { ... }
func ListSaves() ([]SaveInfo, error) { ... }
```

Top-level controller functions in `package controller` operate directly on `*model.Project`. No struct state.

```go
// controller/playback.go
package controller

func RunPlayback(ctx context.Context, project *model.Project, out midi.ToExternal) {
    ticker := time.NewTicker(...)
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            if !project.Playback.Playing.Load() { continue }
            tick(project, out)
        }
    }
}

func tick(project *model.Project, out midi.ToExternal) {
    // reads project.Playback.CurrentEvents, walks, dispatches, at-wrap swaps buffers
    // and does project.Compile.Channel <- trackIdx
}
```

```go
// controller/compile.go
package controller

func RunCompile(ctx context.Context, project *model.Project) {
    for {
        select {
        case <-ctx.Done(): return
        case trackIdx := <-project.Compile.Channel:
            compileTrack(project, trackIdx)
        }
    }
}

func compileTrack(project *model.Project, trackIdx int) {
    track := project.Tracks[trackIdx]
    track.mu.RLock()
    defer track.mu.RUnlock()
    seed := uint64(time.Now().UnixNano()) ^ uint64(trackIdx) ^ project.Compile.LoopCounter.Add(1)
    var events model.CompiledPattern
    switch track.Type {
    case model.DeviceDrum:       events = drum.Compile(track.Drum, seed)
    case model.DevicePiano:      events = piano.Compile(track.Piano, seed)
    case model.DeviceMetropolix: events = metropolix.Compile(track.Metropolix, seed)
    }
    events.EditCounter = project.Playback.EditCounters[trackIdx].Load()
    project.Playback.NextEvents[trackIdx].Store(&events)
}
```

```go
// controller/transport.go
package controller

func Play(project *model.Project) {
    project.Playback.T0 = time.Now()
    project.Playback.Tick.Store(0)
    project.Playback.Playing.Store(true)
}

func Pause(project *model.Project) { project.Playback.Playing.Store(false) }
func SetTempo(project *model.Project, bpm int) { /* mutates Tempo, re-anchors T0 */ }
```

View functions (e.g., `view/devices/drum.HandlePad`) interpret input gestures in domain context and call mutator functions in `controller/devices/drum.go`. The mutator bumps the edit counter and signals compile:

```go
// controller/devices/drum.go
package drum

func ToggleStep(project *model.Project, trackIdx int, lane, step int) {
    track := project.Tracks[trackIdx]
    track.mu.Lock()
    track.Drum.Patterns[N].Notes[lane].Steps[step].Active = ...
    track.mu.Unlock()
    project.Playback.EditCounters[trackIdx].Add(1)
    project.Compile.Channel <- trackIdx
}
```

### 5.5 `controller/surface/`

Hardware control surfaces. Currently Launchpad X.

```go
package surface

type Surface interface {
    PadEvents() <-chan PadEvent
    SetLEDs(leds []LED)
    Close() error
}

type PadEvent struct { Row, Col int; Down bool }
type LED struct { Row, Col int; R, G, B uint8 }
```

---

## 6. Concurrency model

Four kinds of goroutines, all operating on `*model.Project`.

| Goroutine                     | Reads                                                    | Writes                                                                                       |
|-------------------------------|----------------------------------------------------------|----------------------------------------------------------------------------------------------|
| `controller.RunPlayback`      | Playback.CurrentEvents/NextEvents (atomic), Schedule    | Playback.Cursors (own), Playback.Tick, atomic-swap CurrentEvents/NextEvents at wrap, Schedule promotion (under track.mu Lock) |
| `controller.RunCompile`       | track spec under track.mu RLock, Playback.EditCounters  | Playback.NextEvents (atomic store), Compile.LoopCounter                                      |
| `controller.RunMidiInput`     | midi.Subscribe channels                                  | track spec under track.mu Lock (via device mutators); Playback.EditCounters; Compile.Channel |
| `view.Run` (+ pad/key loops)  | model.Project (everything, via RLock where needed)      | calls into controller mutators; never writes model directly                                  |

### Synchronization primitives

All on `*model.Project`:

- **`Playback.CurrentEvents[i]`, `Playback.NextEvents[i]`**: `atomic.Pointer[CompiledPattern]`. Lock-free reads. Compile stores; playback loads + atomic-swaps at wrap.
- **`Track.mu` (sync.RWMutex)**: protects spec fields. Controller mutator functions take Lock; Compile and view reading take RLock. Playback does NOT touch spec — only reads events.
- **`Playback.EditCounters[i]`**: `atomic.Uint64`. Bumped by controller mutators after any spec/schedule mutation. Compile stamps the value onto each CompiledPattern, used at wrap for stale-detection.
- **`Compile.Channel`**: `chan int` buffered ~16. Sender doesn't block. Receivers = Compile goroutine.
- **`Playback.Tick`, `Playback.Playing`**: `atomic.Int64` and `atomic.Bool`.

---

## 7. Compile invariants and PRNG

- Compile is a pure function of (spec, seed). Same inputs → same output. Seed is the only source of non-determinism.
- Fast: target sub-millisecond for typical patterns.
- No side effects: no I/O, no mutation of spec, no global state.

Seed: `time.Now().UnixNano() ^ trackIdx ^ loopCounter`. Logged at debug for replay.

PRNG: xorshift64 in `controller/rng.go` (or a sub-package). Not Go's `math/rand` default.

Compile triggered (via `compileCh <- trackIdx`) on:
- Pattern edit (via controller mutator functions called from view)
- Pattern queue (Session view → controller/system/session.QueuePattern)
- Wrap boundary (PlaybackEngine after swap)
- MIDI input that records into pattern
- Project load (one signal per track)

---

## 8. Validation and edge cases

`Project.Validate()` runs after Load:

- `Tempo` ∈ [20, 300]
- Per-track `Schedule.Playing` ∈ [0, NumPatterns-1]
- Per-track `Schedule.Queued` ∈ [-1, NumPatterns-1]
- Pattern lengths ≥ 1
- `Track.Type` matches exactly one non-nil device field
- Domain-specific clamps (pitch 0-127, etc.)

Edge cases handled by uniform code paths:
- Tempo=0 → no ticks. Pattern length=0 → clamped to 1. Probability=0/100 → never/always fire. Pattern length change mid-play → cursor modular-wraps. Compile overrun → either reuse current (fine) or sync-compile (if stale).

---

## 9. Open questions

1. **Per-track compile goroutines vs. one shared?** Plan starts with one. Upgrade if profiling shows bottleneck.
2. **Save format compatibility** — pre-1.0; breaking is acceptable.
3. **Triple-buffering** if double doesn't suffice. Defer.
4. **Do `controller/devices/drum.go` and `view/devices/drum.go` share a Go package name?** Probably no — separate packages (`drum` for controller side, `drumview` for view side, OR `package devices` repeated in both layers). Cleaner with separate packages so name collisions don't happen at import sites.

---

## 10. Out of scope (future work)

- True per-step rolling (hardware-style live roll). Our approach has fresh rolls per loop, not per step. For small patterns indistinguishable.
- Named pattern chains ("meta-patterns"). Schedule extends cleanly.
- Multi-controller support (Surface interface admits this).
- VST hosting (not a DAW).
- Audio output (MIDI only).
- MIDI clock out.
