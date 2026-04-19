package drum

import (
	"sort"

	"go-sequence/controller/devices/rng"
	"go-sequence/view/surface"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Package drum is the stateless drum device: a compiler turning a Drum spec
// into TimedEvents, plus input handlers that mutate the spec. All behavior
// lives here; Track.Drum in the model holds only spec data.
//
// Callers (Compiler, input router) hold the Track's mutex; functions here
// never touch it themselves.

// noteDurationTicks is how long each drum NoteOn is held before the matching
// NoteOff. Drum hits are short, so a fixed short duration is fine.
const noteDurationTicks int64 = 60

// Compile renders a single drum pattern into a CompiledPattern.
//
// Pure: no I/O, no mutation of spec, no globals. Same (pattern, kit, seed)
// → same output. The caller picks which pattern to render (the Compiler
// goroutine reads track.Drum.Patterns[req.Pattern] and passes it in), so
// the Schedule.Playing indirection no longer lives here.
//
// Signature diverges from DESIGN.md §5.4: Compile takes an explicit
// `kit devices.Kit` because the kit lives on the Track, not on the Drum
// spec.
func Compile(pat *devices.DrumPattern, kit devices.Kit, seed uint64) devices.CompiledPattern {
	// Rng is built for future use (probability, humanize). Drum currently has
	// no random behavior; keep the seed plumbing in place so probability can
	// be added without changing the signature.
	// TODO(probability): use r when per-step probability lands.
	_ = rng.NewRng(seed)

	if pat == nil {
		return devices.CompiledPattern{Length: 1}
	}

	length := int64(pat.Length)
	if length < 1 {
		length = 1
	}

	ticksPerStep := int64(devices.PPQ / 4)
	// stepsInPattern is how many 16th-note steps fit within pat.Length.
	// Steps beyond this index exist in the spec's fixed [32]DrumStep array
	// but are not played — the pattern's length is the gate.
	stepsInPattern := int(length / ticksPerStep)
	if stepsInPattern > 32 {
		stepsInPattern = 32
	}

	events := make([]devices.TimedEvent, 0, stepsInPattern*4)

	for laneIdx := 0; laneIdx < 16; laneIdx++ {
		lane := &pat.Notes[laneIdx]
		note := kit.Notes[laneIdx]

		for stepIdx := 0; stepIdx < stepsInPattern; stepIdx++ {
			step := lane.Steps[stepIdx]
			if !step.Active {
				continue
			}

			velocity := step.Velocity
			if velocity == 0 {
				velocity = 100
			}

			baseTick := int64(stepIdx) * ticksPerStep
			onTick := baseTick + int64(step.Nudge)
			// Clamp to [0, length). Nudge may push a step off the front or
			// past the end; snap to the pattern instead of dropping it.
			if onTick < 0 {
				onTick = 0
			}
			if onTick >= length {
				onTick = length - 1
			}

			offTick := onTick + noteDurationTicks
			if offTick >= length {
				offTick = length - 1
			}

			events = append(events, devices.TimedEvent{
				Tick: onTick,
				Event: midi.Event{
					Type:     midi.NoteOn,
					Note:     note,
					Velocity: velocity,
				},
			})
			events = append(events, devices.TimedEvent{
				Tick: offTick,
				Event: midi.Event{
					Type:     midi.NoteOff,
					Note:     note,
					Velocity: 0,
				},
			})
		}
	}

	// Stable sort so equal-tick events keep insertion order (NoteOn before
	// NoteOff when they collide at the pattern boundary, etc.).
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return devices.CompiledPattern{
		Events: events,
		Length: length,
	}
}

// HandlePad handles a pad press or release on the drum view.
//
// Signature diverges from DESIGN.md §5.4: takes `*model.Track` rather than
// `*model.Drum` so the handler can reach the track's PortName, Channel, and
// Kit for preview MIDI sends. Drum methods access `track.Drum` internally.
// Releases (down == false) are no-ops for now — the drum device is
// press-only today.
func HandlePad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]

	// Top 4 rows (rows 4-7): step toggle for the selected lane.
	if row >= 4 && row <= 7 {
		stepIdx := (7-row)*8 + col
		if stepIdx < 0 || stepIdx >= 32 {
			return
		}
		toggleStep(pat, state.SelectedNoteIdx, stepIdx)
		state.Cursor = stepIdx
		return
	}

	// Bottom-left 4x4: note-lane select (and recording/preview pad hits).
	if row < 4 && col < 4 {
		laneIdx := row*4 + col
		if laneIdx < 0 || laneIdx >= 16 {
			return
		}
		state.SelectedNoteIdx = laneIdx
		if state.Cursor >= 32 {
			state.Cursor = 31
		}

		// Preview: send the lane's drum note directly. No queueing, no
		// compile — the hit is audible immediately.
		kit := devices.GetKit(track.Kit)
		note := kit.Notes[laneIdx]
		_ = out.Send(track.PortName, track.Channel, midi.Event{
			Type:     midi.NoteOn,
			Note:     note,
			Velocity: 100,
		})

		// Record-arm + pad tap = write this hit into the editing pattern at
		// the cursor position. (Old behavior used currentStep at live tick;
		// we don't have playback position here, so use the cursor as the
		// write head. Revisit in step 4 when we can read playback tick.)
		if state.Recording {
			setStep(pat, laneIdx, state.Cursor, 100)
		}
		return
	}

	// Bottom-right 4x4: command pads.
	if row < 4 && col >= 4 {
		switch {
		case row == 0 && col == 4: // clear lane
			clearLane(pat, state.SelectedNoteIdx)
		case row == 0 && col == 5: // clear pattern
			clearPattern(pat)
		case row == 3 && col == 5: // record toggle
			state.Recording = !state.Recording
		}
		return
	}
}

// HandleKey handles a keyboard event on the drum view.
//
// Signature diverges from DESIGN.md §5.4: takes `*model.Track` (see HandlePad).
func HandleKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	pat := state.Patterns[state.EditingPatternIdx]

	switch key {
	case "h", "left":
		if state.Cursor > 0 {
			state.Cursor--
		}
	case "l", "right":
		if state.Cursor < 31 {
			state.Cursor++
		}
	case "j", "down":
		if state.SelectedNoteIdx < 15 {
			state.SelectedNoteIdx++
		}
	case "k", "up":
		if state.SelectedNoteIdx > 0 {
			state.SelectedNoteIdx--
		}
	case " ":
		toggleStep(pat, state.SelectedNoteIdx, state.Cursor)
	case "c":
		clearLane(pat, state.SelectedNoteIdx)
	case "C":
		clearPattern(pat)
	case "<", ",":
		if state.EditingPatternIdx > 0 {
			state.EditingPatternIdx--
		}
	case ">", ".":
		if state.EditingPatternIdx < devices.NumPatterns-1 {
			state.EditingPatternIdx++
		}
	case "r", "R":
		state.Recording = !state.Recording
	}
}

// HandleMIDI records incoming MIDI hits into the editing pattern when
// recording is armed.
//
// Signature diverges from DESIGN.md §5.4: takes `*model.Track` (see HandlePad).
func HandleMIDI(track *model.Track, out midi.ToExternal, ev midi.Event) {
	if track == nil || track.Drum == nil {
		return
	}
	state := track.Drum
	if !state.Recording {
		return
	}
	if ev.Type != midi.NoteOn || ev.Velocity == 0 {
		return
	}

	// Reverse-lookup MIDI note → lane index via the kit.
	kit := devices.GetKit(track.Kit)
	laneIdx := -1
	for i, n := range kit.Notes {
		if n == ev.Note {
			laneIdx = i
			break
		}
	}
	if laneIdx < 0 {
		return
	}

	// We don't have the live playback tick here (not plumbed yet). Write at
	// the cursor and advance — matches the pad-recording shortcut above.
	// Revisit in step 4 when playback tick is available to the input path.
	pat := state.Patterns[state.EditingPatternIdx]
	setStep(pat, laneIdx, state.Cursor, ev.Velocity)
}

// Render is a stub; step 5 will port the TUI view from sequencer/drum.go.
// TODO(step5): port rendering from sequencer/drum.go when view/ rewires.
func Render(track *model.Track, project *model.Project) string { return "" }

// RenderLEDs is a stub; step 5 will port the LED layout from sequencer/drum.go.
// TODO(step5): port rendering from sequencer/drum.go when view/ rewires.
func RenderLEDs(track *model.Track, project *model.Project) []surface.LED { return nil }

// --- spec mutators (package-private; callers hold track.mu) ---

// toggleStep flips a step's Active bit, resetting velocity when turning on.
func toggleStep(pat *devices.DrumPattern, lane, step int) {
	if lane < 0 || lane >= 16 || step < 0 || step >= 32 {
		return
	}
	s := &pat.Notes[lane].Steps[step]
	s.Active = !s.Active
	if s.Active && s.Velocity == 0 {
		s.Velocity = 100
	}
}

// setStep forces a step Active with the given velocity.
func setStep(pat *devices.DrumPattern, lane, step int, velocity uint8) {
	if lane < 0 || lane >= 16 || step < 0 || step >= 32 {
		return
	}
	s := &pat.Notes[lane].Steps[step]
	s.Active = true
	s.Velocity = velocity
}

// clearLane zeroes every step in one lane.
func clearLane(pat *devices.DrumPattern, lane int) {
	if lane < 0 || lane >= 16 {
		return
	}
	for i := range pat.Notes[lane].Steps {
		pat.Notes[lane].Steps[i] = devices.DrumStep{}
	}
}

// clearPattern zeroes every step in every lane.
func clearPattern(pat *devices.DrumPattern) {
	for l := range pat.Notes {
		for s := range pat.Notes[l].Steps {
			pat.Notes[l].Steps[s] = devices.DrumStep{}
		}
	}
}
