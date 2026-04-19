# go-sequence refactor: implementation plan

Ordered, sized plan for restructuring the project from the current `sequencer/`
monolith into the MVC shape described in `DESIGN.md`. Produced by a Plan agent
on 2026-04-19 after design was locked.

Read `DESIGN.md` first — it is the source of truth for the target architecture.
This document answers "in what order do we build it."

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
- **Machine pattern**: current = per-device queue filled/peeked/popped → target = `[]TimedEvent` stored on each pattern in Model.
- **State singleton**: current `sequencer.S` global → target `*model.Project` passed at construction.
- **Devices with fields**: current schedule+queue+callback+previewChan on device → target empty structs (`var Drum devices.Drum`).
- **Locking**: current single global `Manager.mu` → target per-track `sync.RWMutex` on each device state.
- **Preview MIDI**: current preview chan + goroutine → target direct `out.Send()` in handler.

---

## Implementation plan

Each step ends buildable (`go build ./...` passes) and ideally runnable. Size: **S** ≤1h, **M** ~half-day, **L** ~day, **XL** multi-day.

### Step 1 — Skeleton directories + `model/` package (M)

Create target directories. Move data-only types out of `sequencer/state.go`:
- `model/project.go` — `Project`, `Transport`, `Track`, `DeviceKind`, `PPQ` const, `NumPatterns` const, `New()`, `Validate()`.
- `model/drum.go` — `Drum` (with `sync.RWMutex`), `DrumPattern{Human, Machine}`, `DrumHuman{Notes [16]NoteLane}`, `NoteLane`, `DrumStep`, validate.
- `model/piano.go` — `Piano`, `PianoPattern`, `PianoHuman`, machine side, validate.
- `model/metropolix.go` — `Metropolix`, `MetropolixPattern{Human, Machine}`, `MetropolixHuman`, `PlaybackMode`, `ScaleType`, `ScaleCount`, validate.
- `model/schedule.go` — `Schedule{Playing int, Queued int}`.
- `model/event.go` — `TimedEvent{Tick int64, Event midi.Event, StageID int, Probability uint8}`.
- `model/transport.go` — `Transport{Playing atomic.Bool, T0 time.Time}`, `Project.Tick atomic.Int64`.

`sequencer/` still builds because it keeps its own types; `model/` is parallel but unreferenced.

**Verify**: `go build ./...` passes.

### Step 2 — `controller/midi/` and `controller/surface/` (M)

- Move `midi/event.go` → `controller/midi/event.go` (rename `Type uint8` → `Type EventType`).
- `controller/midi/ports.go`: `Ports` struct. Implements `ToExternal.Send` and `FromExternal.Subscribe` (64-buffer, oldest-drop on overflow).
- `controller/midi/interfaces.go`: `ToExternal`, `FromExternal`.
- `controller/surface/surface.go`: `Surface` interface, `PadEvent`, `LED{R,G,B uint8}`.
- `controller/surface/launchpad.go`: port from `midi/launchpad.go`.
- `controller/surface/mock.go`: MockSurface — injects pad events, records `SetLEDs` frames for test assertions.

Keep old `midi/` until step 6.

**Verify**: `go build ./...`, `go test ./controller/midi ./controller/surface`.

### Step 3 — `controller/devices/` stateless singletons (L)

For each device, `type Drum struct{}` with methods operating on pointers into `*model.Project`.

- `controller/devices/drum.go`: `Compile(*model.DrumHuman) *model.MachinePattern`, `HandlePad`, `HandleKey`, `HandleMIDI`, `Render`, `RenderLEDs`.
- `controller/devices/piano.go`, `metropolix.go`, `session.go`, `settings.go`, `empty.go`: same shape.
- `controller/devices/save.go`: former SaveDevice. `HandleKey` delegates to `controller.SaveProject`/`LoadProject` (injected as callback to avoid import cycle).
- `controller/devices/kits.go`: ported from `sequencer/kits.go`.

Co-locate small boundary tests as each device lands.

**Verify**: `go build ./... && go test ./controller/devices/...`.

### Step 4 — `controller/playback.go` + `controller/input.go` + `controller/project.go` (L)

- `controller/playback.go`: `PlaybackEngine{project, cursors, out}`. `Run(ctx)` spawns clock goroutine per DESIGN 4.6. Tick function per DESIGN 4.1.
- `controller/input.go`: `InputManager{project, out, in, surface, focused}`. `Run(ctx)` fans in: surface pad events, MIDI in subscriptions, view keyboard events. All writes under per-track `Lock`.
- `controller/project.go`: `SaveProject(*model.Project, path) error`, `LoadProject(path) (*model.Project, error)`. Load stops playback, Validate, recompile all patterns.
- `controller/projectlist.go`: `ListProjects/ListSaves/DeleteSave/RenameSave/DeleteProject/RenameProject` for the Save device.

Write an end-to-end smoke test: construct `*model.Project` with one drum pattern (one active step), hook up a mock ToExternal, run PlaybackEngine via a test-only `Step(n)` method, assert expected event emitted.

**Verify**: `go build ./... && go test ./controller/...`.

### Step 5 — Rewire `main.go` + `view/` on top of new controller (M)

- `view/tui.go`: Bubbletea Model imports only `controller` + `theme`. Input flows to `InputManager.HandleKey`. Rendering via `controller.Snapshot()` helpers that take per-track RLock internally.
- Rewrite `main.go`:
  1. Load config.
  2. Build `*model.Project`.
  3. `ports, _ := controllermidi.NewPorts()`.
  4. `surface, _ := surface.OpenLaunchpad(ports)` (or MockSurface for `--no-hardware`).
  5. `pe := controller.NewPlaybackEngine(project, ports); go pe.Run(ctx)`.
  6. `im := controller.NewInputManager(project, ports, ports, surface); im.Run(ctx)`.
  7. `p := tea.NewProgram(view.New(im, project, theme))`.

**Verify**: `go build ./...`, launch the app, edit a drum step, press play, confirm tick advances.

### Step 6 — Delete `sequencer/`, `midi/`, `tui/` (S)

Remove old packages outright. Keep `widgets/`, `theme/`, `palettes/` — they're infra, not MVC-bounded. No compat shim, no migration. `git rm -r sequencer/ midi/ tui/`.

**Verify**: `go build ./... && go vet ./...`. `grep -r "sequencer" .` returns only docs.

### Step 7 — Boundary tests: MockSurface + mock ToExternal/FromExternal (M)

Consolidate tests:
- `model/project_test.go`: JSON round-trip; Validate clamps; length=0 guard.
- `controller/devices/drum_test.go`: Compile empty → 0 events; Compile with active step → 1 TimedEvent at right tick/velocity; HandlePad toggle then inspect `Machine.Events`.
- `controller/devices/metropolix_test.go`: one stage gate on → Compile → scale-quantized note at right tick.
- `controller/playback_test.go`: MockSurface + mock ToExternal. Two-track project, drive `pe.Step(n)`, assert exact ordered sequence of `Send` calls. Schedule promotion test.
- `controller/input_test.go`: pad routing; MIDI input to recording-armed drum; focus changes.

`controller/internal/testkit` helper: `NewTestProject()`, `NewMockPorts()`, `NewMockSurface()`.

**Verify**: `go test ./...` passes, nothing flaky.

---

## Testing strategy

| Package | Boundary tests | Landed in step |
|---|---|---|
| `model/` | JSON round-trip, Validate clamps, length-0 guard | 1 (stubs) / 7 (finalize) |
| `controller/midi/` | Send records gomidi message via test seam; Subscribe demuxes by port+channel, drops oldest on overflow | 2 |
| `controller/surface/` | MockSurface frame recording, pad event injector | 2 |
| `controller/devices/*` | One Compile + one HandlePad per device | 3 |
| `controller/playback.go` | End-to-end with MockToExternal, Step(n), assert event log; Schedule promotion | 4, 7 |
| `controller/input.go` | Pad routing, MIDI recording, focus | 7 |
| `controller/project.go` | Save→Load→DeepEqual; Load recompiles; Load stops playback | 4, 7 |
| `view/` | Manual only | n/a |

---

## Risks & gotchas

1. **Schedule format is a breaking change.** Current `{StartTick, Patterns []int}` → `{Playing, Queued}`. Don't attempt auto-migration; `Validate()` rejecting old shape is fine.
2. **PPQ=960 compile-time.** `const PPQ = 960` in `model/project.go`. Don't parameterize.
3. **Concurrency primitives on Transport.** `atomic.Bool` for `Transport.Playing`, `atomic.Int64` for `Project.Tick`. Not JSON-tagged (runtime only). `T0` is plain `time.Time`, written only when Playing transitions, read only by tick goroutine — no lock.
4. **Track device is a union.** `Track{Drum *Drum, Piano *Piano, Metropolix *Metropolix}` with "exactly one non-nil" rule enforced in `Validate()` given `Track.Type`. Zero-value Track (Type==None) is valid and skipped by tick/render.
5. **MachinePattern.Events is `json:"-"`.** Regenerated on Load. Don't bother with custom marshaling.
6. **Preview MIDI.** Today's `previewChan` + `handlePreviewEvents` goroutine → gone. Per DESIGN 4.4, `Drum.HandlePad` calls `out.Send(...)` directly when `state.Recording == false`.
7. **Slice-backing-array immutability invariant** (DESIGN §3.6). Every HandlePad that touches `Machine.Events` must do `state.Patterns[i].Machine = Drum{}.Compile(...)` (whole replace), never append/index-assign an existing slice. Review-checklist item.
8. **Import cycle risk.** `devices.Save` needs to call `controller.SaveProject/LoadProject`. Mitigation: inject as callback parameter from InputManager at construction time. Cleaner than a `projectio` split.
9. **RtMidi C++ warnings.** Pre-existing, ignore.
10. **Focus mutation.** `FocusTarget` mutated only by InputManager's own goroutine (DESIGN 4.8). No lock needed. View reads via `im.Focused() FocusTarget` (value copy).
11. **Kits.** `Track.Kit` is "" for new tracks; `GetKit` defaults to GM. Kit note translation belongs in `devices.Drum.Compile` — each drum TimedEvent's `Event.Note` is pre-resolved. Eliminates the odd `Note < 16` branch in today's `midiOutputLoop`.
12. **NoteInput port.** Today's `State.NoteInputPort` single string → gone. MIDI in is per-track via `Track.PortName + Track.Channel` per DESIGN 4.7. Settings device configures each track.

---

## Open questions

1. **Where does `midi.Event` live?** `model.TimedEvent` uses `midi.Event`, implying `model` imports `controller/midi`. Crosses MVC boundary if strict. Alternatives: (a) define minimal `model.MidiEvent` with `controller/midi.Event = model.MidiEvent`; (b) accept the dep since `controller/midi` is effectively the wire-format package. Lean (a). Needs decision before Step 1.
2. **MachinePattern.Events size ceiling?** Piano with many notes could balloon. Not correctness; memory note only.
3. **Save device vs. global save hotkey.** Keep `S` as global quick-save AND `D`-focused browser view. Save device = browser/focus target, not only save path.
4. **MockSurface.PushPad ergonomics**: synchronous (no-buffer) or buffered? Lean synchronous for test determinism.
5. **Settings → reconnect MIDI port.** Settings changes `Track.PortName/Channel`; `Ports` needs `Resubscribe(track, portName, channel uint8)`. Needs small interface extension.
6. **Quick-save target when `ProjectName == ""`**: save to `"untitled"` as today's behavior, or prompt for name? Low-priority.
7. **Keep `cmd/miditest/`?** Not mentioned in DESIGN. Assume yes as a dev utility; already doesn't depend on `sequencer/`.

---

## Time estimate

Solo evenings/weekends: **1.5–2.5 weeks**. Tight.

Breakdown:
- Steps 1, 2: half a day each (mechanical type moves)
- Step 3: largest single step, ~2 days (porting 3,400+ lines of device logic, but simplifying as we go — net ~40% smaller)
- Step 4: ~1 day (new code: tick loop, input fan-in, SaveProject/LoadProject)
- Step 5: half a day
- Step 6: minutes (git rm)
- Step 7: 1 day

Per DESIGN principles, devices get *simpler* in this refactor (queue/dirty/callback plumbing evaporates). The novel engineering is in PlaybackEngine's tick loop (clock math, cursor modular wrap, boundary promotion, probability at dispatch) — that's the part that deserves care.
