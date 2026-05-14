# Architecture history

This file tracks the patterns the codebase has actually used over time —
why each was tried, why each was dropped, and where to find the code that
embodied it. Newest first.

## Pattern 2 — pure MVC, free functions, model owns all state (current)

Status: live. First commit: `196e1bd` (refactor step 1, 2026-04-19).
Current HEAD: `4a9a677` and beyond.

Three layers, parallel paths per domain:
- `model/` — state. Devices (Drum/Piano/Metropolix) and system widgets
  (Session/Settings/Project) live here. UIState is a model field too —
  per DESIGN: "model owns all state."
- `controller/` — free functions on state. No structs, no methods. The
  big background routines are `RunPlayback`, `RunPatternCompiler`, plus
  per-domain mutator functions called from view input handlers.
- `view/` — three surfaces. `view/launchpad/` (pads + LEDs),
  `view/tui/` (terminal text), `view/keyboard/` (MIDI-in routing).
  Each captures input, mutates the model directly, calls
  `MarkTrackDirty` to wake the compiler.

Source of truth: `DESIGN.md` at repo root.

Why pure MVC: prior managers + device interfaces were doing two jobs
each (state + behavior) and lock granularity was unclear. Splitting
state from behavior made the lock story trivial (per-track RWMutex,
held by view handlers, released before compile reads).

## Pattern 1 — Manager + Device interface (legacy, abandoned)

Status: removed in `a945b09` (2026-04-19, "delete old sequencer/ and tui/").
Last commit on this pattern: `0679146` (2026-01-28, "many").

Stack:
- `sequencer/` package held a top-level `Manager` struct that owned the
  clock, the eight devices, and routed MIDI to ports.
- A `Device` interface with `View()`, `HandleKey()`, `Tick()`, etc.
  Concrete types: `DrumDevice`, `PianoRollDevice`, `MetropolixDevice`,
  `SessionDevice`, `SettingsDevice`, `EmptyDevice`.
- Each device held its own UI state (cursor, selected note, popup) as
  struct fields. `Manager` held the project-level state (tracks,
  device assignments, MIDI port configs).
- TUI and Launchpad both called into `Device.View()` / `Device.HandleKey()`.

Why it was tried: matched the way users *think* about devices —
"the drum machine" is a thing, give it a Go type. Made the per-device
keyboard map an obvious method.

Why it was dropped:
- State was spread across Manager, Device structs, and a singleton
  global (`S` in `sequencer/state.go`). Hard to reason about.
- Methods on Device couldn't agree on lock granularity — some
  mutated through the interface, some bypassed it.
- Save/load had to round-trip through method calls instead of
  serializing one struct.

The original markdown notes for Pattern 1 are preserved verbatim in
`ARCHITECTURE/legacy/`. Do not treat them as accurate to current
code — they describe the abandoned manager/device layout.

## Pattern 0 — early scrappy single-file (legacy, abandoned)

Status: pre-Pattern-1 prototyping. Commits `5407e9b` … `4cb6535`
(2026-01-12 / 13).

A single `main.go` + a few helpers, hardcoded paths from "key press"
through to "MIDI out". No real structure, just enough to make sound.
Scaffolding for Pattern 1.
