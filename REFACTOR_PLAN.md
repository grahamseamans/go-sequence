# go-sequence refactor: implementation plan

Ordered, sized plan for restructuring the project from the current `sequencer/`
monolith into the MVC shape described in `DESIGN.md`.

Read `DESIGN.md` first — it is the source of truth for the target architecture.
This document answers "in what order do we build it."

Revised 2026-04-19 after Grok design review: architecture shifted to
double-buffered async pre-rendering with a dedicated compile goroutine.
Transport state moved out of Model. Machine patterns no longer persisted —
compiled events live only in PlaybackEngine's runtime state.

---

## Verified current state (as of 2026-04-19)

`go build ./...` passes. ~8,500 LOC of project code.

| Path | Current MVC role |
|---|---|
| `sequencer/state.go` (501) | Mostly Model — Project and per-device state, `var S *State` global |
| `sequencer/device.go` (62) | Controller interface |
| `sequencer/drum.go` (871) | Controller + View (device holds schedule, queue, preview chan) |
| `sequencer/pianoroll.go` (997) | Controller + View |
| `sequencer/metropolix.go` (1418) | Controller + View |
| `sequencer/manager.go` (810) | Engine: tick/queue/MIDI/LED goroutines |
| `sequencer/session.go` (335) | Controller UI, holds `*Manager` pointer |
| `sequencer/settings.go` (596) | Controller UI, note-input flag |
| `sequencer/save.go` (497) | Controller UI, calls recreateDevicesFromState |
| `sequencer/empty.go` (104) | Controller UI stub |
| `sequencer/project.go` (313) | Model persistence |
| `sequencer/kits.go` (129) | Model data |
| `midi/` (~700) | OS port I/O + Launchpad |
| `tui/model.go` (271) | View (Bubbletea) |
| `widgets/`, `theme/`, `palettes/`, `debug/`, `config/`, `cmd/miditest/` | Infra, keep |

### Key structural mismatches to resolve

- **Schedule format**: current `{StartTick, Patterns []int}` → target `{Playing, Queued}` (breaks JSON; pre-approved).
- **No machine pattern in Model**: current = per-device queue filled/peeked/popped → target = events live only in PlaybackEngine's `current_events` / `next_events` (runtime state, not persisted, never in Model).
- **Transport/Tick out of Model**: current = various flags/fields in `State` and `Manager` → target = owned by PlaybackEngine (atomic types), not in `model.Project`.
- **State singleton**: current `sequencer.S` global → target `*model.Project` passed at construction.
- **Devices with fields**: current schedule+queue+callback+previewChan on device → target empty structs (`var Drum devices.Drum`). Pure compilers (`Compile(spec, seed)`), no stored state.
- **Locking**: current single global `Manager.mu` → target per-track `sync.RWMutex` on each device state.
- **Preview MIDI**: current preview chan + goroutine → target direct `out.Send()` in handler.
- **Probability**: current = not yet implemented consistently → target = rolled at compile time (fresh seed per compile → fresh rolls every loop).
- **Compile goroutine is new**: no equivalent today. Consumes `compileCh`, produces `next_events`. Separate from PlaybackEngine.

---

## Implementation plan

Each step ends buildable (`go build ./...` passes) and ideally runnable. Size: **S** ≤1h, **M** ~half-day, **L** ~day, **XL** multi-day.

### Step 1 — `midi/` at root + skeleton `model/` (M)

Two pieces, both near-mechanical:

**midi/ at repo root** (NOT inside controller/):
- Move `midi/event.go` in place (tweak: `Type uint8` → `Type EventType`).
- `midi/ports.go`: `Ports` struct implementing `ToExternal.Send` and `FromExternal.Subscribe` (64-buffer, oldest-drop on overflow).
- `midi/interfaces.go`: `ToExternal`, `FromExternal`.
- `midi/keyboard.go`: keyboard controller port input (carried over from current `midi/keyboard.go`).
- Leave `midi/launchpad.go` — Step 2 relocates to `controller/surface/`.

**model/ skeleton** (no machine fields):
- `model/project.go` — `Project{Tempo, Tracks}`, `DeviceKind`, `NumPatterns` const, `New()`, `Validate()`. NO Transport, NO Tick — those live in PlaybackEngine.
- `model/drum.go` — `Drum` (with `sync.RWMutex`), `DrumPattern{Notes, Length}`, `NoteLane`, `DrumStep`. No Machine field, no Events field.
- `model/piano.go` — `Piano`, `PianoPattern` (spec only).
- `model/metropolix.go` — `Metropolix`, `MetropolixPattern` (spec only), `PlaybackMode`, `ScaleType`.
- `model/schedule.go` — `Schedule{Playing int, Queued int}`.
- `model/kits.go` — drum kit maps.
- `model/time.go` — `PPQ` const (960), tick helpers.
- `model/persist.go` — *placeholder*; Save/Load move to `controller/project.go` in Step 4.

`sequencer/` still builds because it keeps its own types; `midi/` and `model/` are parallel but unreferenced.

**Verify**: `go build ./...` passes.

### Step 2 — `controller/surface/` (S)

- `controller/surface/surface.go`: `Surface` interface, `PadEvent`, `LED{R,G,B uint8}`.
- `controller/surface/launchpad.go`: port from current `midi/launchpad.go`. Uses `midi` at root for sending/receiving raw bytes.
- `controller/surface/mock.go`: MockSurface — injects pad events, records `SetLEDs` frames for test assertions.

No `controller/midi/` package — `midi/` at root serves both model and controller.

**Verify**: `go build ./... && go test ./controller/surface`.

### Step 3 — `controller/devices/` pure compilers (L)

Each device is an empty struct. The Compile method is the workhorse.

- `controller/devices/drum.go`:
  - `type Drum struct{}`
  - `Compile(spec *model.Drum, seed uint64) CompiledPattern` — pure function. For each active step in the current playing pattern, emit `TimedEvent{Tick, midi.Event}`. Uses Drum-specific logic (kit translation, velocities). Seed controls any random behavior (currently none for drum, but signature is uniform across devices).
  - `HandlePad(state *model.Drum, project *model.Project, out midi.ToExternal, row, col int)` — mutates spec; caller holds track lock.
  - `HandleKey(state, project, out, key)`, `HandleMIDI(state, out, event)` — same pattern.
  - `Render(state, project) string`, `RenderLEDs(state, project) []surface.LED`.
- `controller/devices/piano.go`: same shape. Compile produces TimedEvents per MIDI-style note (on at tick, off at tick+duration).
- `controller/devices/metropolix.go`: same shape. **Compile is the ONE place probability rolls happen** — using the provided seed + xorshift RNG. Accumulators evolve during compile.
- `controller/devices/session.go`, `settings.go`, `empty.go`: no `Compile` method (don't generate events). `HandlePad` still mutates state (e.g., Session sets `Schedule.Queued`).
- `controller/devices/save.go`: former SaveDevice. `HandleKey` delegates to `controller.SaveProject/LoadProject` — injected as callback to avoid import cycle.
- `controller/devices/kits.go` (OR in `model/kits.go` — decide during impl): drum kit maps. Kit note translation lives in `Drum.Compile` so TimedEvents carry pre-resolved MIDI notes.
- `controller/devices/rng.go`: xorshift64 PRNG struct. Devices' Compile methods use this — NOT `math/rand`.

Co-locate small boundary tests as each device lands.

**Verify**: `go build ./... && go test ./controller/devices/...`. Drum compile of a one-step pattern emits exactly one event at the right tick.

### Step 4 — `controller/playback.go` + `input.go` + `compile.go` + `project.go` (L)

Four files, each with a single responsibility.

**`controller/playback.go`** — tick loop + double-buffer:
- `type PlaybackEngine struct{ project, out, cursors, currentEvents[8], nextEvents[8], compileCh, lastGlobalTick, playing(atomic.Bool), t0(time.Time), tick(atomic.Int64), editCounters[8] }`.
- `type CompiledPattern struct{ Events []TimedEvent, Length int64, EditCounter uint64 }`.
- `type TimedEvent struct{ Tick int64, Event midi.Event }` — lives here, not in model.
- `Run(ctx)` spawns tick goroutine. On each tick: load `current_events`, walk for events in (prev, now], dispatch via `out.Send(track.PortName, track.Channel, ev)`. On wrap: atomic-swap current←next, emit `compileCh <- track` to request a new next.
- Wrap handling: if next is nil AND current is stale (EditCounter < editCounters[track]), sync-compile. Else reuse current.
- Delta clamping: if `delta > MAX_DELTA_TICKS`, clamp to 1.
- `Play() / Pause() / SetTempo(bpm)` — public methods for InputManager. Mutates `playing`, `t0`.

**`controller/compile.go`** — dedicated compile goroutine:
- `type Compiler struct{ project, editCounters*[8]atomic.Uint64, nextEvents*[8]atomic.Pointer[CompiledPattern], compileCh <-chan int, loopCounter atomic.Uint64 }`.
- `Run(ctx)`: for each trackIdx received on compileCh: read track.mu (RLock), dispatch by Track.Type to the right `devices.X.Compile(...)` with a fresh seed (`time.Now().UnixNano() ^ trackIdx ^ loopCounter.Add(1)`), record the current editCounter on the resulting CompiledPattern, atomic-store into `nextEvents[trackIdx]`.
- Log seed at DEBUG level for replay.

**`controller/input.go`** — sole writer:
- `type InputManager struct{ project, out, in, surface, focused, compileCh, editCounters*[8] }`.
- `Run(ctx)` fans in pad events (from surface), MIDI input (per-track goroutines subscribing via `midi.Subscribe`), and `HandleKey` calls from View.
- On any mutation to a track's spec/schedule: take track.mu.Lock, call `devices.<Kind>.HandleX(...)`, unlock, `editCounters[track].Add(1)`, `compileCh <- track`.
- Schedule mutations (Queue) also trigger `compileCh <- track` (so `next_events` rebuilds with the queued pattern's spec).
- Global hotkeys: `Space` calls `pe.Play()/Pause()`. `+/-` tempo. `S` save. Number keys focus. `,` Settings. `D` Save browser.

**`controller/project.go`** — save/load + project browser helpers:
- `SaveProject(p *model.Project, path string) error` — just `json.Marshal` + write.
- `LoadProject(path string) (*model.Project, error)` — read, unmarshal, `Validate()`. Does NOT recompile anything (PlaybackEngine's next compile signal handles it).
- `ListProjects/ListSaves/DeleteSave/RenameSave/DeleteProject/RenameProject` — used by `devices.Save`.
- `NewProject(name) *model.Project`.

Write an end-to-end smoke test: construct `*model.Project` with one drum pattern (one active step), hook up a mock ToExternal + mock compileCh that just calls Drum.Compile synchronously, run `pe.Step(n)` (test-only), assert the expected event was emitted in the right tick range.

**Verify**: `go build ./... && go test ./controller/...`.

### Step 5 — Rewire `main.go` + `view/` on top of new controller (M)

- `view/tui.go`: Bubbletea Model imports only `controller` + `theme`. Sends input to `InputManager.HandleKey`. Rendering via Manager accessors (`m.Focused()`, `m.RenderFocused()`) that take per-track RLock internally.
- `view/widgets/`: carry over from current `widgets/`.
- Rewrite `main.go` with the wiring:
```go
func main() {
    ctx := context.Background()
    project := controller.LoadProject(...) or controller.NewProject("untitled")

    ports, _ := midi.NewPorts()
    surface, _ := surface.OpenLaunchpad(ports)   // or surface.NewMock() with --no-hardware

    compileCh := make(chan int, 16)

    pe := controller.NewPlaybackEngine(project, ports, compileCh)
    im := controller.NewInputManager(project, ports, ports, surface, compileCh, pe)
    compiler := controller.NewCompiler(project, pe.NextEvents(), pe.EditCounters(), compileCh)

    go pe.Run(ctx)
    go im.Run(ctx)
    go compiler.Run(ctx)

    // Initial compile trigger: one per track so next_events gets populated
    for i := range project.Tracks { compileCh <- i }

    p := tea.NewProgram(view.New(im, project, theme))
    p.Run()
}
```

**Verify**: `go build ./...`, launch the app, edit a drum step, press play, confirm tick advances + event fires on expected tick.

### Step 6 — Delete `sequencer/`, `tui/` (S)

Remove old packages outright. `midi/` stays (we moved it there in Step 1). Keep `widgets/` (merged into `view/widgets/`), `theme/`, `palettes/`, `config/`, `debug/`, `cmd/miditest/`.

`git rm -r sequencer/ tui/`.

**Verify**: `go build ./... && go vet ./...`. `grep -r "sequencer" .` returns only docs / git history.

### Step 7 — Boundary tests (M)

Consolidate tests at module boundaries:

- `model/project_test.go`: JSON round-trip; Validate clamps (out-of-range fields snap to valid); length=0 guard.
- `controller/devices/drum_test.go`: Compile empty → 0 events; Compile with one active step → 1 TimedEvent at right tick/velocity; HandlePad toggle mutates spec. Deterministic (same seed → same events).
- `controller/devices/metropolix_test.go`: same-seed determinism; different-seed non-determinism (probability rolls); scale-quantized note at right tick.
- `controller/playback_test.go`: MockSurface + mock ToExternal + injectable clock. Two-track project, drive `pe.Step(n)`, assert exact ordered sequence of `Send` calls. Schedule promotion test (Queue a pattern, wrap, verify it plays).
- `controller/compile_test.go`: Send trackIdx on compileCh, verify `nextEvents[trackIdx]` gets populated with non-nil CompiledPattern. EditCounter matches.
- `controller/input_test.go`: pad routing; MIDI input to recording-armed drum increments editCounter; focus changes.

`controller/internal/testkit`: `NewTestProject()`, `NewMockPorts()`, `NewMockSurface()`, `NewFakeClock()`.

**Verify**: `go test ./... -race` passes, nothing flaky.

---

## Testing strategy

| Package | Boundary tests | Landed in step |
|---|---|---|
| `model/` | JSON round-trip, Validate clamps, length-0 guard | 1 (stubs) / 7 (finalize) |
| `midi/` | Send records gomidi message via test seam; Subscribe demuxes by port+channel, drops oldest on overflow | 1 |
| `controller/surface/` | MockSurface frame recording, pad event injector | 2 |
| `controller/devices/*` | One Compile (deterministic given seed) + one HandlePad per device; probability tests for Metropolix | 3 |
| `controller/playback.go` | End-to-end with MockToExternal, Step(n), assert event log; Schedule promotion; stale-reuse fallback | 4, 7 |
| `controller/compile.go` | Send trackIdx → nextEvents populated + EditCounter correct | 4, 7 |
| `controller/input.go` | Pad routing, MIDI recording, focus | 7 |
| `controller/project.go` | Save→Load→DeepEqual; Load doesn't recompile (that's compile goroutine's job) | 4, 7 |
| `view/` | Manual only | n/a |

---

## Risks & gotchas

1. **Schedule format is a breaking change.** Current `{StartTick, Patterns []int}` → `{Playing, Queued}`. Don't attempt auto-migration; `Validate()` rejecting old shape is fine.
2. **PPQ=960 compile-time.** `const PPQ = 960` in `model/time.go`. Don't parameterize.
3. **No runtime state in Model.** Transport/Tick moved to `controller/playback.go`. Input calls `pe.Play()`/`pe.Pause()`/`pe.SetTempo()`. Old design drew these on `Project` — that's wrong now.
4. **Track device is a union.** `Track{Drum *Drum, Piano *Piano, Metropolix *Metropolix}` with "exactly one non-nil" rule enforced in `Validate()` given `Track.Type`. Zero-value Track (Type==None) is valid and skipped by tick/render.
5. **CompiledPattern never persisted.** Lives only in `PlaybackEngine.currentEvents/nextEvents`. No JSON tags needed. Regenerated via compileCh signals.
6. **EditCounter invariant.** InputManager MUST increment `editCounters[track]` after every mutation that affects generated events (spec edits, queue, MIDI recording). Compiler stamps the counter it saw into CompiledPattern. At wrap, playback compares: if current.EditCounter < editCounters[track].Load() AND next is nil → sync-compile; else reuse.
7. **Compile must be pure.** `Compile(spec, seed)` — no side effects, no mutation of spec, no global state. Same inputs → same output. All randomness driven by seed.
8. **Preview MIDI is direct.** `Drum.HandlePad` calls `out.Send(...)` directly when `state.Recording == false`. No channel, no goroutine, no Intent.
9. **Slice-backing-array invariant.** When input mutates a pattern's spec, other readers (Compiler goroutine) might be reading the same Track's mu-protected state. Use per-track `sync.RWMutex` (Lock for writes, RLock for reads). When Compiler produces a new CompiledPattern, it always allocates a fresh `[]TimedEvent` — never mutates a slice that current_events might still point at.
10. **Import cycle risk.** `devices.Save` needs to call `controller.SaveProject/LoadProject`. Mitigation: inject as callback parameter from InputManager. Cleaner than a `projectio` split.
11. **Focus mutation.** `FocusTarget` mutated only by InputManager's own goroutine. No lock needed. View reads via `im.Focused() FocusTarget` (value copy).
12. **Kits.** `Track.Kit` is "" for new tracks; `GetKit` defaults to GM. Kit note translation in `devices.Drum.Compile` — TimedEvents carry pre-resolved MIDI notes. Eliminates the odd `Note < 16` branch in today's midi output loop.
13. **NoteInput port.** Today's `State.NoteInputPort` single string → gone. MIDI in is per-track via `Track.PortName + Track.Channel` per DESIGN 4.7. Settings device configures each track.
14. **Seed quality.** Use xorshift64 (`controller/devices/rng.go`), not `math/rand`. Seed via `time.Now().UnixNano() ^ trackIdx ^ loopCounter`. Log seeds at DEBUG for reproducing accidents.
15. **RtMidi C++ warnings.** Pre-existing, ignore.

---

## Open questions

These are the ones still unresolved after the Grok design review:

1. **`controller/devices/save.go` → `controller.SaveProject` import cycle.** Solution proposed: inject `saveProject func(*model.Project, string) error` as a construction parameter. Needs a concrete decision during Step 3.
2. **MockSurface.PushPad ergonomics**: synchronous (no-buffer, blocks until consumed) or buffered? Lean synchronous for test determinism.
3. **Settings → reconnect MIDI port.** Settings changes `Track.PortName/Channel`; existing per-track MIDI subscription goroutine needs to restart on change. `midi.Ports` needs `Resubscribe(port, channel)` or the input manager needs a "restart my per-track subscriber" signal.
4. **Quick-save target when `ProjectName == ""`**: save to `"untitled"` as today's behavior, or prompt? Low-priority UX call.
5. **Keep `cmd/miditest/`?** Assume yes as a dev utility.
6. **Per-track compile goroutines vs. one shared.** Plan uses one shared goroutine draining `compileCh`. If profiling shows compile-rate burstiness is a problem, upgrade to a worker pool. Defer unless we see it.

---

## Time estimate

Solo evenings/weekends: **1.5–2.5 weeks**. Tight but realistic.

Breakdown:
- Step 1: half a day (type moves + midi/ re-home, mechanical)
- Step 2: couple hours (surface interface + Launchpad port + Mock)
- Step 3: ~2 days (porting device logic to pure compilers + seed plumbing; Metropolix is the longest)
- Step 4: ~1 day (playback + compile goroutine + input; new code)
- Step 5: half a day (main.go wiring + view)
- Step 6: minutes (git rm)
- Step 7: 1 day (tests against clean boundaries)

The novel engineering is concentrated in Step 4 (double-buffer wiring, editCounter invariant, compile goroutine). Step 3 is largely mechanical simplification (devices get ~40% smaller as queue/dirty/callback plumbing evaporates). Steps 1, 2, 5, 6 are routine.
