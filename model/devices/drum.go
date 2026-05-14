package devices

import "sync/atomic"

// DrumStep is one step in a drum note lane.
type DrumStep struct {
	Active   bool  `json:"active"`
	Velocity uint8 `json:"velocity"`
	Nudge    int8  `json:"nudge"`
}

// NoteLane is a 32-step lane for a single drum voice.
type NoteLane struct {
	Steps [32]DrumStep `json:"steps"`
}

// DrumPattern is a drum pattern spec. Length is in ticks.
//
// Machine is the per-pattern dual-buffer of rendered events. Playback reads
// Machine[CurrentSlot]; when a loop wraps, the playback routine clears the
// consumed slot (Store(nil)) and signals compile to refill it with a fresh
// roll. Compile fills any nil slot and stores a *CompiledPattern.
//
// Not persisted (json:"-"). The pattern is only referenced via *DrumPattern
// (see Drum.Patterns below) because atomic.Pointer cannot be safely copied —
// pointer patterns give us stable backing addresses.
type DrumPattern struct {
	Notes   [16]NoteLane                       `json:"notes"`
	Length  int                                `json:"length"`
	Machine [2]atomic.Pointer[CompiledPattern] `json:"-"`
}

// ConfirmKind values for the drum view's confirm-dialog. Zero is "no
// dialog active", non-zero values select which destructive action the y-key
// should run. The dialog itself lives in the Drum struct (transient UI
// state) — see the ConfirmKind/ConfirmMsg fields.
const (
	ConfirmNone         = 0
	ConfirmClearNote    = 1
	ConfirmClearPattern = 2
)

// Drum is the persisted per-track drum device spec.
//
// Patterns holds pointers so each DrumPattern (which contains non-copyable
// atomic.Pointer fields in Machine) lives at a stable heap address. Never
// copy a *DrumPattern by value; always operate through the pointer.
//
// Preview, ConfirmKind, ConfirmMsg are transient UI state — not persisted.
// Preview toggles MIDI-thru for the keyboard/launchpad lane-select pads.
// ConfirmKind / ConfirmMsg implement a tiny modal dialog used to guard
// destructive actions (clear-lane, clear-pattern) in the TUI.
type Drum struct {
	Patterns          [NumPatterns]*DrumPattern `json:"patterns"`
	EditingPatternIdx int                       `json:"editingPattern"`
	SelectedNoteIdx   int                       `json:"selectedNote"`
	Cursor            int                       `json:"cursor"`
	Recording         bool                      `json:"-"`
	Preview           bool                      `json:"-"`
	ConfirmKind       int                       `json:"-"`
	ConfirmMsg        string                    `json:"-"`
}

// Validate clamps Drum fields into valid ranges and fills in defaults.
// Per-track playback state (current / queued pattern) lives on the
// runtime Cursor and is validated by the project, not the device.
func (d *Drum) Validate() {
	if d.EditingPatternIdx < 0 {
		d.EditingPatternIdx = 0
	} else if d.EditingPatternIdx > NumPatterns-1 {
		d.EditingPatternIdx = NumPatterns - 1
	}
	if d.SelectedNoteIdx < 0 {
		d.SelectedNoteIdx = 0
	} else if d.SelectedNoteIdx > 15 {
		d.SelectedNoteIdx = 15
	}
	if d.Cursor < 0 {
		d.Cursor = 0
	} else if d.Cursor > 31 {
		d.Cursor = 31
	}

	defaultLength := 16 * (PPQ / 4) // one bar of 16th-notes in ticks
	for i := range d.Patterns {
		if d.Patterns[i] == nil {
			d.Patterns[i] = &DrumPattern{}
		}
		pat := d.Patterns[i]
		if pat.Length < 1 {
			pat.Length = defaultLength
		}
		for n := range pat.Notes {
			for s := range pat.Notes[n].Steps {
				step := &pat.Notes[n].Steps[s]
				if step.Velocity > 127 {
					step.Velocity = 127
				}
			}
		}
	}
}
