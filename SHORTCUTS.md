# Shortcuts

Things we did the quick/janky way and want to come back to.

Format: one line per shortcut. Date, file:line if relevant, what was done, why it's a shortcut.

---

- 2026-04-19 view/{launchpad,tui,keyboard}/piano.go: piano viewport / edit-step / note-mutator helpers are duplicated per-view-package (pianoViewScale, pianoAddNote, pianoSortNotes, pianoCenterOnSelection, etc.). Could be lifted into a shared leaf package (e.g. view/devices/piano or model/devices/pianoutil) once the shape stabilizes. Same reasoning applies to the drum step-mutator helpers duplicated between view/launchpad/drum.go and view/tui/drum.go.
- 2026-04-19 view/tui/settings.go: port-name selection is not wired. Picking a MIDI out port requires an interactive list UI (or at least a cycle-through-available-ports keybind) — neither is trivial text. Add once a port-enumeration helper exists on the midi side. (2026-05-10: resolved — midi.OutputPortNames() added and the settings popup picker uses it.)
- 2026-05-10 view/tui/settings.go: the popup picker is appended *below* the rest of the page rather than overlaying the table area. lipgloss can't compose layered text cheaply and the alt-screen has plenty of vertical room. Old UI drew the box on top of the table; ours pushes the table up. Revisit if vertical real-estate ever gets tight.
- 2026-05-10 view/tui/session.go: launching uses 'enter' instead of 'space' because space is reserved as a global play/pause hotkey in the new dispatcher (tui.go HandleKey). Old UI used space. Either rebind global play/pause or accept this — bound to confuse muscle memory.
- 2026-04-19 view/launchpad/session.go: pattern columns 0..7 map directly to patterns 0..7; patterns 8..15 aren't reachable from the 8x8 pad grid. A second "page" or a modifier key could address this later.
- 2026-04-19 view/launchpad/metropolix.go settings page (page 7): only the first 4 of 22 scales are reachable from pads (row 6, cols 0..3) — same layout as old UI. Cycling through the full 22 requires the TUI '</>' keys. A scrolling scale-picker (e.g. a 4x5 grid of named scales) would let the launchpad reach them all.
- 2026-04-19 view/launchpad/metropolix.go settings page (page 7): rows 0..2 are reserved/blank. Could host per-stage parameters the main editor can't reach from pads (e.g. a second probability preset or accumulator value toggles).
- 2026-04-19 view/launchpad/piano.go: row 7 is a nav/command bar (pan/zoom/record/clear) with the piano roll on rows 0..6. Old sequencer used the full 8 rows for the roll and handled pan/zoom only via keyboard; we reserve a row so the launchpad is usable standalone, at the cost of one pitch row of visible range.
