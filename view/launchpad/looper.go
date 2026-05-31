package launchpad

import (
	looperctl "go-sequence/controller/devices/looper"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Looper view colors.
//
// Grid: chromatic-fourths keyboard. Root notes (note%12==0) get a brighter
// shade; every other note is dim. RGB values are chosen to map cleanly
// through hw.go's mapRGBToLaunchpad palette.
//
// Outer ring (top row at Row:8, right column at Col:8) holds the controls,
// mirroring the layout in DESIGN/looper.md.
var (
	looperRoot = LED{R: 0, G: 100, B: 255} // root note (bright blue)
	looperKey  = LED{R: 40, G: 60, B: 120} // non-root grid key (dim blue)

	looperRecDim   = LED{R: 180, G: 60, B: 60} // record disarmed
	looperRecOn    = LED{R: 255, G: 0, B: 0}   // record armed
	looperDubDim   = LED{R: 180, G: 80, B: 40} // overdub disarmed
	looperDubOn    = LED{R: 255, G: 100, B: 0} // overdub armed
	looperClear    = LED{R: 255, G: 0, B: 0}   // clear
	looperQuantDim = LED{R: 0, G: 100, B: 0}   // quantize off
	looperQuantOn  = LED{R: 0, G: 255, B: 0}   // quantize on
	looperOctave   = LED{R: 0, G: 200, B: 200} // octave -/+
	looperLoopLen  = LED{R: 150, G: 0, B: 200} // loop length -/+
)

// looperPad handles a pad press or release on the looper view.
//
// Layout (DESIGN/looper.md):
//
//	rows 0-7, cols 0-7 (8x8 grid): chromatic-fourths keyboard. Press plays /
//	                    records a NoteOn, release a NoteOff. Both edges matter
//	                    so note lengths are captured — do NOT early-return on
//	                    release here.
//	                    note = 60 + 12*Octave + 5*row + col.
//	row 8 (top control row, press only):
//	                    col 0 Record, col 1 Overdub, col 2 Clear,
//	                    col 3 Quantize, col 4 Octave-, col 5 Octave+,
//	                    col 6 LoopLen-, col 7 LoopLen+.
//	col 8 (right column, press only): pattern slots, slot = 7-row. Selecting
//	                    a slot makes it the editing pattern AND queues it, so
//	                    what you hear and what you record are the same loop.
//
// Caller (HandlePad in launchpad.go) holds track.Lock and calls
// MarkTrackDirty on press after this returns; selecting a slot sets
// EditingPatternIdx so the newly-selected slot compiles for free. We never
// mark dirty from here.
func looperPad(track *model.Track, project *model.Project, out midi.ToExternal, ev PadEvent) {
	if track == nil {
		return
	}
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Resolve this track's index in project.Tracks; prefer the UI focus as a
	// fast path, fall back to a linear search (mirrors drum.go).
	trackIdx := project.UI.Focus.Track
	if trackIdx < 0 || trackIdx >= 8 || project.Tracks[trackIdx] != track {
		trackIdx = project.TrackIndex(track)
		if trackIdx < 0 {
			return
		}
	}

	switch {
	case ev.Row == 8: // top control row — act on press only
		if !ev.Down {
			return
		}
		switch ev.Col {
		case 0: // Record: arm/disarm; arming wipes the loop and clears overdub
			d.Recording = !d.Recording
			if d.Recording {
				pat.Events = nil
				d.Overdub = false
			}
		case 1: // Overdub: arm/disarm; arming keeps events, clears record
			d.Overdub = !d.Overdub
			if d.Overdub {
				d.Recording = false
			}
		case 2: // Clear: wipe the loop and disarm both
			pat.Events = nil
			d.Recording = false
			d.Overdub = false
		case 3: // Quantize toggle
			pat.Quantize = !pat.Quantize
		case 4: // Octave-
			if d.Octave > -4 {
				d.Octave--
			}
		case 5: // Octave+
			if d.Octave < 4 {
				d.Octave++
			}
		case 6: // Loop length-
			stepTicks := int64(devices.PPQ / 4)
			if pat.Length > stepTicks {
				pat.Length -= stepTicks
			}
		case 7: // Loop length+
			stepTicks := int64(devices.PPQ / 4)
			if pat.Length < 32*stepTicks {
				pat.Length += stepTicks
			}
		}
		return

	case ev.Col == 8: // right column — pattern slots, press only
		if !ev.Down {
			return
		}
		slot := 7 - ev.Row
		if slot < 0 || slot > 7 {
			return
		}
		d.EditingPatternIdx = slot
		project.Playback.Cursors[trackIdx].QueuedPattern = slot
		return

	default: // 8x8 grid — chromatic-fourths keyboard. Handle press AND release.
		note := 60 + 12*d.Octave + 5*ev.Row + ev.Col
		if note < 0 || note > 127 {
			return
		}
		ev2 := midi.Event{
			Note:    uint8(note),
			Channel: track.Channel,
		}
		if ev.Down {
			ev2.Type = midi.NoteOn
			ev2.Velocity = ev.Velocity
		} else {
			ev2.Type = midi.NoteOff
			ev2.Velocity = 0
		}
		looperctl.RecordOrPreview(track, project, trackIdx, out, ev2)
		return
	}
}

// looperLEDs renders the full LED frame for the looper view: 64 grid cells
// (rows 0-7, cols 0-7), 8 top-row controls (Row:8), and 8 right-column
// pattern slots (Col:8) — 80 LEDs total. Every cell is emitted explicitly:
// SetLEDs rewrites the whole frame and un-emitted cells keep stale color
// (see the note in session.go).
func looperLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil {
		return nil
	}
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return nil
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	trackIdx := project.UI.Focus.Track
	if trackIdx < 0 || trackIdx >= 8 || project.Tracks[trackIdx] != track {
		trackIdx = project.TrackIndex(track)
	}

	leds := make([]LED, 0, 80)

	// 8x8 grid: chromatic-fourths keyboard. Root notes (note%12==0) bright,
	// every other note dim.
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			note := 60 + 12*d.Octave + 5*row + col
			c := looperKey
			if ((note%12)+12)%12 == 0 {
				c = looperRoot
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	// Top control row (Row:8, cols 0-7).
	topRow := [8]LED{
		ledIf(d.Recording, looperRecOn, looperRecDim), // 0 Record
		ledIf(d.Overdub, looperDubOn, looperDubDim),   // 1 Overdub
		looperClear, // 2 Clear
		ledIf(pat.Quantize, looperQuantOn, looperQuantDim), // 3 Quantize
		looperOctave,  // 4 Octave-
		looperOctave,  // 5 Octave+
		looperLoopLen, // 6 LoopLen-
		looperLoopLen, // 7 LoopLen+
	}
	for col, c := range topRow {
		leds = append(leds, LED{Row: 8, Col: col, R: c.R, G: c.G, B: c.B})
	}

	// Right column (Col:8, rows 0-7): pattern slots, slot = 7-row. Reuses the
	// session color precedence: playing > queued > has-content > empty.
	var cursor model.Cursor
	if trackIdx >= 0 {
		cursor = project.Playback.Cursors[trackIdx]
	}
	for row := 0; row < 8; row++ {
		slot := 7 - row
		var c LED
		if trackIdx < 0 {
			c = sessionEmpty
		} else {
			c = sessionCellColor(track, cursor, slot)
		}
		leds = append(leds, LED{Row: row, Col: 8, R: c.R, G: c.G, B: c.B})
	}

	return leds
}

// ledIf returns on when cond is true, otherwise off. Keeps the control-row
// table above readable.
func ledIf(cond bool, on, off LED) LED {
	if cond {
		return on
	}
	return off
}
