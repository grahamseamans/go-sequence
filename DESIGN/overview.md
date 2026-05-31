# go-sequence — the instrument

> **What this folder is.** DESIGN describes go-sequence from the *user's seat* —
> what you see, what you press, how it feels to play. It is the UX spec, not the
> implementation. How any of it actually works under the hood (playback engine,
> pattern compilation, the surface plumbing) lives in `ARCHITECTURE/`, not here.
>
> This is the **target** experience. Most of it isn't built yet.

## What it is

A **live MIDI composer/looper**, played primarily from a Novation Launchpad X.
Not a DAW. A MIDI brain that sits between your controller and your synths.

You don't arrange a song on a timeline. You **build and perform the song live**,
in the moment — launching patterns, playing melodies, dropping in fills,
launching parts in and out to create tension and release. The song's structure
lives in your hands, not on a screen.

This makes it strong for music that *is* a performance — techno, house, ambient,
hip-hop beats, loop-based songwriting — and deliberately bad at precise,
always-identical linear arrangements. That's the trade we're choosing.

## The shape

- **8 tracks.** Each track is assigned exactly one device.
- **Each track has a bank of pattern slots** you fill with patterns and trigger live.
- **Two devices**, and only two:
  - **Drum** — a step sequencer. The rhythmic foundation: kicks, snares, hats,
    percussion. (See `drum.md`.)
  - **Looper** — the melodic voice. You *play it like a keyboard* on the 8×8 grid
    and it records what you play into a loop: bass, chords, leads, textures.
    (See `looper.md`.)

Drums hold it down; loopers carry everything melodic. If you want distinct bass,
chords, and lead, you give each its own looper track.

## The surfaces

You play this on hardware. The screen is for setup and glancing, not performing.

- **Launchpad X — the instrument.** The 8×8 RGB grid plus the ring of round
  buttons (top row, right column). This is where you perform: play notes, launch
  patterns, trigger scenes. The primary surface.
- **Terminal (TUI) — the overview.** A read-mostly window onto the same state:
  what's playing, what's queued, the settings. Secondary. You glance at it; you
  don't perform on it.
- **MIDI keyboard — note input.** An external keyboard can play notes into the
  focused melodic track, the same as playing the grid does.

At any moment the Launchpad is **focused** on one thing — a device's editor, the
Session, Settings, or the Project browser. Focus is how the one grid serves many
jobs. You change focus, the grid becomes that thing.

## Session — where you compose live

The **Session** is the heart of live performance: an 8×8 grid where each column
is an instrument and each row is a pattern. You queue patterns, fire whole
pattern rows across all instruments as **scenes** via the right-column "play
all" buttons, drop parts by launching empty slots, and watch the transport tell
you what's coming. This is how a song gets built from nothing in front of
people. (See `session.md`.)

Silencing a track is the **mixer's** job, not the sequencer's — there's no
track mute/solo here. (See `session.md`.)

## Conscious non-goals

These are doors we've deliberately closed, not features we forgot:

- **No timeline / arrangement view.** No laying patterns out over time. Everything
  is live. This is the whole point.
- **No song mode.** Not even a lightweight "record my scene launches and replay
  them" — that's a timeline in disguise. Scenes and live launching give you the
  repeatability that matters without it.
- **No track mute/solo.** Silencing a part is the downstream mixer's job; the
  sequencer decides what plays, not what you hear.
- **Pattern chaining** (short sequences of patterns that advance on their own) is
  a *far-future maybe*, not a goal.
