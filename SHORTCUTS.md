# Shortcuts

Things we did the quick/janky way and want to come back to.

Format: one line per shortcut. Date, file:line if relevant, what was done, why it's a shortcut.

---

- 2026-04-19 view/{launchpad,tui,keyboard}/piano.go: piano viewport / edit-step / note-mutator helpers are duplicated per-view-package (pianoViewScale, pianoAddNote, pianoSortNotes, pianoCenterOnSelection, etc.). Could be lifted into a shared leaf package (e.g. view/devices/piano or model/devices/pianoutil) once the shape stabilizes. Same reasoning applies to the drum step-mutator helpers duplicated between view/launchpad/drum.go and view/tui/drum.go.
- 2026-04-19 view/keyboard/keyboard.go: RunRoutine polls project.UI.KeyboardRoute every routePoll (100ms) to detect reroutes. A channel-based notification would be cleaner but requires wiring a setter helper for UI code to call when it mutates KeyboardRoute; polling keeps the write path simple for now.
- 2026-04-19 view/tui/tui.go: Bubbletea Model + HandleKey + Render are wired but the per-domain Render functions are still stubs (they return ""). Full TUI render bodies need to be ported from sequencer/* in a follow-up.
- 2026-04-19 view/launchpad/*.go: per-domain LED render functions (drumLEDs, pianoLEDs, ...) are stubs. LED frames need to be ported from sequencer/* in a follow-up.
- 2026-04-19 view/tui/settings.go: port-name selection is not wired. Picking a MIDI out port requires an interactive list UI (or at least a cycle-through-available-ports keybind) — neither is trivial text. Add once a port-enumeration helper exists on the midi side.
- 2026-04-19 view/launchpad/session.go: pattern columns 0..7 map directly to patterns 0..7; patterns 8..15 aren't reachable from the 8x8 pad grid. A second "page" or a modifier key could address this later.
- 2026-04-19 view/launchpad/metropolix.go settings page (page 7): only the first 4 of 22 scales are reachable from pads (row 6, cols 0..3) — same layout as old UI. Cycling through the full 22 requires the TUI '</>' keys. A scrolling scale-picker (e.g. a 4x5 grid of named scales) would let the launchpad reach them all.
- 2026-04-19 view/launchpad/metropolix.go settings page (page 7): rows 0..2 are reserved/blank. Could host per-stage parameters the main editor can't reach from pads (e.g. a second probability preset or accumulator value toggles).
- 2026-04-19 view/tui/piano.go: lipgloss colors are hardcoded (pianoStyleNote, pianoStyleSelected, ...) instead of pulling from theme.Theme. Wiring theme.Theme through the TUI (and picking palette roles for each role) is a follow-up; the numbers match the old launchpad LED palette so the two surfaces read as the same instrument.
- 2026-04-19 view/launchpad/piano.go: row 7 is a nav/command bar (pan/zoom/record/clear) with the piano roll on rows 0..6. Old sequencer used the full 8 rows for the roll and handled pan/zoom only via keyboard; we reserve a row so the launchpad is usable standalone, at the cost of one pitch row of visible range.
