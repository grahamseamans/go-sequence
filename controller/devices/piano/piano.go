package piano

import (
	"sort"

	"go-sequence/controller/devices/rng"
	"go-sequence/view/surface"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Package piano is the stateless piano-roll device: a compiler turning a Piano
// spec into TimedEvents, plus input handlers that mutate the spec. All
// behavior lives here; Track.Piano in the model holds only spec data.
//
// Callers (Compiler, input router) hold the Track's mutex; functions here
// never touch it themselves.

// --- view/edit constants ported from sequencer/pianoroll.go ---
//
// These drive the piano roll's viewport (zoom, row density) and per-edit step
// size (how far "move note" nudges in beats / semitones). They're
// package-level because they're lookup tables indexed by the spec's
// ViewScale / EditHoriz / EditVert ints.

// pianoViewScales is beats-per-column for each zoom level (index = ViewScale).
var pianoViewScales = []float64{
	0.03125, // 1/32 per col — super zoomed in
	0.0625,  // 1/16 per col
	0.125,   // 1/8 per col
	0.25,    // 1/4 per col
	0.5,     // 1/2 per col
	1.0,     // 1 beat per col
	2.0,     // 2 beats per col
	4.0,     // 4 beats per col — zoomed out
}

// pianoEditHorizSteps is the beat delta for each horizontal-edit sensitivity.
var pianoEditHorizSteps = []float64{
	0.015625, // 1/64
	0.03125,  // 1/32
	0.0625,   // 1/16
	0.125,    // 1/8
	0.25,     // 1/4
	0.5,      // 1/2
	1.0,      // 1 beat
}

// pianoEditVertSteps is the semitone delta for each vertical-edit sensitivity.
var pianoEditVertSteps = []int{1, 12}

// PianoViewSmushed / PianoViewSpread are the two vertical row densities for
// the TUI piano roll (number of pitch rows rendered).
const (
	PianoViewSmushed = 12
	PianoViewSpread  = 24
)

// minNoteDurationBeats is the smallest duration a note may have. Matches the
// old pianoroll's hard floor to avoid zero-length notes.
const minNoteDurationBeats = 0.25

// Compile renders a single piano pattern into a CompiledPattern.
//
// Pure: no I/O, no mutation of spec, no globals. Same (pattern, seed) →
// same output. The caller (Compiler goroutine) picks which pattern to
// render — the Schedule.Playing indirection no longer lives here.
//
// Tick math: note.Start and note.Duration are beats (float64); PPQ=960 gives
// `tick = int64(beat * PPQ)`. Off-ticks that extend past the pattern end are
// clamped to `length - 1` (matches drum.go's boundary convention); the loop
// wrap will then send the NoteOff slightly before the NoteOn on the next
// iteration, which is the intended short-tail behavior.
func Compile(pat *devices.PianoPattern, seed uint64) devices.CompiledPattern {
	// Rng is built for future use (probability, humanize). Piano currently has
	// no random behavior; keep the seed plumbing in place so probability can
	// be added without changing the signature.
	// TODO(probability): use r when per-note probability lands.
	_ = rng.NewRng(seed)

	if pat == nil {
		return devices.CompiledPattern{Length: 1}
	}

	length := int64(pat.Length * float64(devices.PPQ))
	if length < 1 {
		length = 1
	}

	events := make([]devices.TimedEvent, 0, len(pat.Notes)*2)

	for i := range pat.Notes {
		n := &pat.Notes[i]

		onTick := int64(n.Start * float64(devices.PPQ))
		if onTick < 0 {
			onTick = 0
		}
		if onTick >= length {
			onTick = length - 1
		}

		offTick := int64((n.Start + n.Duration) * float64(devices.PPQ))
		if offTick <= onTick {
			offTick = onTick + 1
		}
		if offTick >= length {
			offTick = length - 1
		}

		events = append(events, devices.TimedEvent{
			Tick: onTick,
			Event: midi.Event{
				Type:     midi.NoteOn,
				Note:     n.Pitch,
				Velocity: n.Velocity,
				Channel:  0,
			},
		})
		events = append(events, devices.TimedEvent{
			Tick: offTick,
			Event: midi.Event{
				Type:     midi.NoteOff,
				Note:     n.Pitch,
				Velocity: 0,
				Channel:  0,
			},
		})
	}

	// Stable sort so equal-tick events keep insertion order (NoteOn before
	// NoteOff when a zero-length note collapses onto one tick, etc.).
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Tick < events[j].Tick
	})

	return devices.CompiledPattern{
		Events: events,
		Length: length,
	}
}

// HandlePad handles a pad press or release on the piano view.
//
// Signature matches drum.go: takes `*model.Track` to reach PortName/Channel
// for preview MIDI sends. Releases (down == false) are no-ops.
//
// Semantics (ported from sequencer/pianoroll.go): the 8x8 grid renders a
// window into the piano roll centered on (CenterBeat, CenterPitch) at the
// current ViewScale zoom. A tap on a cell either selects an existing note
// under that cell or adds a new note at that beat/pitch.
//
// TODO(piano-play-mode): the refactor plan mentions "edit mode vs play mode"
// — currently the source has no such split, so all taps behave as edit. Add
// a play-mode toggle + direct out.Send preview when the model gets a
// Piano.Mode field.
func HandlePad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]

	basePitch := int(state.CenterPitch) - 4
	viewScale := pianoViewScale(state.ViewScale)
	startBeat := state.CenterBeat - 4*viewScale

	// Pad (0,0) is top-left on the Launchpad grid; row increases downward.
	// The old code mapped row → pitch as `basePitch + row`, treating the
	// bottom-left as the lowest pitch. Port as-is.
	pitch := uint8(basePitch + row)
	beat := startBeat + float64(col)*viewScale
	beatEnd := beat + viewScale

	if beat < 0 || beat >= pat.Length || int(basePitch+row) < 0 || int(basePitch+row) > 127 {
		return
	}

	// Cell hit existing note → select it.
	for i := range pat.Notes {
		n := &pat.Notes[i]
		if n.Pitch != pitch {
			continue
		}
		noteEnd := n.Start + n.Duration
		if n.Start < beatEnd && noteEnd > beat {
			state.SelectedNote = i
			centerPianoOnSelection(state)
			return
		}
	}

	// Empty cell → add a new note the width of one column, clamped to the
	// minimum duration.
	dur := viewScale
	if dur < minNoteDurationBeats {
		dur = minNoteDurationBeats
	}
	addPianoNote(pat, beat, dur, pitch, 100)
	state.SelectedNote = len(pat.Notes) - 1
	sortPianoNotes(pat, state)
	centerPianoOnSelection(state)
}

// HandleKey handles a keyboard event on the piano view. Mutates spec only;
// caller holds track.mu.
//
// Keybindings ported verbatim from sequencer/pianoroll.go.HandleKey. If a
// binding looks surprising, that's the old behavior — don't invent new ones
// here.
func HandleKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	pat := state.Patterns[state.EditingPatternIdx]
	editH := pianoEditHoriz(state.EditHoriz)
	editV := pianoEditVert(state.EditVert)

	switch key {
	case "h", "left":
		selectPianoNoteByTime(state, -1)
	case "l", "right":
		selectPianoNoteByTime(state, 1)
	case "j", "down":
		selectPianoNoteByPitch(state, -1)
	case "k", "up":
		selectPianoNoteByPitch(state, 1)

	case "y":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			delta := -editH
			if n.Start+delta < 0 {
				delta = -n.Start
			}
			movePianoNoteTime(pat, state.SelectedNote, delta)
			centerPianoOnSelection(state)
		}
	case "o":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			delta := editH
			if n.Start+n.Duration+delta > pat.Length {
				delta = pat.Length - n.Start - n.Duration
			}
			movePianoNoteTime(pat, state.SelectedNote, delta)
			centerPianoOnSelection(state)
		}
	case "u":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			movePianoNotePitch(pat, state.SelectedNote, -editV)
			centerPianoOnSelection(state)
		}
	case "i":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			movePianoNotePitch(pat, state.SelectedNote, editV)
			centerPianoOnSelection(state)
		}

	case "n":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			if n.Duration > editH {
				setPianoNoteDuration(pat, state.SelectedNote, n.Duration-editH)
			}
		}
	case "m":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			n := &pat.Notes[state.SelectedNote]
			if n.Start+n.Duration+editH <= pat.Length {
				setPianoNoteDuration(pat, state.SelectedNote, n.Duration+editH)
			}
		}

	case "q":
		if state.ViewScale < len(pianoViewScales)-1 {
			state.ViewScale++
		}
	case "w":
		if state.ViewScale > 0 {
			state.ViewScale--
		}
	case "a":
		state.ViewRows = PianoViewSmushed
	case "s":
		state.ViewRows = PianoViewSpread

	case "d":
		if state.EditHoriz < len(pianoEditHorizSteps)-1 {
			state.EditHoriz++
		}
	case "f":
		if state.EditHoriz > 0 {
			state.EditHoriz--
		}
	case "e":
		if state.EditVert < len(pianoEditVertSteps)-1 {
			state.EditVert++
		}
	case "r":
		if state.EditVert > 0 {
			state.EditVert--
		}

	case " ":
		dur := pianoEditHoriz(state.EditHoriz) * 4
		addPianoNote(pat, state.CenterBeat, dur, uint8(state.CenterPitch), 100)
		state.SelectedNote = len(pat.Notes) - 1
		centerPianoOnSelection(state)

	case "x":
		if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
			deletePianoNote(pat, state.SelectedNote)
			if state.SelectedNote >= len(pat.Notes) {
				state.SelectedNote = len(pat.Notes) - 1
			}
			if state.SelectedNote >= 0 {
				centerPianoOnSelection(state)
			}
		}

	case "[":
		if pat.Length > 1.0 {
			setPianoPatternLength(pat, pat.Length-1.0)
		}
	case "]":
		if pat.Length < 64.0 {
			setPianoPatternLength(pat, pat.Length+1.0)
		}

	case "c":
		clearPianoPattern(pat)
		state.SelectedNote = -1

	case "<":
		if state.EditingPatternIdx > 0 {
			state.EditingPatternIdx--
			state.SelectedNote = -1
		}
	case ">", ".":
		if state.EditingPatternIdx < devices.NumPatterns-1 {
			state.EditingPatternIdx++
			state.SelectedNote = -1
		}

	case "R":
		// Capital-R toggles recording. Lowercase 'r' is bound to edit-vert
		// sensitivity above, matching the old pianoroll binding.
		state.Recording = !state.Recording
	}

	sortPianoNotes(pat, state)
}

// HandleMIDI records incoming MIDI notes into the editing pattern when
// recording is armed. Drops the old pending-note-on/off pairing and the live
// quantize-from-tick math — we don't have a playback tick in the handler
// path yet. The recorded Start is the current CenterBeat, the Duration is
// the current edit-horizontal size. Crude but deterministic.
//
// TODO(piano-record-tick): when the input router gets access to the live
// playback tick, restore the quantize-to-beat behavior and the
// NoteOn→NoteOff pairing for accurate durations.
func HandleMIDI(track *model.Track, out midi.ToExternal, ev midi.Event) {
	if track == nil || track.Piano == nil {
		return
	}
	state := track.Piano
	if !state.Recording {
		return
	}
	if ev.Type != midi.NoteOn || ev.Velocity == 0 {
		return
	}

	pat := state.Patterns[state.EditingPatternIdx]
	dur := pianoEditHoriz(state.EditHoriz) * 4
	if dur < minNoteDurationBeats {
		dur = minNoteDurationBeats
	}

	start := state.CenterBeat
	if start < 0 {
		start = 0
	}
	if start+dur > pat.Length {
		// Clamp so we don't append a note that Compile would have to clip.
		if pat.Length >= dur {
			start = pat.Length - dur
		} else {
			start = 0
			dur = pat.Length
		}
	}

	pat.Notes = append(pat.Notes, devices.NoteEvent{
		Start:    start,
		Duration: dur,
		Pitch:    ev.Note,
		Velocity: ev.Velocity,
	})
	sortPianoNotes(pat, state)
}

// Render is a stub; step 5 will port the TUI view from sequencer/pianoroll.go.
// TODO(step5): port rendering from sequencer/pianoroll.go when view/ rewires.
func Render(track *model.Track, project *model.Project) string { return "" }

// RenderLEDs is a stub; step 5 will port the LED layout from sequencer/pianoroll.go.
// TODO(step5): port rendering from sequencer/pianoroll.go when view/ rewires.
func RenderLEDs(track *model.Track, project *model.Project) []surface.LED { return nil }

// --- spec accessors (safe index into the lookup tables) ---

// pianoViewScale returns beats-per-column for the given ViewScale index,
// clamped into range.
func pianoViewScale(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoViewScales) {
		idx = len(pianoViewScales) - 1
	}
	return pianoViewScales[idx]
}

// pianoEditHoriz returns the beat delta for the given EditHoriz index, clamped.
func pianoEditHoriz(idx int) float64 {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditHorizSteps) {
		idx = len(pianoEditHorizSteps) - 1
	}
	return pianoEditHorizSteps[idx]
}

// pianoEditVert returns the semitone delta for the given EditVert index, clamped.
func pianoEditVert(idx int) int {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(pianoEditVertSteps) {
		idx = len(pianoEditVertSteps) - 1
	}
	return pianoEditVertSteps[idx]
}

// --- spec mutators (package-private; callers hold track.mu) ---

// addPianoNote appends a note to the pattern, clamping to pattern bounds.
// Drops the note if it can't fit; no silent truncation. (If the user asked
// for a note off the pattern end, that's a UI bug, not something we hide.)
func addPianoNote(pat *devices.PianoPattern, start, duration float64, pitch, velocity uint8) {
	if duration < minNoteDurationBeats {
		duration = minNoteDurationBeats
	}
	if start < 0 {
		start = 0
	}
	if start+duration > pat.Length {
		return
	}
	pat.Notes = append(pat.Notes, devices.NoteEvent{
		Start: start, Duration: duration, Pitch: pitch, Velocity: velocity,
	})
}

// deletePianoNote removes the note at idx. No-op for out-of-range indices.
func deletePianoNote(pat *devices.PianoPattern, idx int) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	pat.Notes = append(pat.Notes[:idx], pat.Notes[idx+1:]...)
}

// movePianoNoteTime shifts a note's start by deltaBeat, clamped to pattern.
func movePianoNoteTime(pat *devices.PianoPattern, idx int, deltaBeat float64) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	n.Start += deltaBeat
	if n.Start < 0 {
		n.Start = 0
	}
	if n.Start+n.Duration > pat.Length {
		n.Start = pat.Length - n.Duration
	}
}

// movePianoNotePitch shifts a note's pitch by deltaSemitones, clamped to [0,127].
func movePianoNotePitch(pat *devices.PianoPattern, idx int, deltaSemitones int) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	target := int(n.Pitch) + deltaSemitones
	if target < 0 {
		target = 0
	}
	if target > 127 {
		target = 127
	}
	n.Pitch = uint8(target)
}

// setPianoNoteDuration updates a note's duration, clamped to pattern end and
// the minimum duration floor.
func setPianoNoteDuration(pat *devices.PianoPattern, idx int, duration float64) {
	if idx < 0 || idx >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[idx]
	if duration < minNoteDurationBeats {
		duration = minNoteDurationBeats
	}
	if n.Start+duration > pat.Length {
		duration = pat.Length - n.Start
	}
	n.Duration = duration
}

// setPianoPatternLength updates the pattern length in beats, clamped to [1,64].
func setPianoPatternLength(pat *devices.PianoPattern, beats float64) {
	if beats < 1.0 || beats > 64.0 {
		return
	}
	pat.Length = beats
}

// clearPianoPattern drops every note in the editing pattern.
func clearPianoPattern(pat *devices.PianoPattern) {
	pat.Notes = pat.Notes[:0]
}

// --- selection / viewport helpers ---

// centerPianoOnSelection scrolls the viewport to show the selected note.
func centerPianoOnSelection(state *devices.Piano) {
	pat := state.Patterns[state.EditingPatternIdx]
	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		return
	}
	n := &pat.Notes[state.SelectedNote]
	state.CenterBeat = n.Start
	state.CenterPitch = float64(n.Pitch)
}

// selectPianoNoteByTime moves SelectedNote forward/backward through the
// time-sorted note list, wrapping at the ends.
func selectPianoNoteByTime(state *devices.Piano, direction int) {
	pat := state.Patterns[state.EditingPatternIdx]
	if len(pat.Notes) == 0 {
		return
	}
	state.SelectedNote += direction
	if state.SelectedNote < 0 {
		state.SelectedNote = len(pat.Notes) - 1
	} else if state.SelectedNote >= len(pat.Notes) {
		state.SelectedNote = 0
	}
	centerPianoOnSelection(state)
}

// selectPianoNoteByPitch walks up/down pitch looking for the nearest-in-time
// note at each pitch step. Ports the old "jump to closest at next pitch"
// heuristic from sequencer/pianoroll.go.
func selectPianoNoteByPitch(state *devices.Piano, direction int) {
	pat := state.Patterns[state.EditingPatternIdx]
	if len(pat.Notes) == 0 {
		return
	}

	if state.SelectedNote < 0 || state.SelectedNote >= len(pat.Notes) {
		state.SelectedNote = 0
		centerPianoOnSelection(state)
		return
	}

	current := pat.Notes[state.SelectedNote]
	targetPitch := int(current.Pitch) + direction

	bestIdx := -1
	bestDist := 1000.0
	for searchPitch := targetPitch; searchPitch >= 0 && searchPitch <= 127; searchPitch += direction {
		for i, n := range pat.Notes {
			if int(n.Pitch) != searchPitch {
				continue
			}
			dist := n.Start - current.Start
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			break
		}
		if direction == 0 {
			break // defensive: direction must be ±1, but don't infinite loop
		}
	}

	if bestIdx >= 0 {
		state.SelectedNote = bestIdx
		centerPianoOnSelection(state)
	}
}

// sortPianoNotes keeps the pattern's Notes sorted by start time ascending,
// preserving the user's current selection by matching start+pitch after the
// sort. Ported from sequencer/pianoroll.go.
func sortPianoNotes(pat *devices.PianoPattern, state *devices.Piano) {
	var selected *devices.NoteEvent
	if state.SelectedNote >= 0 && state.SelectedNote < len(pat.Notes) {
		n := pat.Notes[state.SelectedNote]
		selected = &n
	}

	sort.SliceStable(pat.Notes, func(i, j int) bool {
		return pat.Notes[i].Start < pat.Notes[j].Start
	})

	if selected == nil {
		return
	}
	for i, n := range pat.Notes {
		if n.Start == selected.Start && n.Pitch == selected.Pitch {
			state.SelectedNote = i
			return
		}
	}
}
