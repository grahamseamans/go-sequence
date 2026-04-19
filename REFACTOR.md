# go-sequence: refactor plan

State of thinking after the 2026-04-18 session. Resume from here.

## Product thesis

Free, software clone of the expensive hardware sequencers (Cirklon $2,099, Hapax $1,299, Pyramid ~$700, Metropolix $579, Torso T-1 $699), driven by a Novation Launchpad most people already own.

No good free version of this kind of tool exists. Hardware vendors won't build it. DAWs solve a different problem.

Reference hardware, NOT DAWs. This is a MIDI brain, not a DAW.

## UI direction: Wails

Keep the Go core. The 5-goroutine architecture (UI tick, LED tick, MIDI input, MIDI output, queue manager) gives sub-5ms MIDI jitter — that's the timing we'd lose by going pure-web. Swap **Bubbletea → Wails** (Go backend + HTML/JS UI in a system webview). Single ~20MB binary. ~3-6 weeks evenings/weekends.

Considered and rejected:
- **Pure web (Web MIDI API)**: 10-50ms jitter, JS event loop is the clock — wrong for a sequencer.
- **Rust rewrite**: midir is the same RtMidi-based design as gomidi; no material gain. Hot-plug pain is identical.
- **C++ rewrite (Dear ImGui or Qt)**: 8-16 weeks. ImGui state model gets ugly with multi-device complexity. Qt distribution is bad (50MB+ installers). Throws away working code for marginal gain.
- **JUCE**: overkill, plugin-hosting infrastructure is dead weight for non-VST app.

## Architecture: MVC

```
model/        — state structs, validation, save/load. Pure data.
view/         — Bubbletea now → Wails later. Reads Model, sends events to Controller.
controller/   — the entire engine.
    midi/       — OS MIDI port management, send/receive bytes
    surface/    — Control Surface interface + Launchpad X impl + Mock for tests
    devices/    — drum, pianoroll, metropolix, session, settings, empty
    manager.go  — tick loop, queue, dispatch, MIDI routing
```

Three top-level chunks. MIDI I/O and Control Surface live *inside* Controller because they're capabilities the Controller uses to talk to the outside world, not peer chunks.

"Control Surface" (DAW term) NOT "Controller" — the latter is reserved for its MVC meaning. The Launchpad is a Control Surface.

## Principles

- **One source of truth for state, no parallel save format.** Model is whatever lives in `model/` — can be one struct or many. Save = serialize the model package. The constraint is "no second copy of the data living elsewhere," not "literally one struct."
- **Devices are pure Controllers. They have no state.** They take input, read from Model, mutate Model, emit MIDI events. The `DrumState` struct lives in `model/`; the `DrumDevice` (in `controller/devices/`) is just behavior that operates on it. They share a name by naming convention, not by ownership.
- **Test at module boundaries**, not internals. Each chunk gets `*_test.go` exercising the public API. Mock implementations (MockSurface, fake MIDI port) let the engine run end-to-end with no hardware.
- **Data flow**: Inputs (clock, keyboard via View, pads via Surface, MIDI in) → Controller → Model mutations → Views (screen, LEDs, MIDI out).
- **Naming**: Same domain name in both packages, role conveyed by package path. `model.Drum` is the data, `controller/devices.Drum` is the behavior. No `Device`/`State`/`Controller` suffixes — Go's anti-stutter convention plus our packages already say what role each type plays. Reads as `func (d *devices.Drum) Tick(state *model.Drum) []midi.Event`.

## Audit punch list

### Critical (refactor-blocking)

1. **Finish schedule refactor in piano + metropolix.** Direction is correct (Schedule struct = Model, queue derivation = Controller). Currently ~75% done in those two devices; not yet applied to drum/session/settings/empty.
2. **Apply schedule pattern to remaining devices** for interface consistency.

(Note: audit agent flagged `LoadProject()` as a stub — verified 2026-04-19, it's fully implemented at `project.go:161-216`. Agent was wrong.)

### High (pre-merge)

- Split Device interface — recording methods only used by 3/7 devices. Wants `SequencerDevice` + `RecordableDevice`.
- Move `midi/*.go` → `controller/midi/`.
- Extract Control Surface from `midi/launchpad.go` + `manager.go`'s LED loop → `controller/surface/`.
- Extract duplicated schedule helpers (drum/piano/metropolix) → one `ScheduleManager` utility.
- Fix `sequencer/save.go:96` TODO (silent error swallow).
- Stop type-asserting devices in `manager.go` to wire callbacks; pass at construction.

### Medium (polish, post-refactor)

- Inject `*State` into devices instead of global `S` singleton — needed for multi-state testing.
- Extract Launchpad color mapping into `controller/surface/launchpad.go`. Devices return semantic colors; surface maps to hardware. Reduces RenderLEDs duplication across devices.
- Clarify "MIDI event" (hardware) vs "sequencer event" (queued) terminology.

## Tangles worth knowing

- **`manager.go` is 810 lines** — does device lifecycle, MIDI dispatch, queue filling, MIDI input, LED rendering. Internally well-structured (separate goroutines), but wants splitting along MVC sub-package lines.
- **Devices reach back into Manager** via `manager` pointer (e.g., Session). Should be passed callbacks at construction.
- **LED color logic duplicated** across each device's RenderLEDs.

## Test strategy when we get there

- `model/` — round-trip serialize/deserialize/equal. Validation clamps corrupt fields.
- `controller/midi/` — fake port, send/receive bytes.
- `controller/surface/` — Mock impl, exercise interface.
- `controller/devices/{drum,piano,metropolix,...}` — load known state, tick N, assert emitted events.
- `controller/manager.go` — load known state, run tick loop with MockSurface + fake MIDI port, assert events routed correctly.
- `view/` — manual.

## Next session

1. Decide order: finish the in-flight schedule refactor first, or restructure the directory layout first? My (Claude's) instinct: schedule refactor first, since it's already in flight and won't get easier with time. Directory restructure can ride along.
2. Plan stage — turn this punch list into a sized, ordered roadmap with concrete steps.
