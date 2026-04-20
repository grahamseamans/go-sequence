package keyboard

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// metropolixMIDI handles an incoming MIDI event on the Metropolix view.
//
// Historically (old sequencer) this was a no-op — the Metropolix device
// generates pitch from its stage grid, not from external keyboard input, so
// there's no natural "record" operation to perform. We keep that behavior
// here: DO NOT silently record, preview, or route keyboard input.
//
// The signature accepts project and trackIdx for symmetry with drumMIDI /
// pianoMIDI and to leave the door open for future stage-recording (tap a
// key → advance to next stage, stamp the key's scale degree onto it).
//
// TODO(stage-recording): define note → (stage, Note-degree) mapping when
// we add this. Until then, don't guess.
func metropolixMIDI(track *model.Track, project *model.Project, trackIdx int, out midi.ToExternal, ev midi.Event) {
	// No-op. See comment above.
}
