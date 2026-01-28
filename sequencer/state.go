package sequencer

import "time"

// Timing constants
const (
	PPQ = 960 // Pulses (ticks) per quarter note - ~0.5ms resolution at 120 BPM
)

// S is the global state singleton
var S *State

func init() {
	S = NewState()
}

// State is the single source of truth for all application state
type State struct {
	Tempo         int            `json:"tempo"`
	Tracks        [8]*TrackState `json:"tracks"`
	NoteInputPort string         `json:"noteInputPort,omitempty"` // MIDI keyboard input
	ProjectName   string         `json:"-"`                       // runtime only - current project name

	// Runtime timing state (not persisted)
	Playing bool      `json:"-"` // true when playback is active
	T0      time.Time `json:"-"` // wall-clock reference when play started
	Tick    int64     `json:"-"` // current global tick position
}

// TrackState holds all state for a single track
type TrackState struct {
	Name     string     `json:"name"`
	Channel  uint8      `json:"channel"`
	Muted    bool       `json:"muted"`
	Solo     bool       `json:"solo"`
	PortName string     `json:"portName,omitempty"`
	Type     DeviceType `json:"type"`
	Kit      string     `json:"kit,omitempty"` // drum kit mapping ("gm", "rd8", etc.)

	// Device-specific state (only one populated based on Type)
	Drum       *DrumState       `json:"drum,omitempty"`
	Piano      *PianoState      `json:"piano,omitempty"`
	Metropolix *MetropolixState `json:"metropolix,omitempty"`
}

// DrumState holds all state for a drum device
type DrumState struct {
	Patterns [NumPatterns]DrumPatternState `json:"patterns"`

	// Playback
	PlayingPatternIdx int `json:"pattern"`
	Next              int `json:"next"`
	Step              int `json:"-"` // runtime only

	// UI
	SelectedNoteIdx   int `json:"selected"`
	EditingPatternIdx int `json:"editing"`
	Cursor            int `json:"cursor"`

	// Recording
	Recording bool `json:"-"` // runtime only - record input to pattern
	Preview   bool `json:"-"` // runtime only - MIDI thru
}

// DrumPatternState holds pattern data
type DrumPatternState struct {
	Notes [16]DrumNoteState `json:"notes"`
}

// DrumNoteState holds a single drum note lane (one of 16 drum sounds)
type DrumNoteState struct {
	Steps  [32]DrumStepState `json:"steps"`
	Length int               `json:"length"`
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

	// Recording
	Recording bool `json:"-"` // runtime only - record input to pattern
	Preview   bool `json:"-"` // runtime only - MIDI thru
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

// PlaybackMode defines how the Metropolix sequences through stages
type PlaybackMode int

const (
	ModeForward PlaybackMode = iota
	ModeReverse
	ModePendulum
	ModeRandom
)

// ScaleType defines quantization scale
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
	ScaleCount // sentinel for validation
)

// MetropolixState holds ALL state for a Metropolix device.
// Everything persists via JSON - no runtime vs save distinction.
type MetropolixState struct {
	// ─────────── Patterns ───────────
	Patterns [NumPatterns]MetropolixPatternState `json:"patterns"`

	// ─────────── UI/Session ───────────
	Editing      int `json:"editing"`      // Pattern being edited
	Page         int `json:"page"`         // Launchpad page
	Selected     int `json:"selected"`     // Selected stage
	AccumSubPage int `json:"accumSubPage"` // Accum sub-page: 0=value, 1=reset, 2=mode
	Next         int `json:"next"`         // Queued pattern (-1=none)

	// ─────────── Playback Position ───────────
	Pattern     int `json:"pattern"`     // Playing pattern
	Stage       int `json:"stage"`       // Current stage
	StageStep   int `json:"stageStep"`   // Tick within stage
	RatchetStep int `json:"ratchetStep"` // Ratchet counter
	Direction   int `json:"direction"`   // Pendulum: +1/-1

	// ─────────── Note Tracking ───────────
	ActiveNote  uint8 `json:"activeNote"`  // Held note (0=none)
	NoteEndStep int   `json:"noteEndStep"` // Step for note-off (-1=none)

	// ─────────── Slide State ───────────
	Sliding       bool `json:"sliding"`
	SlideProgress int  `json:"slideProgress"`
	SlideStart    int  `json:"slideStart"`
	SlideTarget   int  `json:"slideTarget"`
	SlideDuration int  `json:"slideDuration"`

	// ─────────── Accumulator Runtime (per-stage) ───────────
	Accum      [8]int `json:"accum"`      // Current offset
	AccumCount [8]int `json:"accumCount"` // Triggers toward reset
	AccumDir   [8]int `json:"accumDir"`   // Direction: +1/-1
}

// MetropolixPatternState holds pattern data
type MetropolixPatternState struct {
	Stages [8]MetropolixStageState `json:"stages"`

	// Pattern-level settings
	Length    int          `json:"length"`    // Active stages (1-8)
	Mode      PlaybackMode `json:"mode"`      // FWD, REV, PEND, RAND
	Scale     ScaleType    `json:"scale"`     // Chromatic, Major, etc.
	RootNote  uint8        `json:"rootNote"`  // MIDI note (e.g., 60 = C4)
	SlideTime int          `json:"slideTime"` // Glide duration (1-8)
}

// MetropolixStageState holds a single stage's parameters
type MetropolixStageState struct {
	Octave      int  `json:"octave"`      // 0-7 (4 = middle C area)
	Note        int  `json:"note"`        // Scale degree 0-7 (index into scale)
	Gate        bool `json:"gate"`        // Note on/off
	PulseCount  int  `json:"pulseCount"`  // Clocks per stage (1-8)
	Ratchets    int  `json:"ratchets"`    // Subdivisions (1-8)
	Probability int  `json:"probability"` // 0-100
	Slide       bool `json:"slide"`       // Glide to next stage
	GateLength  int  `json:"gateLength"`  // 0-5 index into gateLengthValues (0=trigger, 5=full)
	Accumulator int  `json:"accumulator"` // Semitones per trigger (-4 to +3)
	AccumReset  int  `json:"accumReset"`  // Reset after N triggers (0 = never)
	AccumMode   int  `json:"accumMode"`   // 0=reset, 1=ping-pong, 2=hold at limit
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
		PlayingPatternIdx: 0,
		Next:              0,
		Step:              0,
		SelectedNoteIdx:   0,
		EditingPatternIdx: 0,
		Cursor:            0,
	}

	for i := range d.Patterns {
		for n := 0; n < 16; n++ {
			d.Patterns[i].Notes[n] = DrumNoteState{
				Length: 16,
			}
			for s := 0; s < 32; s++ {
				d.Patterns[i].Notes[n].Steps[s] = DrumStepState{
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

// NewMetropolixState creates a new Metropolix state with defaults
func NewMetropolixState() *MetropolixState {
	m := &MetropolixState{
		// UI/Session
		Editing:      0,
		Page:         5, // Notes page by default
		Selected:     0,
		AccumSubPage: 0,
		Next:         -1, // No queued pattern

		// Playback Position
		Pattern:     0,
		Stage:       0,
		StageStep:   0,
		RatchetStep: 0,
		Direction:   1,

		// Note Tracking
		ActiveNote:  0,
		NoteEndStep: -1,

		// Slide State
		Sliding:       false,
		SlideProgress: 0,
		SlideStart:    0,
		SlideTarget:   0,
		SlideDuration: 0,

		// Accumulator Runtime
		AccumDir: [8]int{1, 1, 1, 1, 1, 1, 1, 1}, // All directions start positive
	}

	for i := range m.Patterns {
		m.Patterns[i] = MetropolixPatternState{
			Length:    8,
			Mode:      ModeForward,
			Scale:     ScaleMajor,
			RootNote:  60, // C4
			SlideTime: 3,
		}
		for s := 0; s < 8; s++ {
			m.Patterns[i].Stages[s] = MetropolixStageState{
				Octave:      4,     // Middle C area
				Note:        s % 8, // Walk up the scale
				Gate:        true,  // All gates on by default
				PulseCount:  1,     // 1 clock per stage
				Ratchets:    1,     // No ratchets
				Probability: 100,   // Always trigger
				Slide:       false, // No slide
				GateLength:  3,     // 1/4 note by default
				Accumulator: 0,     // No pitch drift
				AccumReset:  0,     // Never reset
				AccumMode:   0,     // Reset mode
			}
		}
	}

	return m
}

// ResetPlayback resets playback position (for transport stop/start)
func (s *MetropolixState) ResetPlayback() {
	s.Stage = 0
	s.StageStep = 0
	s.RatchetStep = 0
	s.Direction = 1
	s.ActiveNote = 0
	s.NoteEndStep = -1
	s.Sliding = false
}

// ResetAccumulators resets all accumulator state (on pattern change)
func (s *MetropolixState) ResetAccumulators() {
	for i := range s.Accum {
		s.Accum[i] = 0
		s.AccumCount[i] = 0
		s.AccumDir[i] = 1
	}
}

// Validate clamps all fields to valid ranges (call after load)
func (s *MetropolixState) Validate() {
	// Clamp top-level state
	s.Editing = clamp(s.Editing, 0, NumPatterns-1)
	s.Page = clamp(s.Page, 0, 7)
	s.Selected = clamp(s.Selected, 0, 7)
	s.AccumSubPage = clamp(s.AccumSubPage, 0, 2)
	s.Pattern = clamp(s.Pattern, 0, NumPatterns-1)
	s.Stage = clamp(s.Stage, 0, 7)
	s.Direction = clamp(s.Direction, -1, 1)
	if s.Direction == 0 {
		s.Direction = 1
	}

	// Clamp Next (-1 = none, 0-15 = pattern)
	if s.Next < -1 || s.Next >= NumPatterns {
		s.Next = -1
	}

	// Clamp patterns
	for i := range s.Patterns {
		pat := &s.Patterns[i]
		pat.Length = clamp(pat.Length, 1, 8)
		pat.Mode = PlaybackMode(clamp(int(pat.Mode), 0, 3))
		pat.Scale = ScaleType(clamp(int(pat.Scale), 0, int(ScaleCount)-1))
		pat.RootNote = uint8(clamp(int(pat.RootNote), 0, 127))
		pat.SlideTime = clamp(pat.SlideTime, 1, 8)

		for j := range pat.Stages {
			stage := &pat.Stages[j]
			stage.Octave = clamp(stage.Octave, 0, 7)
			stage.Note = clamp(stage.Note, 0, 7)
			stage.PulseCount = clamp(stage.PulseCount, 1, 8)
			stage.Ratchets = clamp(stage.Ratchets, 1, 8)
			stage.Probability = clamp(stage.Probability, 0, 100)
			stage.GateLength = clamp(stage.GateLength, 0, 5)
			stage.Accumulator = clamp(stage.Accumulator, -4, 3)
			stage.AccumReset = clamp(stage.AccumReset, 0, 8)
			stage.AccumMode = clamp(stage.AccumMode, 0, 2)
		}
	}

	// Ensure accum directions are initialized
	for i := range s.AccumDir {
		if s.AccumDir[i] == 0 {
			s.AccumDir[i] = 1
		}
	}
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// TickDuration returns the duration of one tick at the current tempo
func (s *State) TickDuration() time.Duration {
	// tickDuration = (60s / BPM) / PPQ
	quarterNote := time.Duration(float64(time.Second) * 60.0 / float64(s.Tempo))
	return quarterNote / PPQ
}

// TickToTime converts a tick number to wall-clock time (relative to T0)
func (s *State) TickToTime(tick int64) time.Time {
	return s.T0.Add(time.Duration(tick) * s.TickDuration())
}

// TimeToTick converts wall-clock time to tick number (relative to T0)
func (s *State) TimeToTick(t time.Time) int64 {
	elapsed := t.Sub(s.T0)
	return int64(elapsed / s.TickDuration())
}

// StepToTick converts a 16th-note step to ticks (PPQ/4 ticks per step)
func StepToTick(step int) int64 {
	return int64(step) * (PPQ / 4)
}

// TickToStep converts ticks to 16th-note step
func TickToStep(tick int64) int {
	return int(tick / (PPQ / 4))
}

// Step returns the current 16th-note step (0-15) derived from global Tick
func (s *State) Step() int {
	return TickToStep(s.Tick) % 16
}

// MasterLength returns the longest note length in a drum pattern
func (p *DrumPatternState) MasterLength() int {
	max := 1
	for i := 0; i < 16; i++ {
		if p.Notes[i].Length > max {
			max = p.Notes[i].Length
		}
	}
	return max
}
