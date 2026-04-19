# go-sequence

A free, software clone of expensive hardware MIDI sequencers (Cirklon, Hapax, Pyramid, Metropolix, Torso T-1), driven by a Novation Launchpad. Not a DAW — a MIDI brain that sits between your controller and your synths.

This document describes the target architecture. Source of truth for design decisions. Check git history for older versions.

---

## 1. The whole system

Three layers, organized by domain within each:

- **`model/`** — state. 
- **`controller/`** — functions that operate on state
- **`view/`** — renders state, captures user input (pad presses, keys) and calls controller functions in response.

Example:
```
model/devices/drum.go         — drum state struct
controller/devices/drum.go    — functions on drum state (Compile, ToggleStep, ...)
view/devices/drum.go          — drum UI: render the grid, capture pad presses
```

The "drum device" is a real conceptual entity (the user thinks about it that way), but it is **NOT a Go type**. It's a domain that has files in each of the three layers, all at parallel paths. Same shape for every domain.

---

## 3. Principles

4. **View is reactive — `view = f(model)`.** Renders model state. Captures user input gestures and interprets them, then calls controller functions to mutate state. View never holds state of its own. Controller never calls view.

6. **View owns UI input loops** (pad presses, terminal keyboard) — knows the launchpad surface for both rendering and input. Reads `project.UI.Focus`, dispatches to `view/devices/<kind>.HandlePad(state, project, row, col)` which interprets the gesture and calls controller mutators.

7. **Controller owns external input loops** — `controller.RunMidiInput(ctx, project, midiPorts)` subscribes per-track to `(port, channel)` and dispatches MIDI events to controller mutators (e.g., `drum.RecordEvent`). MIDI input is external hardware, not UI gestures.

8. **Playback is a pure-walker function.** `controller.RunPlayback(ctx, project)` is the goroutine entry point. it reads from the model and puts that information out of the midi device. On each tick it computes where the playhead should be (the playhead is not stored) and then it reads that midi data, and it shoots it out.

9. **Views trigger compile.** they call the controler to change the state of the human pattern and then recompile the machine pattern, it signals compile (via channel send).

10. **Fresh rolls every loop.** When playback wraps and swaps in `next_events`, signals compile to produce new `next_events` with a fresh seed. - this is for randomness




basic vibe (from human, so badly written, but actually good design hopefully)

The way that this machine works is

you have a basically endlessly polling loop

it, based on time, figures where to read from, reads from it, and puts that out midi

now

if the user changes something, we change what there is to read from (or what there will be to read from)

this is the whole ting


we store patterns basically twice

once as the pattern as the device sees it, this is much simpler hopefully, really just the device settings, we call it the human pattern
then also we store it as the machine pattern, this is the sorta comiled patter, but it's midi that we can shoot out

these patterns are, of course, stored in the model, because they're state

we have 8 tracks

each track can have a device assigned to it

each track can have a bunch of patterns

we also have the selected patterns

these patterns are neat

that's where we know where to read from

we use some math with the absolute position that we're at, and where we started, and how much time has passed, and when we started playing the current pattern, to figure out what midi click to read from

when we get to the end of a pattern, we say - that one needs to be deleted and replaced, this allows for randomness

all patterns always have 2 patterns ready to go at all times, if you finish one, you delete it, replace it with a new one, read the one thats in the queue

the devices are then hopefully pretty simple

a lot of their logic lies in the views, at least for how it's displayed it's all there

then model of the device has a lot to say about how it works as well

the main job of the controller really is to do compilations

most of the other tasks are problaby handled by the view

i'm tempted call renedering on the launchpad also a view, i think that this is accurate 

each pattern that's rendered out has a uuid so that we can trash it and know which one we're reading from, or an counter idx, idk

now

i wasnt totally clear earlier with the dual pattern thing for the queue

each human pattern, has two machine patterns for it ready to go

the playhead knows which one it's reading, if it finishes palying ti back, it triggers a go routine (pattern fresh go routeine) to delete the now stale pattern, and generate another machine pattern for that same human pattern

so it's

human pattern - (machine pattern a, machine pattern b)

idk what the model types look like for each device, and ikd if this is too complex as it is for you to figure it out, if it is I can come up with those too


i'm wondering what's really going to be in the view code

can the view code update the human pattern based on the user input? i feel like it could

that does feel like some heavy lifting for the view code

but if that's the case, then the only thing that the device controller does is compile, which sounds nice

so yea

then 

view does a lot, it gets into state and manages it? or is that too cursed. I feel like, making a controller do it is also strange, you know, i really think that the view can just do this

the view is just a funciton that visually shows the human pattern, beause of this, it necissarily udnerstand the human pattern, and therefore is a good place to edit it.

so there are two types of midi inputs

one is from the lanchpad, one is from the keyboard

my claim i that the launchpad is part of the display, it is therefore a view. it renders, it displays the current thing that we are viewing just as the screen does.

the keyboard on the other hand is pretty fickle. I'm not sure what that is

we need to be able to assign it to a track, so, that would be a part of the model, something that assigns *where* the midi from the keyboard *goes*

then you I geuss have a somehting that interprets this? that's all very strange

ah

the keyboard is also a view

it shows you notes

the keyboard is also a view. then this is just simple?

we still need to define which track is focused for the keyboard, but it's just another view, and edits state the same as the others.



### stuff:

Overall composing stuff;
  - Session — clip launcher (queue patterns on each track)

Musical devices (track-assignable):
  - Drum — 16 lanes × 32 steps + velocity + length + per-pattern kit
  - Piano — polyphonic notes with start/duration/pitch/velocity
  - Metropolix — 8 stages with pitch/gate/probability/ratchets/slides/accumulators
  - Empty — placeholder for unconfigured tracks

System UIs (project-level, focusable):
  - Project / Save browser — list/load/save/rename/delete project files
  - Settings — per-track port/channel config, tempo, kit selection

Surfaces (views) — human I/O
  - Launchpad X — Renders 8×8 RGB LEDs and some extras, captures pad presses
  - Terminal TUI — Renders text, captures keystrokes
  - Keyboard (MIDI in) — captures notes from external MIDI keyboard for human pttern update

Background machinery (controller)
  - Playback — time-driven, reads machine patterns from model, sends MIDI out
  - Compile — on-demand, generates machine patterns from human patterns + seed
  - Pattern fresh goroutine — regenerates dual-buffer machine patterns when one gets consumed

Outputs
  - MIDI out — to synths/hardware, multiple ports/channels

State (model)
  - Project — name, tempo
  - tracks: device kind, port name, channel, kit, scheduled pattern number (Playing+Queued)(can be empty) (playing loops if no queued), N human patterns each with 2 machine patterns, timestep of current pattern at pattern start
  - Transport runtime — playing, t0,
  - UI state — focus (which device/widget shown), keyboard routing (which track receives MIDI in), browser cursors, edit positions


## other details

you figure out where to read from the pattern by:
t0 - when you started the current pattern = position in current pattern.
if that's longer than the pattern you're done with it
 - tell the compiler go routeine to make you a new one and trash what is your current one
 - make your current one the next available one
 - this might *not* be t0 for the current pattern, because you might have missed clicks, so you need to calculate where you *should* have started this pattern, if that makes sense. if not ask me and I can explain it more





