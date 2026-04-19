package keyboard

import (
	"go-sequence/midi"
	"go-sequence/model"
)

// metropolixMIDI currently ignores incoming MIDI — the old sequencer left
// a stub comment ("could record incoming notes to stages") and we keep the
// same behavior here. Do NOT silently record.
//
// TODO(stage-recording): define note → stage mapping when we add this.
func metropolixMIDI(track *model.Track, out midi.ToExternal, ev midi.Event) {
	// No-op.
}
