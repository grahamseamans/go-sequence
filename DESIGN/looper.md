# Looper — the melodic voice

> User-perspective UX. The target experience, not what's built. Mechanics
> (recording, compilation, double-buffering) live in `ARCHITECTURE/`.

The Looper is how you play melody, bass, chords, and texture. You **play it like
an instrument** on the 8×8 grid and it records what you play into a loop. One
looper track is one voice; give bass, chords, and lead their own tracks.

## The keyboard

The 8×8 grid is a **chromatic-fourths** keyboard — the same isomorphic layout as
Ableton Push and Novation's Note mode:

- **root note at bottom-left**
- **moving right: +1 semitone**
- **moving up a row: +5 semitones (a perfect fourth)**

```
 row up = +5 (a 4th)        every chord and scale shape is the
   ↑                        SAME shape in every key — you learn a
 → +1 semitone              shape once and it works everywhere
```

Because it's isomorphic, a chord shape you learn in one place works in every key.
It feels guitar-like (fourths). The **Octave** buttons shift the whole grid up or
down so you can reach bass or lead range.

**All twelve notes are present** — no scale-lock. This is deliberate: the point of
a playable grid is freedom — chromatic runs, outside notes, quick key changes. The
grid stays fully chromatic. (A scale-lock mode could be revisited far down the
line, but it's not a goal.)

## Playing feel

- **Pads are momentary.** Hold a pad and the note sustains; release and it stops.
  The loop captures **real note lengths**, so chords, sustained pads, and staccato
  stabs all record the way you played them.
- **Velocity-sensitive, always.** Play harder, get louder — the loop **keeps your
  dynamics**. This is the whole reason it feels like an instrument and not a
  trigger pad. Velocity sensitivity is always on; there's no flat-velocity mode.
- **Notes don't wrap around the loop.** A note still held when the loop reaches
  its end gets a clean note-off at the seam — loops never leave a hung note
  ringing, and a held note never bleeds back into the top of the loop.

## What the grid LEDs show (so the layout helps you play)

Kept deliberately minimal — orientation, not a light show:

- **Root notes** highlighted in a distinct color — your anchor on a chromatic grid.
- **Octave markers** — a subtle shade change each octave so you know your register.
- **Notes currently sounding** in the playing loop light up, so you can *see* what
  the loop contains, not just hear it.
- When **Record** or **Overdub** is armed, the whole grid takes on a gentle tint /
  pulse: "you are capturing now."

## The controls (outer ring)

The grid plays; the round buttons control. (Launchpad X has buttons only on the
top and right.)

**Top row — record & shape:**

| Button | Does |
|--------|------|
| **Record** | Arms record. Replaces the loop from scratch on the next downbeat. Pulsing red = "next pass wipes everything." |
| **Overdub** | Layers new notes on top of the existing loop without erasing. Additive and forgiving — never cuts your earlier layer. |
| **Clear** | Erases the loop. |
| **Quantize** | Snaps the loop tight to the grid. Non-destructive — toggle it off and your original human timing comes back (nothing is thrown away). Off = loose/human, on = tight/techno. |
| **Octave −/+** | Shift the keyboard down/up. Takes effect **immediately**. |
| **Loop length −/+** | Shorten/lengthen the loop. Takes effect at the **loop boundary** so it doesn't glitch. |

**Right column — 8 pattern slots:** select/launch a pattern slot for this track.
Selecting a slot makes it the one you play into *and* queues it to play, so what
you hear and what you record are always the same loop. Slot colors match Session:
off = empty, blue = has content, green = playing, amber = queued.

## Recording a loop live

1. **Arm** Record (replace) or Overdub (layer). The grid tints to confirm.
2. You **monitor dry while armed** — you hear your own playing through the track's
   synth, mixed over the existing loop, so you can play in time and in key.
3. Recording starts on the **next downbeat** (a visual count-in helps you come in).
4. **Record** captures one pass and replaces; **Overdub** keeps layering each pass
   until you disarm.
5. Don't like it? **Clear** and go again, or just overdub a fix on top.
