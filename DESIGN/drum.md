# Drum — the rhythmic foundation

> User-perspective UX. The target experience, not what's built. Mechanics live in
> `ARCHITECTURE/`.

The Drum device holds down the rhythm: kicks, snares, hats, percussion. Where the
Looper is *played*, the drum is *programmed* — a step sequencer you build and
mutate live while it plays. It is **not** a melodic keyboard; the Looper owns that
role. Step-editing is the right interaction for drums, and the goal is to make it
feel as fast and live as playing the looper grid.

## What a drum pattern is

- **16 lanes** — one per drum sound.
- **Up to 32 steps** per pattern, variable length.
- **16 pattern slots** per track.
- **Kits** map the 16 lanes to actual MIDI notes: **GM**, **RD-8**, **TR-8S**,
  **ER-1**. Patterns store lane indices, so switching kit re-voices the same
  pattern onto different hardware.

## The Launchpad layout

```
 ┌─────────────────────────────┐
 │  step  step  step  ...  step │  top 4 rows = the 32 step toggles
 │  step  step  step  ...  step │  for the selected lane
 │  step  step  step  ...  step │  (lit = on, dim = off, white = playhead,
 │  step  step  step  ...  step │   dark = beyond pattern length)
 ├──────────────┬──────────────┤
 │  lane select │   commands   │  bottom-left 4×4 = pick which of the 16
 │   (4×4 = 16) │    (4×4)     │  lanes you're editing
 │              │              │  bottom-right 4×4 = clear lane, clear
 │              │              │  pattern, preview-arm, record-arm
 └──────────────┴──────────────┘
```

Lane pads do double duty: they **select** a lane to edit, and when **preview** is
armed they **audition** the sound, and when **record** is armed and the transport
is running they punch a step in at the playhead.

## Building and tweaking live

The whole pattern is editable **while it plays** — tap a step to add or remove it
and it's just there on the next pass. That's the live sketchpad feel: lay down a
kick, tap in some hats, mutate as the groove runs.

- **Per-step velocity is not in scope yet.** Steps play at a fixed velocity for
  now. On-surface velocity editing (e.g. hold a step and adjust) is a possible
  future addition, not a current goal.
- **Fills and variations** come from **Session**, not a separate drum feature:
  put a fill in another pattern slot and fire it as a **one-shot** — it plays once
  and the groove returns. (See `session.md`.)
- **Record from a MIDI keyboard / pads:** incoming notes auto-map to lanes via the
  kit and punch in live, with their played velocity.

## TUI (secondary)

The terminal mirrors the same edit: arrows/`hjkl` move the cursor, `space`/`x`
toggles a step, `[`/`]` resize the pattern, `c`/`C` clear lane/pattern (with a
confirm), `r`/`p` arm record/preview, `<`/`>` change pattern. It's for setup and
glancing — performing happens on the Launchpad so you're not staring at a screen.

## Direction

The drum has to get the same live-first polish the looper is getting (immediate
visual feedback, fast step toggling, one-shot fills) or it becomes the "lesser"
device and people fall back to canned patterns — which breaks the build-it-live
promise. Step-editing + live MIDI recording is the right core; everything else is
making that core fast enough to do mid-performance.
