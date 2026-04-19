package devices

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
type DrumPattern struct {
	Notes  [16]NoteLane `json:"notes"`
	Length int          `json:"length"`
}

// Drum is the persisted per-track drum device spec.
type Drum struct {
	Patterns          [NumPatterns]DrumPattern `json:"patterns"`
	Schedule          Schedule                 `json:"schedule"`
	EditingPatternIdx int                      `json:"editingPattern"`
	SelectedNoteIdx   int                      `json:"selectedNote"`
	Cursor            int                      `json:"cursor"`
	Recording         bool                     `json:"-"`
}

// Validate clamps Drum fields into valid ranges and fills in defaults.
func (d *Drum) Validate() {
	d.Schedule.Validate(NumPatterns)

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
		pat := &d.Patterns[i]
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
