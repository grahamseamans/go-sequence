package devices

import "sync/atomic"

// NoteEvent is a single note in a piano roll pattern. Start and Duration are
// in beats (matches current Piano usage); units may be reconciled later.
type NoteEvent struct {
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Pitch    uint8   `json:"pitch"`
	Velocity uint8   `json:"velocity"`
}

// PianoPattern is a piano roll pattern spec. Length is in beats.
//
// Machine holds the dual-buffer of rendered events for this pattern; see the
// DrumPattern doc comment for the dual-buffer contract. Not persisted.
type PianoPattern struct {
	Notes   []NoteEvent                        `json:"notes"`
	Length  float64                            `json:"length"`
	Machine [2]atomic.Pointer[CompiledPattern] `json:"-"`
}

// Piano is the persisted per-track piano roll device spec. Patterns is a
// pointer array so PianoPattern values (with their non-copyable atomic
// fields) stay at stable heap addresses.
type Piano struct {
	Patterns          [NumPatterns]*PianoPattern `json:"patterns"`
	Schedule          Schedule                   `json:"schedule"`
	EditingPatternIdx int                        `json:"editingPattern"`
	CenterBeat        float64                    `json:"centerBeat"`
	CenterPitch       float64                    `json:"centerPitch"`
	ViewScale         int                        `json:"viewScale"`
	ViewRows          int                        `json:"viewRows"`
	EditHoriz         int                        `json:"editHoriz"`
	EditVert          int                        `json:"editVert"`
	SelectedNote      int                        `json:"selectedNote"`
	Recording         bool                       `json:"-"`
}

// Validate clamps Piano fields into valid ranges.
func (p *Piano) Validate() {
	p.Schedule.Validate(NumPatterns)

	if p.EditingPatternIdx < 0 {
		p.EditingPatternIdx = 0
	} else if p.EditingPatternIdx > NumPatterns-1 {
		p.EditingPatternIdx = NumPatterns - 1
	}

	for i := range p.Patterns {
		if p.Patterns[i] == nil {
			p.Patterns[i] = &PianoPattern{}
		}
		pat := p.Patterns[i]
		if pat.Length < 0 {
			pat.Length = 0
		}
		for j := range pat.Notes {
			n := &pat.Notes[j]
			if n.Pitch > 127 {
				n.Pitch = 127
			}
			if n.Velocity > 127 {
				n.Velocity = 127
			}
			if n.Duration < 0 {
				n.Duration = 0
			}
			if n.Start < 0 {
				n.Start = 0
			}
		}
	}
}
