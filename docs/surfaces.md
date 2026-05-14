> DRAFT — written by AI from current code. Edit / rewrite / delete freely.

# surfaces — launchpad, tui, midi keyboard

there are three input surfaces and two display surfaces. they all read the same model. nobody owns state but the model.

## launchpad x (primary controller)

a novation launchpad x. plug it in, go-sequence finds it and sends a sysex on startup that puts it in programmer mode. no button dance.

physical layout:
- 8×8 main grid (rows 0..7, cols 0..7) — the meat. every device defines its own layout here.
- 8 scene buttons on the right column (col 8) — usually "switch page" or "select something".
- 8 control buttons across the top row (row 8) — usually transport / mode toggles.

LEDs are RGB-ish. internally each pad takes a palette index (0..127); render code passes RGB and it's mapped to the nearest palette color. so don't expect pixel-perfect color, expect "close enough".

pad events come in as note-on / note-off (main grid + right column) or CC (top row). the rest of the code just gets (row, col, down) — the CC/note distinction is hidden.

## tui (companion display)

terminal screen. runs in alt-screen mode (clears when you quit). renders text at 30Hz. one keyboard, one focused domain.

every device renders its own tui view that mirrors what the launchpad shows, plus extra detail (selected note info, key help, etc.). vim-style keys throughout (hjkl, <>, [], etc.). a few keys are globally reserved — see focus / global keys below.

## midi keyboard (input)

an external midi keyboard plugged into the computer. one keyboard, one destination track at a time. project.UI.KeyboardRoute names the destination (0..7); the keyboard goroutine watches a channel for route changes so switches are instant — no polling.

right now it's used for:
- piano: record-arm + play notes → they get written into the editing pattern
- drum (preview-armed): play a pad sound from keyboard keys

## focus / global keys

doesn't matter which surface you're on, these always do the same thing:
- `space` — play / pause
- `+` / `-` — tempo ±1 BPM
- `1`..`8` — focus track N
- `,` — focus settings
- `S` (shift+s) — open save / load browser
- `tab` — toggle to session (clip launcher)

everything else routes to whichever domain has focus.

## color conventions (where they exist)

not a hard rule across devices yet, but recurring colors:
- bright white = playhead / currently playing
- bright yellow / orange = queued
- dim purple / blue = "has content"
- very dim = "exists but empty"
- bright color (per device) = the device's primary action pad

each device's launchpad legend (rendered at the bottom of its tui view) shows its own palette explicitly. that legend is the source of truth — if you're confused about what a pad does, look at the bottom of the tui screen for that device.
