> DRAFT — written by AI from current code. Edit / rewrite / delete freely.

# metropolix device

clone of the intellijel metropolix's brain. an 8-stage parametric sequencer.

per stage (8 stages per pattern):
- octave (0..7)
- note (scale degree 0..7 — resolved through the pattern's scale + root)
- gate (on/off — off = rest)
- pulse count (1..8 — how many times the stage retriggers before advancing)
- ratchets (1..8 — subdivisions per pulse)
- probability (0..100 — chance the stage fires this loop)
- slide (on/off — slides into the next pitch)
- accumulator (-4..+3 — adds to pitch each pulse)
- accum mode (Reset / PingPong / Hold)
- accum reset (after N pulses)
- gate length (Trg / 1/16 / 1/8 / 1/4 / 1/2 / Full)

per pattern (one set):
- mode: Forward / Reverse / Pendulum / Random
- scale: 22 of them (Chromatic, Major, Minor, Pentatonic, every mode, plus exotic ones like Hirajoshi, Phrygian Dominant, Bhairavi)
- root note (midi pitch)
- length (1..8 active stages)
- slide time

probability is the source of variation. with any stage below 100%, the pattern doesn't repeat identically — each loop re-rolls. that's exactly what the dual-buffered compile in play_loop.md is for.

surfaces:
- launchpad has 8 pages of parameters, one per scene button on the right column. each page shows one parameter type across all 8 stages.
- tui shows one big table: stages as columns, parameters as rows.

still rough: only the first 4 of the 22 scales are reachable from pads. gate length and accumulator are in the model and the tui but the pad layout for editing them is sparse.
