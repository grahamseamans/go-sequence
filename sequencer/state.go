package sequencer

// S is the global state singleton
var S *State

func init() {
	S = NewState()
}

// State is the single source of truth for all application state
type State struct {
	Tempo   int            `json:"tempo"`
	Step    int            `json:"step"`
	Playing bool           `json:"playing"`
	Tracks  [8]*TrackState `json:"tracks"`
}

// TrackState holds all state for a single track
type TrackState struct {
	Name     string     `json:"name"`
	Channel  uint8      `json:"channel"`
	Muted    bool       `json:"muted"`
	Solo     bool       `json:"solo"`
	PortName string     `json:"portName,omitempty"`
	Type     DeviceType `json:"type"`

	// Device-specific state (only one populated based on Type)
	Drum  *DrumState  `json:"drum,omitempty"`
	Piano *PianoState `json:"piano,omitempty"`
}

// DrumState holds all state for a drum device
type DrumState struct {
	Patterns [NumPatterns]DrumPatternState `json:"patterns"`

	// Playback
	Pattern int `json:"pattern"`
	Next    int `json:"next"`
	Step    int `json:"-"` // runtime only

	// UI
	Selected int `json:"selected"`
	Editing  int `json:"editing"`
	Cursor   int `json:"cursor"`
}

// DrumPatternState holds pattern data
type DrumPatternState struct {
	Tracks [16]DrumTrackState `json:"tracks"`
}

// DrumTrackState holds a single drum track
type DrumTrackState struct {
	Steps  [32]DrumStepState `json:"steps"`
	Length int               `json:"length"`
	Note   uint8             `json:"note"`
}

// DrumStepState holds a single step
type DrumStepState struct {
	Active   bool  `json:"active"`
	Velocity uint8 `json:"velocity"`
	Nudge    int8  `json:"nudge"`
}

// PianoState holds all state for a piano roll device
type PianoState struct {
	Patterns [NumPatterns]PianoPatternState `json:"patterns"`

	// Playback
	Pattern  int     `json:"pattern"`
	Next     int     `json:"next"`
	Step     int     `json:"-"` // runtime only
	LastBeat float64 `json:"-"` // runtime only

	// UI
	Editing int `json:"editing"` // which pattern we're editing

	// Viewport
	CenterBeat  float64 `json:"centerBeat"`
	CenterPitch float64 `json:"centerPitch"`
	ViewScale   int     `json:"viewScale"`
	ViewRows    int     `json:"viewRows"`
	EditHoriz   int     `json:"editHoriz"`
	EditVert    int     `json:"editVert"`

	// Selection
	SelectedNote int `json:"selectedNote"`
}

// PianoPatternState holds pattern data
type PianoPatternState struct {
	Notes  []NoteEventState `json:"notes"`
	Length float64          `json:"length"`
}

// NoteEventState holds a single note
type NoteEventState struct {
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Pitch    uint8   `json:"pitch"`
	Velocity uint8   `json:"velocity"`
}

// NewState creates a new state with defaults
func NewState() *State {
	s := &State{
		Tempo: 120,
	}

	// Initialize all 8 tracks
	for i := 0; i < 8; i++ {
		s.Tracks[i] = &TrackState{
			Name:    "",
			Channel: uint8(i + 1),
			Type:    DeviceTypeNone,
		}
	}

	return s
}

// NewDrumState creates a new drum state with defaults
func NewDrumState() *DrumState {
	d := &DrumState{
		Pattern:  0,
		Next:     0,
		Step:     0,
		Selected: 0,
		Editing:  0,
		Cursor:   0,
	}

	// GM drum notes
	gmNotes := []uint8{
		36, 38, 42, 46, 41, 43, 45, 49,
		51, 39, 56, 75, 54, 69, 70, 37,
	}

	for i := range d.Patterns {
		for t := 0; t < 16; t++ {
			d.Patterns[i].Tracks[t] = DrumTrackState{
				Length: 16,
				Note:   gmNotes[t],
			}
			for s := 0; s < 32; s++ {
				d.Patterns[i].Tracks[t].Steps[s] = DrumStepState{
					Active:   false,
					Velocity: 100,
					Nudge:    0,
				}
			}
		}
	}

	return d
}

// NewPianoState creates a new piano state with defaults
func NewPianoState() *PianoState {
	p := &PianoState{
		Pattern:      0,
		Next:         0,
		Step:         0,
		CenterBeat:   2.0,
		CenterPitch:  60.0,
		ViewScale:    2,
		ViewRows:     ViewSpread,
		EditHoriz:    2,
		EditVert:     0,
		SelectedNote: -1,
	}

	for i := range p.Patterns {
		p.Patterns[i] = PianoPatternState{
			Notes:  []NoteEventState{},
			Length: 4.0,
		}
	}

	return p
}

// MasterLength returns the longest track length in a drum pattern
func (p *DrumPatternState) MasterLength() int {
	max := 1
	for i := 0; i < 16; i++ {
		if p.Tracks[i].Length > max {
			max = p.Tracks[i].Length
		}
	}
	return max
}
