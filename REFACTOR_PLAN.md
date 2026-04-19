# go-sequence refactor: implementation plan

Ordered, sized plan for restructuring the project from the current `sequencer/`
monolith into the MVC shape described in `DESIGN.md`.

Read `DESIGN.md` first — it is the source of truth for the target architecture.
This document answers "in what order do we build it."

**Revised 2026-04-19 (second pass):** earlier pass committed Steps 1-4 with
"device" Go types that bundled state + Compile + HandlePad + Render methods
into one struct. That's a layer-violation — devices aren't Go types,
they're conceptual entities split across model/controller/view at parallel
paths. This revision adds a restructuring step and reshapes Steps 3 and 5.

**Revised 2026-04-19 (third pass):** doubled down on the principle. Model
owns ALL state, including runtime state (playback cursors/events, compile
channel, UI focus). Controller is JUST bags of functions — no struct
types anywhere in controller. `PlaybackEngine struct` / `Compiler struct` /
`InputManager struct` all become `func RunPlayback(ctx, project)` etc.
No "manager" types. View handles UI input (pad/keyboard) and rendering;
controller handles external MIDI input. Input never mutates state
directly — always goes through controller mutator functions in
`controller/devices/<kind>.go`. Check DESIGN.md for current canonical
architecture; this plan's step contents may trail.

---

## Verified current state (as of 2026-04-19, after first pass)

`go build ./...` passes. Steps 1, 2, 3a-e, 4 committed.

**What's correct already:**
- `model/` skeleton with state types
- `midi/` at repo root with Event/EventType/Ports
- `controller/surface/` with Launchpad + Mock
- `controller/playback.go` — PlaybackEngine + double-buffer
- `controller/compile.go` — Compiler goroutine
- `controller/input.go` — InputManager
- Per-track RWMutex
- compileCh + editCounters
- Schedule = {Playing, Queued}

**What needs restructuring:**
- `controller/devices/<kind>.go` files contain `type Drum struct{}` etc. with methods like `Compile`, `HandlePad`, `Render`, `RenderLEDs` all on the struct. Smushes controller and view concerns into one type. Needs splitting.
- `controller/project.go` exists for save/load, but should live at `controller/system/project.go`. The Save UI also needs to move from `controller/devices/save.go` to `view/system/project.go`.
- Settings, Session, Empty similar — they're system widgets, not devices. Move from `controller/devices/` to `controller/system/` (functions) + `view/system/` (UI).
- View doesn't exist as a real package yet — currently `tui/` is empty / unported.

---

## Implementation plan (restructured)

Each step ends buildable. Sizes: **S** ≤1h, **M** ~half-day, **L** ~day, **XL** multi-day.

### Step 1 — Already done ✓

`midi/` at root + `model/` skeleton committed. Some adjustments needed:
- Check `model/` has `model/devices/` and `model/system/` subdirs (move drum/piano/metropolix into `model/devices/`; add `model/system/project.go` etc. for system-widget state if there's any beyond what's in the main Project struct).

### Step 2 — Already done ✓

`controller/surface/` with Launchpad + Mock.

### Step 3 — RESTRUCTURE: split "device" structs into per-layer free functions (L)

The in-container Claude built `controller/devices/drum.go` with `type Drum struct{}` and methods. Restructure each device:

For each musical device (drum, piano, metropolix):

**3a. Move state into `model/devices/`** if not already there.
```go
// model/devices/drum.go
package devices  // or each its own subpackage

type Drum struct {
    Patterns [16]DrumPattern
    Schedule Schedule
    // ...
}
```

**3b. Replace `controller/devices/<kind>.go` smushed type with free functions.**

Currently:
```go
type Drum struct{}
func (Drum) Compile(state *model.Drum, seed uint64) CompiledPattern { ... }
func (Drum) HandlePad(state *model.Drum, ...) { ... }
func (Drum) Render(state *model.Drum, ...) string { ... }
```

Become:
```go
// controller/devices/drum.go
package drum   // or package devices with subdirs

func Compile(state *devices.Drum, seed uint64) controller.CompiledPattern { ... }
func ToggleStep(state *devices.Drum, lane, step int) { ... }
func ClearPattern(state *devices.Drum, idx int) { ... }
// no Handle*, no Render — those move to view/
```

**3c. Move Render/RenderLEDs into `view/devices/<kind>.go`.**
```go
// view/devices/drum.go
package drumview   // or appropriate

func Render(state *devices.Drum, project *model.Project) string { ... }
func RenderLEDs(state *devices.Drum, project *model.Project) []surface.LED { ... }
func HandlePad(state *devices.Drum, project *model.Project, row, col int) {
    // interpret gesture in drum context, call controller/devices/drum mutators
    drum.ToggleStep(state, lane, step)
}
func HandleKey(state *devices.Drum, project *model.Project, key string) { ... }
func HandleMIDI(state *devices.Drum, ev midi.Event) { ... }
```

For each system widget (project/save, settings, session, empty):

**3d. Move state to `model/system/<kind>.go`** if any per-widget state exists.
**3e. Functions go to `controller/system/<kind>.go`.** SaveProject/LoadProject etc. move out of `controller/project.go` into `controller/system/project.go`.
**3f. UI goes to `view/system/<kind>.go`.**

**Verify:** `go build ./...` passes; manual smoke test of structure (no Device interface, no struct types in controller/devices files).

**Dependencies:** Steps 1, 2, current 3a-e, 4 work outputs.

### Step 4 — Restructure InputManager dispatch (M)

Current InputManager dispatches via the Device interface (calling `device.HandlePad(...)` on the interface). Replace with type-switch dispatch:

```go
func (im *InputManager) HandlePad(row, col int) {
    switch im.focused.Kind {
    case FocusTrack:
        track := im.project.Tracks[im.focused.Track]
        track.mu.Lock()
        switch track.Type {
        case model.DeviceDrum:       drumview.HandlePad(track.Drum, im.project, row, col)
        case model.DevicePiano:      pianoview.HandlePad(track.Piano, im.project, row, col)
        case model.DeviceMetropolix: metroview.HandlePad(track.Metropolix, im.project, row, col)
        }
        track.mu.Unlock()
        im.editCounters[im.focused.Track].Add(1)
        im.compileCh <- im.focused.Track
    case FocusSave, FocusSession, FocusSettings:
        // route to view/system/<kind>.HandlePad
    }
}
```

Same for HandleKey, HandleMIDI.

The Compiler goroutine similarly type-switches:
```go
switch track.Type {
case model.DeviceDrum:       events = drum.Compile(track.Drum, seed)
case model.DevicePiano:      events = piano.Compile(track.Piano, seed)
case model.DeviceMetropolix: events = metropolix.Compile(track.Metropolix, seed)
}
```

No interface anywhere. Clean type-switch dispatch.

**Verify:** `go build ./...`; existing tests still pass.

**Dependencies:** Step 3 restructure.

### Step 5 — Wire main.go + view/ on top of restructured controller (M)

- `view/tui.go`: Bubbletea Model. Routes input to InputManager. Calls per-domain Render/RenderLEDs based on focus. Calls `surface.SetLEDs`.
- Rewrite `main.go`:
```go
func main() {
    ctx := context.Background()
    project, err := system.LoadProject(...)  // or system.NewProject()
    
    ports, _ := midi.NewPorts()
    surf, _ := surface.OpenLaunchpad(ports)
    compileCh := make(chan int, 16)
    
    pe := controller.NewPlaybackEngine(project, ports, compileCh)
    im := controller.NewInputManager(project, ports, ports, surf, compileCh, pe)
    compiler := controller.NewCompiler(project, pe.NextEvents(), pe.EditCounters(), compileCh)
    
    go pe.Run(ctx)
    go im.Run(ctx)
    go compiler.Run(ctx)
    
    for i := range project.Tracks { compileCh <- i }
    
    p := tea.NewProgram(view.New(im, project))
    p.Run()
}
```

**Verify:** `go build ./...`. App launches. Edit a drum step. Press play. Tick advances; event fires.

**Dependencies:** Steps 3, 4.

### Step 6 — Delete `sequencer/`, `tui/` (S)

Same as before. `git rm -r sequencer/ tui/`.

### Step 7 — Boundary tests (M)

Tests at module boundaries:
- `model/devices/drum_test.go`: state validation, JSON round-trip.
- `controller/devices/drum_test.go`: `drum.Compile(...)` deterministic given seed; `drum.ToggleStep(...)` mutates correctly.
- `view/devices/drum_test.go`: `drumview.Render(...)` produces expected string for a known state.
- `controller/playback_test.go`: end-to-end with MockSurface + mock ToExternal + injectable clock.
- `controller/compile_test.go`: send trackIdx → nextEvents populated.
- `controller/input_test.go`: pad routing dispatches to right view function.
- `controller/system/project_test.go`: Save → Load → DeepEqual.

---

## Risks & gotchas (revised)

1. **Schedule format breaking change** (handled in earlier work).
2. **PPQ=960 compile-time const.** In `model/project.go`.
3. **No runtime state in Model.** Transport/Tick own by PlaybackEngine.
4. **No Device interface, no smushed device structs.** This is the big one. If you find yourself writing `func (d Drum) Compile(...)` — stop. It should be a free function `func Compile(state *Drum, ...)` in the right layer.
5. **Conceptual entities have parallel paths across layers.** "drum device" lives at `model/devices/drum.go`, `controller/devices/drum.go`, `view/devices/drum.go`. Don't put drum stuff anywhere else.
6. **Functions are universally callable.** `controller/system/project.SaveProject` can be called from main, InputManager, view/system/project, tests, anywhere. Organization is for navigability, not access control.
7. **Subdirs (`devices/`, `system/`) are organizational, not semantic.** They group related files for findability. They don't define a category that means anything beyond "these files belong together by domain vibe."
8. **CompiledPattern lives only in PlaybackEngine.** Never in Model.
9. **EditCounter invariant.** InputManager (and its callees in view/controller layers that mutate state) must increment after every state change. Compiler stamps the counter on each CompiledPattern.
10. **Compile must be pure.** `drum.Compile(spec, seed)` — no side effects, no global state.
11. **Preview MIDI is direct.** `midi.Send(...)` from view's HandlePad — no controller call needed.
12. **Per-track RWMutex** on each device state. View takes RLock for rendering; controller mutators take Lock.
13. **Import discipline.** `controller/devices/drum` imports `model/devices` (for Drum state type) but NOT `view/`. `view/devices/drum` imports both `model/devices` (state type) and `controller/devices/drum` (mutator functions). View never has functions that controller imports.
14. **Seed quality.** xorshift64, not `math/rand`. Seed `= time ^ trackIdx ^ loopCounter`. Log seeds.

---

## Open questions

1. **Package naming under subdirs.** Does `controller/devices/drum.go` go in `package drum` (one package per domain), or `package devices` (with multiple files)? Trade-off: `package drum` gives short call sites (`drum.Compile`); `package devices` gives one package with shared types. Lean `package drum` per domain — clearer call sites and avoids name collisions when both `controller/devices/drum.go` and `view/devices/drum.go` need different names.
2. **MockSurface.PushPad** synchronous vs buffered.
3. **Settings → resubscribe MIDI port.** Settings now lives in `controller/system/settings.go` + `view/system/settings.go`. The function `settings.ApplyPortChange(track, port, chan)` could either signal InputManager directly or mutate state with a version counter that InputManager watches. Decide during impl.

---

## Time estimate

Restructuring of existing work: **~1 day**. The shape change is real but most of the LOGIC committed in Steps 3 and 4 survives — it just gets relocated and the wrapping Go types disappear. Drum's Compile body, Drum's HandlePad body, Drum's Render body all move to their respective files unchanged.

Then Steps 5-7 as planned: ~2 days for view + main.go wiring + tests.

Total remaining: **~3 days** of evening work to ship a runnable app.
