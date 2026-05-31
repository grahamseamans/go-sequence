package devices

import "sync/atomic"

// LooperPattern is one loop on a looper track. Stores recorded MIDI events
// with tick offsets relative to the loop start. Machine holds the dual-buffer
// of rendered events for this pattern.
type LooperPattern struct {
	Events      []TimedEvent                       `json:"events"`
	Length      int64                              `json:"length"`
	Quantize    bool                               `json:"quantize"`    // per-pattern: snap events to a grid at compile time
	QuantizeDiv int                                `json:"quantizeDiv"` // grid divisions per quarter note (e.g. 4 = 16th notes)
	Machine     [2]atomic.Pointer[CompiledPattern] `json:"-"`
}

// Looper is the persisted per-track looper device spec.
type Looper struct {
	Patterns          [NumPatterns]*LooperPattern `json:"patterns"`
	EditingPatternIdx int                         `json:"editingPattern"`
	Octave            int                         `json:"octave"` // keyboard octave shift, clamped to [-4, 4]
	Recording         bool                        `json:"-"`
	Overdub           bool                        `json:"-"` // runtime: layer onto the loop without erasing
}

func (l *Looper) Validate() {
	if l.EditingPatternIdx < 0 {
		l.EditingPatternIdx = 0
	} else if l.EditingPatternIdx > NumPatterns-1 {
		l.EditingPatternIdx = NumPatterns - 1
	}

	if l.Octave < -4 {
		l.Octave = -4
	} else if l.Octave > 4 {
		l.Octave = 4
	}

	defaultLength := int64(PPQ * 4) // one bar at default length
	for i := range l.Patterns {
		if l.Patterns[i] == nil {
			l.Patterns[i] = &LooperPattern{Length: defaultLength}
		}
		pat := l.Patterns[i]
		if pat.Length < 1 {
			pat.Length = defaultLength
		}
		if pat.QuantizeDiv < 1 {
			pat.QuantizeDiv = 4
		} else if pat.QuantizeDiv > 16 {
			pat.QuantizeDiv = 16
		}
	}
}
