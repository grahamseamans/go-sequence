package devices

import "sync/atomic"

// PlaybackMode selects how the Metropolix walks through its active stages.
type PlaybackMode int

const (
	ModeForward PlaybackMode = iota
	ModeReverse
	ModePendulum
	ModeRandom
)

// ScaleType selects a quantization scale for the Metropolix.
type ScaleType int

const (
	ScaleChromatic ScaleType = iota
	ScaleMajor
	ScaleMinor
	ScalePentatonic
	ScaleDorian
	ScalePhrygian
	ScaleLydian
	ScaleMixolydian
	ScaleLocrian
	ScaleHarmonicMinor
	ScaleMelodicMinor
	ScaleBlues
	ScaleWholeTone
	ScaleDimHalfWhole
	ScaleDimWholeHalf
	ScaleHungarianMinor
	ScaleDoubleHarmonic
	ScalePhrygianDominant
	ScaleHirajoshi
	ScaleInSen
	ScaleYo
	ScaleBhairavi
	ScaleCount // sentinel; always last
)

// MetropolixStage is a single stage's editable parameters.
type MetropolixStage struct {
	Octave      int  `json:"octave"`
	Note        int  `json:"note"`
	Gate        bool `json:"gate"`
	PulseCount  int  `json:"pulseCount"`
	Ratchets    int  `json:"ratchets"`
	Probability int  `json:"probability"`
	Slide       bool `json:"slide"`
	GateLength  int  `json:"gateLength"`
	Accumulator int  `json:"accumulator"`
	AccumReset  int  `json:"accumReset"`
	AccumMode   int  `json:"accumMode"`
}

// MetropolixPattern is a Metropolix pattern spec.
//
// Machine holds the dual-buffer of rendered events for this pattern; see the
// DrumPattern doc comment for the dual-buffer contract. Not persisted.
type MetropolixPattern struct {
	Stages    [8]MetropolixStage                 `json:"stages"`
	Length    int                                `json:"length"`
	Mode      PlaybackMode                       `json:"mode"`
	Scale     ScaleType                          `json:"scale"`
	RootNote  uint8                              `json:"rootNote"`
	SlideTime int                                `json:"slideTime"`
	Machine   [2]atomic.Pointer[CompiledPattern] `json:"-"`
}

// Metropolix ConfirmKind values. Zero means "no dialog active"; non-zero
// selects which destructive action the y-key should run. Dialog state is
// transient (json:"-") and only used by the TUI.
const (
	MetropolixConfirmNone         = 0
	MetropolixConfirmClearPattern = 1
)

// Metropolix is the persisted per-track Metropolix device spec. Patterns is
// a pointer array so MetropolixPattern values (with their non-copyable
// atomic fields) stay at stable heap addresses.
//
// Page selects which launchpad view is active:
//
//	0 = main stage editor (rows = note, cols = stage)
//	1 = octave per stage
//	2 = pulse count per stage
//	3 = ratchets per stage
//	4 = probability per stage
//	5 = gate length per stage (plus gate + slide toggles)
//	6 = accumulator (sub-pages via AccumSubPage: 0=value, 1=reset, 2=mode)
//	7 = settings (mode, scale, length, root, slide time)
//
// ConfirmKind / ConfirmMsg implement a tiny modal dialog used to guard
// destructive actions (clear-pattern) in the TUI.
type Metropolix struct {
	Patterns          [NumPatterns]*MetropolixPattern `json:"patterns"`
	EditingPatternIdx int                             `json:"editingPattern"`
	Page              int                             `json:"page"`
	Selected          int                             `json:"selected"`
	AccumSubPage      int                             `json:"accumSubPage"`
	ConfirmKind       int                             `json:"-"`
	ConfirmMsg        string                          `json:"-"`
}

// Validate clamps Metropolix fields into valid ranges. Per-track playback
// state (current / queued pattern) lives on the runtime Cursor and is
// validated by the project, not the device.
func (m *Metropolix) Validate() {
	if m.EditingPatternIdx < 0 {
		m.EditingPatternIdx = 0
	} else if m.EditingPatternIdx > NumPatterns-1 {
		m.EditingPatternIdx = NumPatterns - 1
	}
	if m.Page < 0 {
		m.Page = 0
	} else if m.Page > 7 {
		m.Page = 7
	}
	if m.Selected < 0 {
		m.Selected = 0
	} else if m.Selected > 7 {
		m.Selected = 7
	}
	if m.AccumSubPage < 0 {
		m.AccumSubPage = 0
	} else if m.AccumSubPage > 2 {
		m.AccumSubPage = 2
	}

	for i := range m.Patterns {
		if m.Patterns[i] == nil {
			m.Patterns[i] = &MetropolixPattern{}
		}
		pat := m.Patterns[i]

		if pat.Length < 1 {
			pat.Length = 1
		} else if pat.Length > 8 {
			pat.Length = 8
		}
		if pat.Mode < ModeForward {
			pat.Mode = ModeForward
		} else if pat.Mode > ModeRandom {
			pat.Mode = ModeRandom
		}
		if pat.Scale < 0 {
			pat.Scale = 0
		} else if pat.Scale >= ScaleCount {
			pat.Scale = ScaleCount - 1
		}
		if pat.RootNote > 127 {
			pat.RootNote = 127
		}
		if pat.SlideTime < 1 {
			pat.SlideTime = 1
		} else if pat.SlideTime > 8 {
			pat.SlideTime = 8
		}

		for s := range pat.Stages {
			stage := &pat.Stages[s]
			if stage.Octave < 0 {
				stage.Octave = 0
			} else if stage.Octave > 7 {
				stage.Octave = 7
			}
			if stage.Note < 0 {
				stage.Note = 0
			} else if stage.Note > 7 {
				stage.Note = 7
			}
			if stage.PulseCount < 1 {
				stage.PulseCount = 1
			} else if stage.PulseCount > 8 {
				stage.PulseCount = 8
			}
			if stage.Ratchets < 1 {
				stage.Ratchets = 1
			} else if stage.Ratchets > 8 {
				stage.Ratchets = 8
			}
			if stage.Probability < 0 {
				stage.Probability = 0
			} else if stage.Probability > 100 {
				stage.Probability = 100
			}
			if stage.GateLength < 0 {
				stage.GateLength = 0
			} else if stage.GateLength > 5 {
				stage.GateLength = 5
			}
			if stage.Accumulator < -4 {
				stage.Accumulator = -4
			} else if stage.Accumulator > 3 {
				stage.Accumulator = 3
			}
			if stage.AccumReset < 0 {
				stage.AccumReset = 0
			} else if stage.AccumReset > 8 {
				stage.AccumReset = 8
			}
			if stage.AccumMode < 0 {
				stage.AccumMode = 0
			} else if stage.AccumMode > 2 {
				stage.AccumMode = 2
			}
		}
	}
}
