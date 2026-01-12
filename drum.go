package main

import "fmt"

type DrumStep struct {
	Active   bool
	Note     uint8
	Velocity uint8
}

type DrumPattern struct {
	Steps  [16]DrumStep
	Length int // 1-16, defaults to 16
}

type DrumDevice struct {
	controller *Controller
	patterns   []*DrumPattern
	pattern    int // currently playing
	next       int // queued (defaults to pattern)
	step       int // internal step counter

	// UI state
	cursor int
}

func NewDrumDevice(controller *Controller) *DrumDevice {
	d := &DrumDevice{
		controller: controller,
		patterns:   make([]*DrumPattern, NumPatterns),
		pattern:    0,
		next:       0,
		step:       0,
		cursor:     0,
	}

	// Initialize patterns
	for i := range d.patterns {
		d.patterns[i] = &DrumPattern{Length: 16}
		for j := range d.patterns[i].Steps {
			d.patterns[i].Steps[j] = DrumStep{
				Active:   false,
				Note:     36, // kick drum
				Velocity: 100,
			}
		}
	}

	return d
}

// Device interface implementation

func (d *DrumDevice) Tick(step int) []MIDIEvent {
	// Pattern switch at own loop boundary
	if d.step == 0 {
		d.pattern = d.next
	}

	pat := d.patterns[d.pattern]
	s := pat.Steps[d.step]

	var events []MIDIEvent
	if s.Active {
		events = append(events, MIDIEvent{
			Type:     NoteOn,
			Note:     s.Note,
			Velocity: s.Velocity,
		})
	}

	d.step = (d.step + 1) % pat.Length
	return events
}

func (d *DrumDevice) QueuePattern(p int) (pattern, next int) {
	if p >= 0 && p < len(d.patterns) {
		d.next = p
	}
	return d.pattern, d.next
}

func (d *DrumDevice) GetState() (pattern, next int) {
	return d.pattern, d.next
}

func (d *DrumDevice) ContentMask() []bool {
	mask := make([]bool, NumPatterns)
	for i, pat := range d.patterns {
		for _, step := range pat.Steps {
			if step.Active {
				mask[i] = true
				break
			}
		}
	}
	return mask
}

func (d *DrumDevice) HandleMIDI(event MIDIEvent) {
	// TODO: record mode - quantize incoming hits to steps
}

func (d *DrumDevice) View() string {
	pat := d.patterns[d.pattern]

	// Build step display
	var cells string
	for i := 0; i < pat.Length; i++ {
		s := pat.Steps[i]
		char := "·"
		if s.Active {
			char = "●"
		}
		if i == d.step {
			char = "▶"
		}
		if i == d.cursor {
			cells += fmt.Sprintf("[%s]", char)
		} else {
			cells += fmt.Sprintf(" %s ", char)
		}
	}

	return fmt.Sprintf("DRUM pattern:%d next:%d step:%d\n%s\n", d.pattern, d.next, d.step, cells)
}

func (d *DrumDevice) HandleKey(key string) {
	pat := d.patterns[d.pattern]

	switch key {
	case "h", "left":
		if d.cursor > 0 {
			d.cursor--
		}
	case "l", "right":
		if d.cursor < pat.Length-1 {
			d.cursor++
		}
	case " ":
		pat.Steps[d.cursor].Active = !pat.Steps[d.cursor].Active
	case "j", "down":
		if pat.Steps[d.cursor].Note > 0 {
			pat.Steps[d.cursor].Note--
		}
	case "k", "up":
		if pat.Steps[d.cursor].Note < 127 {
			pat.Steps[d.cursor].Note++
		}
	}
}

func (d *DrumDevice) HandlePad(row, col int) {
	// Bottom 2 rows = steps 0-15
	if row < 2 {
		step := row*8 + col
		pat := d.patterns[d.pattern]
		if step < pat.Length {
			pat.Steps[step].Active = !pat.Steps[step].Active
			d.cursor = step
		}
	}
}

func (d *DrumDevice) UpdateLEDs() {
	if d.controller == nil {
		return
	}

	pat := d.patterns[d.pattern]

	// Bottom 2 rows show steps
	for i := 0; i < pat.Length; i++ {
		row := i / 8
		col := i % 8
		s := pat.Steps[i]

		var color uint8 = ColorOff
		var channel uint8 = 0

		if i == d.step {
			color = ColorYellow
			channel = 2 // pulsing
		} else if s.Active {
			color = ColorGreen
		}

		d.controller.SetPad(row, col, color, channel)
	}
}
