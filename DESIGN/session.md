# Session — the live composing surface

> User-perspective UX. The target experience, not what's built. Mechanics
> (how queuing, loop-boundary swaps, and compilation work) live in `ARCHITECTURE/`.

The Session is how you turn 8 tracks of patterns into a song, live. It's an 8×8
grid where **each row is a track** and **each column is a pattern slot** on that
track. You build a song by deciding what plays, when — in the moment.

## The grid

```
            col 0   col 1   col 2  ...  col 7
 track 0  [  P0  ] [  P1  ] [ ... ]    [  P7  ]
 track 1  [  P0  ] [  P1  ] [ ... ]    [  P7  ]
   ...
 track 7  [  P0  ] [  P1  ] [ ... ]    [  P7  ]
```

Each cell is a pattern slot. Its color tells you its state at a glance:

- **off** — empty slot, nothing recorded
- **dim / blue** — has content, not playing
- **green** — currently playing
- **amber** — queued: it'll start at the next loop boundary

## Launching and queuing

Tap a slot to **queue** it. It doesn't interrupt — it lights amber and swaps in
cleanly at the track's next loop boundary, so the groove never stutters. When it
takes over it goes green; the old one goes back to dim.

A track loops its current pattern forever until you queue something else.

### Un-queue

Changed your mind before it launches? **Tap the amber (queued) slot again** to
cancel the queue. The current pattern just keeps looping as if you never touched
it. Queuing is a toggle: tap to arm, tap again to call it off.

## Scenes — launching a column across all tracks

A **scene** is one pattern column launched on every track at once. The grid has
rows = tracks and columns = pattern slots, so column *c* is "slot *c* on all 8
tracks." The **top row of round buttons** are the scene triggers: top button *c*
queues column *c* across every track together.

This is how you move between song sections — verse, chorus, drop, breakdown — in
one gesture instead of stabbing eight pads and hoping they land on the same
boundary. Scenes are the backbone of performing structure live.

## One-shots — fills and transitions

Hold the **one-shot** modifier and tap a slot. That pattern plays **exactly
once**, then the track **falls back to whatever was playing before it.**

This is how you do fills and transitions without a dedicated "fill" concept: any
pattern can be a one-shot. Loop a drum groove, one-shot a one-bar fill over the
top, and you're back in the groove automatically the next loop. No re-launching,
no cleanup.

## Dropping and bringing back parts

There's no track mute/solo here — silencing a track is your **downstream mixer's**
job, not the sequencer's. The MIDI brain decides *what plays*; your mixer decides
*what you hear*. So live dynamics come from what you launch:

- **Drop a part** by launching an **empty slot** on that track — it goes silent at
  the next boundary, the rest keep going.
- **Bring it back** by launching a pattern slot again.
- **Sparse vs. full patterns** in different slots give you breakdown vs. drop —
  launch the sparse one for tension, the full one for release.
- **Scenes** swap the whole arrangement at once for section changes.

(If you want to yank a part instantly without waiting for the boundary, that's a
fader/mute on your mixer — which is exactly where it belongs.)

## Transport feedback

You can't perform what you can't see coming. The surface always shows you:

- what's **playing** (green) on every track
- what's **queued** and about to swap (amber)
- where the **playhead** is in the loop

So you can time your launches, scenes, and one-shots to land on the beat.

## How a song gets built (the workflow)

There's no timeline, so a performance is a path through these gestures:

1. **Foundation** — launch a drum groove on one track, queue a bass looper on
   another. They lock together at the boundary.
2. **Build the section** — focus a looper track, play a phrase into the grid,
   record it, overdub a harmony. Switch back to Session, layer it in.
3. **Add tension** — bring in more tracks by launching their slots; hold parts
   back by leaving their slots empty.
4. **Section change** — fire a **scene** to move everyone to the chorus at once.
5. **Transitions** — drop a **one-shot** fill on the boundary.
6. **Breakdown** — launch empty/sparse slots so a single loop breathes, then
   launch the full patterns back in.
7. **Outro** — launch parts down to empty one at a time, stop the transport when
   it's done.

The whole song is this, performed — never drawn.
