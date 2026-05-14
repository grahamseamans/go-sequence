> DRAFT — written by AI from current code. Edit / rewrite / delete freely.

# midi i/o

go-sequence doesn't make sound — it sends midi to whatever you have on the other end of the cable (a synth, sampler, drum machine, software instrument, anything).

## output

per track: a midi output port name and a channel (1..16).
- multiple ports supported — different tracks can target different devices.
- channel 1..16 as the user sees it; stored internally as 0..15.
- if a track's port name is empty, nothing goes out — set it from the settings page picker.

what's emitted:
- note on / note off
- (eventually) cc, program change, etc. — not yet wired

drum tracks pick midi notes through a kit (see drum_device.md). the kit maps slot index 0..15 → midi note. swap the kit, your patterns map to a different drum machine without re-editing.

## input

a single midi keyboard, routed to one track at a time.
- `project.UI.KeyboardRoute` names the destination (0..7).
- the keyboard goroutine watches a channel — when the route changes, it picks up immediately, no polling.

what input does today:
- on the piano device with record-arm on: notes get appended to the editing pattern.
- on the drum device with preview-arm on: notes trigger the corresponding lane sound through the track's normal output.

input doesn't bypass the output port — it goes through the normal track routing, so what you play sounds like what the track plays.

## ports

the midi port list is queried fresh from the OS on demand (e.g. when the settings picker opens) via gomidi. plug a new device in, open the picker, it appears. no rescan key needed.

## not yet implemented

- midi clock in / out
- transport sync (start / stop / continue messages)
- midi thru
- per-track midi input (only the one global keyboard input today)
