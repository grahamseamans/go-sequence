# go-sequence

A free, software clone of expensive hardware MIDI sequencers (Cirklon, Hapax, Pyramid, Metropolix, Torso T-1), driven by a Novation Launchpad. Not a DAW — a MIDI brain that sits between your controller and your synths.

This document describes the target architecture. Source of truth for design decisions. Check git history for older versions.

---

## 1. The whole system, in five lines

- **Model** — state. Human patterns (what you edit), schedule (which patterns in what order), transport (playing/stopped/tempo). No compiled/derived data.
- **Controller: devices** — pure compilers. `Compile(spec, rngSeed)` → list of `(tick, midi event)` pairs. Different seed → different rolls.
- **Controller: playback engine** — walks pre-compiled events with a cursor. Double-buffered per track: `current_events` is playing now, `next_events` is the next iteration pre-rendered. On wrap: atomic-swap buffers, signal compile goroutine to produce a new `next_events`. Tick path is pure walk + dispatch.
- **Controller: input manager** — receives events (pad/key/MIDI in). Mutates state. On pattern edit, signals compile goroutine to re-render that track. On queue, signals compile goroutine to render the queued pattern.
- **View** — renders state.

Everything else is implementation detail.

---

## 2. Architecture

```
                        ┌────────────────┐
                        │     model/     │   data only; human patterns,
                        │ Project, Drum, │   schedule, transport
                        │ Piano, ...     │
                        └────────────────┘
                          ▲             ▲
                  reads   │             │ reads + mutates
                          │             │
                ┌─────────┴───┐  ┌──────┴────────────────────────┐
                │    view/    │  │         controller/           │
                │  TUI now,   │  │  playback.go — tick loop,     │
                │  Wails next │  │    walks current_events,      │
                │             │  │    double-buffered per track  │
                │             │  │  input.go    — event router,  │
                │             │  │    signals compile on edit    │
                │             │  │  compile.go  — goroutine,     │
                │             │  │    consumes channel, renders  │
                │             │  │    events into next_events    │
                │             │  │  project.go  — save/load      │
                │             │  │  ┌──────────────────────────┐ │
                │             │  │  │ devices/ pure compilers  │ │
                │             │  │  └──────────────────────────┘ │
                │             │  │  ┌──────────────────────────┐ │
                │             │  │  │ surface/ Launchpad, Mock │ │
                │             │  │  └──────────────────────────┘ │
                └─────────────┘  └───────────────────────────────┘
                       │                       │
                       │                       │
                       ▼                       ▼
                    keyboard               Launchpad, MIDI hardware
                                           (via midi/ package at root)
```

`midi/` lives at the repo root (not inside `controller/`) because it holds both data types (Event, EventType) and I/O primitives (Ports, Send, Subscribe) that are cross-cutting — Model imports `midi` for type definitions, Controller imports `midi` for types + I/O.

---

## 3. Principles

1. **Model is pure data, no derived/runtime data.** All persisted state lives in `model/`. Save = serialize the package. Load = deserialize back. Compiled events, current cursor positions, RNG seeds — none of that lives here. They're runtime state owned by the playback engine.

2. **Devices are pure compilers.** Empty-struct singletons (`var Drum devices.Drum`). Signature: `Compile(spec *model.Drum, seed uint64) CompiledPattern`. Same spec + same seed → same events. Different seeds → different rolls. Probability, ratchet randomness, any roll-based decisions happen inside Compile.

3. **Playback engine is a pure walker.** Every tick: read current events, dispatch any whose tick is in (prevCursor, newCursor]. No compile logic in the tick path. At wrap boundary, atomic-swap current ← next, send a compile signal.

4. **Input manager owns writes + triggers compiles.** Sole writer to Model. On pattern edit: mutate spec, then signal "recompile this track" to the compile goroutine. On schedule change (queue a pattern): mutate schedule, signal compile with the queued pattern's spec.

5. **Compile goroutine is the only producer of CompiledPatterns.** Receives `trackIdx` on a channel, calls the appropriate device's `Compile`, atomic-stores the result into `next_events[trackIdx]`. Runs on its own goroutine; never blocks playback.

6. **Fresh rolls every loop.** When playback wraps and swaps in `next_events`, it sends a compile signal so the next `next_events` gets rendered with a FRESH seed. Live-feel: every loop sounds different. Deterministic if you log the seeds.

7. **One code path per concept.** Tick loop is always the same logic. Wrap handling is uniform. Modular cursor math handles length changes and pattern swaps for free.

8. **Naming: package path conveys role, no role suffixes.** `model.Drum` is data, `controller/devices.Drum` is behavior. Same domain word in both. Go-idiomatic anti-stutter.

---

## 4. Data flows

### 4.1 Tick (the walker)

Pure walk, no compile work. On wrap, atomic-swap and signal compile goroutine.

```
clock advances → PlaybackEngine.tick(globalTick):
    if not pe.playing.Load(): return
    delta = globalTick - lastGlobalTick
    // Clamp delta to one pattern length max. If the clock jumped (laptop
    // slept, host hiccup, pause→play), we don't try to replay multiple
    // loops' worth of missed events — just resume from current position.
    if delta > MAX_DELTA_TICKS: delta = 1
    for each track:
        buf   = atomic.Load(current_events[track])   // []TimedEvent + length
        plen  = buf.Length
        cursor.Tick %= plen                          // handles prior length changes
        newTick = (cursor.Tick + delta) % plen
        wrapped = newTick < cursor.Tick
        for each event in buf.Events:
            if event.Tick in (cursor.Tick, newTick] (split if wrapped):
                midi.Send(track.Port, track.Channel, event.Event)
        cursor.Tick = newTick
        if wrapped:
            handleWrap(track)

handleWrap(track):
    // Schedule advance (if queued pattern pending)
    if sched.Queued != -1:
        sched.Playing = sched.Queued
        sched.Queued  = -1
    // Atomic swap: the already-compiled next becomes current
    next_buf := atomic.Load(next_events[track])
    current_stale := (buf.EditCounter < editCounters[track].Load())
    if next_buf != nil:
        atomic.Store(current_events[track], next_buf)
        atomic.Store(next_events[track], nil)
    elif current_stale:
        // Edits have happened since current_events was compiled AND next
        // isn't ready. Don't play stale events — sync-compile right here.
        // Brief stall, but bounded (<1ms typical); worst case user hears
        // one tick of silence. Better than wrong notes.
        log.Warn("sync compile at wrap, track", track)
        atomic.Store(current_events[track], syncCompile(track))
    else:
        // next not ready but current is still valid (no edits since render).
        // Just reuse current for another loop. Live-feel lost for one loop.
        log.Debug("next not ready, reusing current, track", track)
    // Request a fresh next iteration
    compileCh <- track
```

### 4.2 Pattern edit (mutation + recompile)

```
pad press → surface → InputManager.HandlePad(row, col)
  → InputManager acquires state.mu.Lock()
  → InputManager calls focused device: devices.Drum.HandlePad(state, project, out, row, col)
    → device mutates state.Patterns[N] (e.g., toggle a step)
    → return
  → InputManager releases state.mu.Unlock()
  → InputManager: compileCh <- trackIdx   // signal: this track's next_events is stale
```

The compile goroutine picks up, renders the edited spec with a fresh seed, and atomic-stores into `next_events[trackIdx]`. On the next wrap boundary, that becomes `current_events` and the edit takes effect.

User-perceived behavior: "edit now, hear change next bar."

### 4.3 Queue (pattern switching)

```
pad press in Session view → Session.HandlePad(state, project, out, row, col)
  → device mutates state.Tracks[row].Drum.Schedule.Queued = col
  → return
  → InputManager: compileCh <- row     // signal: recompile this track's next_events,
                                        //   but using the QUEUED pattern's spec this time
  → LED render next frame: pattern-col pad flashes (reads Schedule.Queued)
```

The compile goroutine reads the track's `Schedule.Queued`, compiles the queued pattern's spec (not the currently playing one), and stores the events as `next_events`. At wrap, playback swaps in and schedules promote — the queued pattern starts playing with its pre-rendered events. Zero compile work at the transition.

### 4.4 Preview MIDI (no state, no compile)

```
pad press in Drum view, not recording → Drum.HandlePad(state, project, out, row, col)
  → device checks state.Drum.Recording == false
  → device calls out.Send(track.PortName, track.Channel, midi.Event{note, vel})
  → return
```

Direct send. No compile, no state change.

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
        return *model.Project
    InputManager replaces the shared project state.
    InputManager signals compile for each track (current_events / next_events
      are stale since they reference the old project's specs).
    When user hits play: playback reads fresh events.
```

No machine patterns to persist or recompute on load. State is just the spec.

### 4.6 Clock

Internal wall-clock timer drives playback. PPQ = 960 (compile-time constant).
Transport state (`playing`, `t0`, `tick`) lives in PlaybackEngine, not Model.

```
tick interval (ns) = 60e9 / (Tempo * PPQ)

on play:                            // InputManager calls pe.Play()
    pe.playing.Store(true)
    pe.t0 = time.Now()
    pe.tick.Store(0)

on pause:                           // InputManager calls pe.Pause()
    pe.playing.Store(false)

on tempo change during play:
    pe.t0 = time.Now() - (pe.tick.Load() * newInterval)
    // Tempo change does NOT require recompile — events use ticks, not wall time.

tick loop goroutine:
    for pe.playing.Load():
        sleep until next tick boundary
        globalTick = int64((time.Now() - pe.t0) / interval)
        if globalTick > pe.tick.Load():
            pe.tick(globalTick)
            pe.tick.Store(globalTick)
```

InputManager controls playback via public methods on PlaybackEngine:
`pe.Play()`, `pe.Pause()`, `pe.SetTempo(bpm)`. No shared runtime state in Model.

### 4.7 MIDI input routing

All routing logic in `midi/`:

```
midi.Subscribe(portName, channel uint8) → <-chan Event
    // filtered; buffer=64; overflow drops oldest
    // multiple subscribers for same (port,channel) each get a copy

InputManager on startup:
    for each track:
        go func(trackIdx int):
            ch := midi.Subscribe(track.PortName, track.Channel)
            for event := range ch:
                track := project.Tracks[trackIdx]
                track.mu.Lock()                   // per-track, consistent with edit/queue flows
                devices.<Kind>.HandleMIDI(trackState, im.out, event)
                track.mu.Unlock()
                editCounters[trackIdx].Add(1)     // signal "spec changed"
                compileCh <- trackIdx             // trigger recompile
```

### 4.8 Focus

```go
type FocusTarget struct {
    Kind  FocusKind
    Track int
}
```

Mutated only by InputManager's own goroutine. Read by View via a small value-copy accessor. Triggers: number keys → track focus, `,` → Settings, `s` → Save, tab → Session. Session pad taps don't change focus.

### 4.9 View rendering

Runs ~60Hz goroutine:

```
for each frame (~16ms):
    focused := InputManager.Focused()            // small value copy
    track.<Kind>.mu.RLock()
    tuiStr  := devices.<Kind>.Render(deviceState, project)
    leds    := devices.<Kind>.RenderLEDs(deviceState, project)
    track.<Kind>.mu.RUnlock()
    render tuiStr; surface.SetLEDs(leds)
```

View only reads Model (the spec). It doesn't need to know about `current_events` / `next_events` — those are playback-internal.

---

## 5. Per-package reference

### 5.1 model/

Pure data. No I/O. No behavior beyond `Validate()`. No derived fields (no compiled events, no cached cursors, no RNG state).

```go
package model

type Project struct {
    Tempo  int        `json:"tempo"`
    Tracks [8]*Track  `json:"tracks"`
}

// No Transport, no Tick, no Playing. Those are runtime state owned by
// PlaybackEngine, not Model. Model is PURE persisted data.

type Track struct {
    Name       string        `json:"name"`
    Channel    uint8         `json:"channel"`
    PortName   string        `json:"portName,omitempty"`
    Type       DeviceKind    `json:"type"`
    mu         sync.RWMutex  `json:"-"`
    Drum       *Drum         `json:"drum,omitempty"`
    Piano      *Piano        `json:"piano,omitempty"`
    Metropolix *Metropolix   `json:"metropolix,omitempty"`
}

type Drum struct {
    Patterns [16]DrumPattern  `json:"patterns"`   // specs only
    Schedule Schedule          `json:"schedule"`
    EditingPatternIdx int      `json:"editingPattern"`
    Recording bool             `json:"-"`
}

type DrumPattern struct {
    // human-editable spec only. No compiled events.
    Notes  [16]NoteLane    `json:"notes"`
    Length int             `json:"length"`   // in ticks
}

// Piano, Metropolix similar — spec only. No machine/events field.

type Schedule struct {
    Playing int `json:"playing"`
    Queued  int `json:"queued"`
}

func (p *Project) Validate()  // clamps all fields recursively
```

Note: `Track.mu` protects writes to the track's spec fields (Drum/Piano/Metropolix) so the compile goroutine reading and InputManager writing don't race.

### 5.2 midi/ (at repo root)

Data types + OS I/O. Cross-cutting — imported by Model (for types), Controller (for types + I/O).

```go
package midi

type EventType uint8
const (NoteOn EventType = iota; NoteOff; CC; PitchBend; /* ... */)

type Event struct {
    Type     EventType
    Note     uint8
    Velocity uint8
    Channel  uint8
    Value    int16   // for PitchBend, CC
}

type ToExternal interface { Send(portName string, channel uint8, ev Event) error }
type FromExternal interface { Subscribe(portName string, channel uint8) (<-chan Event, error) }

type Ports struct { ... }  // implements both interfaces
func NewPorts() (*Ports, error)
func (p *Ports) Send(...) error
func (p *Ports) Subscribe(...) (<-chan Event, error)
func (p *Ports) List() []PortInfo
func (p *Ports) Close() error
```

### 5.3 view/

Bubbletea TUI now, Wails later. Reads Model (via Manager accessor). Sends key events to InputManager.

### 5.4 controller/

#### controller/playback.go

```go
package controller

type PlaybackEngine struct {
    project        *model.Project
    out            midi.ToExternal
    cursors        [8]Cursor
    currentEvents  [8]atomic.Pointer[CompiledPattern]  // what's playing now
    nextEvents     [8]atomic.Pointer[CompiledPattern]  // pre-rendered next iteration
    compileCh      chan<- int                          // signals compile goroutine
    lastGlobalTick int64

    // Transport / clock state — owned here, NOT in Model. Model is pure
    // persisted data; runtime clock state lives with the engine that walks it.
    playing        atomic.Bool
    t0             time.Time    // wall-clock anchor when play started
    tick           atomic.Int64 // current global tick; written by tickLoop, read by everyone else

    // Edit tracking: incremented when InputManager signals a recompile for
    // a track. Compiler records the edit counter it rendered against; at
    // wrap, if current_events has a lower counter than the latest edit, the
    // events are stale and we do a sync recompile rather than reuse them.
    editCounters [8]atomic.Uint64
}

type Cursor struct { Tick int64 }

type CompiledPattern struct {
    Events      []TimedEvent
    Length      int64   // pattern length in ticks
    EditCounter uint64  // Compiler records track's editCounters[i] at compile time.
                        // Used at wrap to detect stale: if current_events.EditCounter
                        // < editCounters[i].Load() and next isn't ready, sync-compile
                        // instead of reusing current.
}

type TimedEvent struct {
    Tick  int64
    Event midi.Event
}

func NewPlaybackEngine(project *model.Project, out midi.ToExternal, compileCh chan<- int) *PlaybackEngine
func (pe *PlaybackEngine) Run(ctx context.Context)
func (pe *PlaybackEngine) ResetCursors()  // called after project swap
```

#### controller/compile.go

```go
package controller

type Compiler struct {
    project    *model.Project
    nextEvents *[8]atomic.Pointer[CompiledPattern]
    compileCh  <-chan int
}

func (c *Compiler) Run(ctx context.Context):
    for trackIdx := range c.compileCh:
        c.compileTrack(trackIdx)

func (c *Compiler) compileTrack(trackIdx int):
    track := c.project.Tracks[trackIdx]
    seed := uint64(time.Now().UnixNano()) ^ uint64(trackIdx) ^ counter.Add()
    track.mu.RLock()
    defer track.mu.RUnlock()
    switch track.Type:
        case Drum:       events = devices.Drum.Compile(track.Drum, seed)
        case Piano:      events = devices.Piano.Compile(track.Piano, seed)
        case Metropolix: events = devices.Metropolix.Compile(track.Metropolix, seed)
    c.nextEvents[trackIdx].Store(&events)
```

The compile goroutine serializes all compiles across all tracks. Simpler than per-track goroutines. If compile is fast (microseconds), this is totally fine. If ever a bottleneck, split into per-track goroutines or a worker pool.

#### controller/input.go

```go
package controller

type InputManager struct {
    project   *model.Project
    out       midi.ToExternal
    in        midi.FromExternal
    surface   surface.Surface
    focused   FocusTarget
    compileCh chan<- int
}

func NewInputManager(project *model.Project, out midi.ToExternal, in midi.FromExternal, surface surface.Surface, compileCh chan<- int) *InputManager
func (im *InputManager) Run(ctx context.Context)
func (im *InputManager) HandleKey(key string)
func (im *InputManager) HandlePad(row, col int)
```

InputManager acquires the track's RWMutex for writes. After any write that affects a track's compiled events (edit, queue), sends the trackIdx on `compileCh`.

#### controller/project.go

```go
package controller

func SaveProject(p *model.Project, path string) error
func LoadProject(path string) (*model.Project, error)
```

#### controller/devices/

Each device is an empty struct with value-receiver methods. Pure compilers + input handlers + renderers.

```go
package devices

type Drum struct{}

// pure compile — spec + seed → compiled pattern
func (Drum) Compile(spec *model.Drum, seed uint64) CompiledPattern

// input handlers — mutate state; caller holds the track's Lock
func (Drum) HandlePad(state *model.Drum, project *model.Project, out midi.ToExternal, row, col int)
func (Drum) HandleKey(state *model.Drum, project *model.Project, out midi.ToExternal, key string)
func (Drum) HandleMIDI(state *model.Drum, out midi.ToExternal, ev midi.Event)

// view helpers
func (Drum) Render(state *model.Drum, project *model.Project) string
func (Drum) RenderLEDs(state *model.Drum, project *model.Project) []surface.LED
```

Compile is the only method that takes a seed (because it's the only one doing rolls). Handlers mutate spec; compile is triggered externally by InputManager via the channel.

#### controller/surface/

```go
package surface

type Surface interface {
    PadEvents() <-chan PadEvent
    SetLEDs(leds []LED)
    Close() error
}

type PadEvent struct { Row, Col int; Down bool }
type LED struct { Row, Col int; R, G, B uint8 }
type Launchpad struct { ... }
type Mock struct { ... }
```

---

## 6. Concurrency model

Three core goroutines:

| Goroutine                    | Reads                                          | Writes                                         |
|------------------------------|------------------------------------------------|------------------------------------------------|
| PlaybackEngine.Run           | current_events (atomic), next_events (atomic) | cursors, pe.tick (atomic), current_events (atomic swap at wrap), next_events (atomic clear at wrap), Schedule.Playing/Queued at wrap boundary (under Track.mu Lock, brief) |
| Compiler.Run                 | project state under Track.mu (RLock), editCounters (atomic) | next_events (atomic store)                     |
| InputManager (Run + HandleKey)| surface pad events, spec                      | spec fields, Schedule (under Track.mu Lock); pe.playing, pe.t0 via methods; editCounters (atomic inc); sends to compileCh |

### Synchronization primitives

- **`current_events[i]`, `next_events[i]`**: `atomic.Pointer[CompiledPattern]`. Lock-free reads. Playback loads, compile stores. Atomic swap at wrap.
- **`Track.mu` (sync.RWMutex)**: protects the spec fields. InputManager takes Lock to mutate; Compile goroutine takes RLock to read; View takes RLock to render. Playback does NOT touch spec — only reads events. Contention expected to be trivial (View 60Hz + sporadic compile + bursty writes). If it ever becomes a bottleneck, upgrade to COW snapshots (atomic pointer per spec) — noted in open questions.
- **`editCounters[i]`**: `atomic.Uint64`. InputManager increments on any edit to track i. Compiler reads and stores on the resulting CompiledPattern, for stale-detection at wrap.
- **`compileCh chan int`**: buffered ~16. InputManager sends trackIdx when a track's events need regenerating (edit, queue, MIDI recording, wrap promotion). Buffered so sender doesn't block.
- **`pe.tick`, `pe.playing`**: `atomic.Int64` and `atomic.Bool`. Owned by PlaybackEngine. `pe.t0` is a plain `time.Time` written only when playing transitions (under a brief write lock) and read only by the tick goroutine.

### Why this works

- Playback never blocks: loads atomic pointer, walks events, done.
- Compile never blocks playback: produces into `next_events` which playback reads via atomic load.
- InputManager briefly blocks compile/view when writing spec under the track mutex — but writes are bursty and sub-millisecond.
- Fresh rolls: every compile uses a new seed; every loop sounds different.
- Fallback: if `next_events` is nil when wrap happens (compile overran), playback re-uses current for another loop. Logs a warning. Should never happen in practice (compile is microseconds).

### Single-writer per field

- Playback writes: cursors, Project.Tick, atomic swap on current/next_events
- Compiler writes: next_events (atomic store only)
- InputManager writes: everything else — spec, schedule, transport, focus

---

## 7. Compile invariants and PRNG

### Compile contract

- Pure function of (spec, seed). Same inputs → same output. Seed is the only source of non-determinism.
- Fast: target sub-millisecond for typical patterns, low-millisecond for dense Metropolix with many stages/ratchets.
- No side effects: no I/O, no mutation of the spec, no global state.

### Seed generation

```
seed = nanosecond_timestamp ^ trackIdx ^ loop_counter
```

Loop counter is an atomic `uint64` incremented on every compile. Every compile gets a unique seed regardless of clock resolution.

Seeds are logged at info level (or higher) so reproducing "that cool accidental pattern" is possible via a "replay seed" UI action.

### PRNG implementation

Use **xorshift64** or similar in `controller/devices/rng.go`. Go's `math/rand` default has poor statistical quality; xorshift is fast and has well-understood properties.

```go
type Rng struct { state uint64 }
func (r *Rng) Seed(s uint64) { r.state = s | 1 /* avoid 0 */ }
func (r *Rng) Next() uint32 {
    r.state ^= r.state << 13
    r.state ^= r.state >> 7
    r.state ^= r.state << 17
    return uint32(r.state)
}
func (r *Rng) Probability(pct uint8) bool { return r.Next() % 100 < uint32(pct) }
```

### Fresh-roll trigger points

Compile is called with a fresh seed (via the compileCh signal) on:
- Pattern edit (step toggle, note change, stage parameter change)
- Pattern queue (Schedule.Queued set)
- Wrap boundary (playback promotes next → current, signals compile for a new next)
- MIDI input that records into the pattern
- Project load

---

## 8. Validation and edge cases

`Project.Validate()` runs after Load. Clamps and enforces:

- `Tempo` ∈ [20, 300]
- Per-track `Schedule.Playing` ∈ [0, NumPatterns-1]
- Per-track `Schedule.Queued` ∈ [-1, NumPatterns-1]
- Pattern lengths ≥ 1 (prevents divide-by-zero in tick modular math)
- `Track.Type` must match exactly one non-nil device field
- Drum/Piano/Metropolix-specific ranges (pitch 0-127, velocity 0-127, probability 0-100)

Edge cases handled by design:

- **Tempo = 0** → infinite tick interval → no ticks advance. Equivalent to paused. UI prevents ≤ 20 via Validate.
- **Pattern length = 0** → clamped to 1.
- **Queue during wrap** — under mutex serialization, the queued pattern either gets compiled in time for the wrap (best case) or compiles during the next iteration (still works, just one loop of "same as before" delay).
- **Compile overrun** — `next_events` is nil at wrap. Playback reuses current. Logs warning. Rare in practice.
- **Pattern length change mid-play** — cursor modular-wraps at next tick (`cursor.Tick %= plen`). No special handler.

---

## 9. Open questions

1. **Per-track compile goroutines vs one shared compile goroutine?** Plan starts with one. If profiling shows bottleneck (unlikely for this workload), upgrade to a worker pool. Flagged as "decide later."
2. **Save format compatibility** — pre-1.0; breaking existing JSON is acceptable.
3. **Triple-buffering** (current / next / rendering) if double isn't enough safety against compile overruns. Defer; double is fine for expected load.

---

## 10. Out of scope (future work)

- **True per-step rolling** (hardware-style live roll vs. our pre-rendered buffer). Our approach has fresh rolls per loop, not per step. For Metropolix's 8-stage patterns this is indistinguishable from per-step. For 64-step drum patterns, the re-roll point is the pattern boundary — arguably noticeable but low priority.
- **Named pattern chains** ("meta-patterns") — reusable sequences of patterns that queue as a single unit. Schedule extends cleanly to reference either a pattern or a chain via a small kind enum. Deferred to v2.
- **Multi-controller support** (`Surface` interface admits this).
- **VST hosting** (not a DAW).
- **Audio output** (MIDI only; route to softsynth for sound).
- **MIDI clock out** (eventually).
