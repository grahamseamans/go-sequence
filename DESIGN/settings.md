# Settings — backstage

> User-perspective UX. The target experience, not what's built. Mechanics live in
> `ARCHITECTURE/`.

Settings is the **backstage** area: where you wire each track to your gear before
the show. It is setup, not performance. The design should let you get in, set
things up, and get out — and gently discourage fiddling mid-song.

## What you configure

Per track:

- **Device** — empty / Drum / Looper
- **MIDI channel** — 1–16
- **Output port** — which OS MIDI port the track sends to
- **Kit** — for Drum tracks (GM / RD-8 / TR-8S / ER-1)

Global:

- **Tempo** (BPM)

## The interface

The terminal is the home for Settings — an 8-row (one per track) × 4-column table
(**Device · Channel · Output · Kit**). Move the cursor with the arrows, press
`enter` to open a picker for the cell, with quick-edit shortcuts (`t` cycle device,
`[`/`]` channel, `K` cycle kit). `esc` drops you back to Session.

This is intentionally a screen-and-keyboard job: it's the one place where reading
a list of OS port names and picking from menus beats poking pads.

## Live or not?

Mostly **not** — Settings should feel like it lives behind a curtain. But two
things genuinely matter mid-performance and deserve a fast path:

- **Re-routing a track** to a different output port or MIDI channel when a synth
  dies or you repatch on the fly.
- **Changing a drum kit** live.

Everything else — assigning device types, rescanning ports — is pre-show work.

The Launchpad Settings view (currently a stub) should expose **only** the
live-critical items as a minimal strip — re-route a track, change a kit — with big
confirmation flashes so you can't accidentally re-route a track during a drop.
Full configuration stays on the TUI.
