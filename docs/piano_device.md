> DRAFT — written by AI from current code. Edit / rewrite / delete freely.

# piano device

a piano roll, but with a specific job: fixing notes you played in via the midi keyboard. not for composing from scratch — the editing keys are intentionally stingy.

the model is just a list of notes per pattern. each note has start (in beats), duration (beats), pitch (midi 0..127), velocity.

when record-armed, notes from your midi keyboard get appended: start = current beat, duration grows while held, pitch = what you played. (recording isn't fully wired yet — flagged in README as priority.)

then you edit. the tui shows a grid centered on the selected note.
- hjkl selects (next/prev note by time, by pitch)
- yuio moves the selected note around
- n / m shortens / lengthens it
- space adds a new note at the view center, x deletes the selected one

view stuff:
- q / w zoom (8 levels, 1/64 beat per column up to 4 beats per column)
- a / s vertical mode: smushed (~12 pitches visible) or spread (~24)
- d / f horizontal edit sensitivity (how big yuio moves are)
- e / r vertical edit sensitivity (1 semitone or 1 octave)

pattern controls:
- < / > switch editing pattern
- [ / ] grow / shrink pattern length (1..64 beats)
- c clear pattern

overlaps render with a heavier rune (═ vs ─) so you can see when two notes share a row at the same beat.

still TODO: real midi-keyboard recording, quantize.
