# Shortcuts

Things we did the quick/janky way and want to come back to.

Format: one line per shortcut. Date, file:line if relevant, what was done, why it's a shortcut.

---

- 2026-04-19 view/{launchpad,tui,keyboard}/piano.go: piano viewport / edit-step / note-mutator helpers are duplicated per-view-package (pianoViewScale, pianoAddNote, pianoSortNotes, pianoCenterOnSelection, etc.). Could be lifted into a shared leaf package (e.g. view/devices/piano or model/devices/pianoutil) once the shape stabilizes. Same reasoning applies to the drum step-mutator helpers duplicated between view/launchpad/drum.go and view/tui/drum.go.
- 2026-04-19 view/keyboard/keyboard.go: RunRoutine polls project.UI.KeyboardRoute every routePoll (100ms) to detect reroutes. A channel-based notification would be cleaner but requires wiring a setter helper for UI code to call when it mutates KeyboardRoute; polling keeps the write path simple for now.
- 2026-04-19 view/tui/tui.go: Bubbletea Model + HandleKey + Render are wired but the per-domain Render functions are still stubs (they return ""). Full TUI render bodies need to be ported from sequencer/* in a follow-up.
- 2026-04-19 view/launchpad/*.go: per-domain LED render functions (drumLEDs, pianoLEDs, ...) are stubs. LED frames need to be ported from sequencer/* in a follow-up.
