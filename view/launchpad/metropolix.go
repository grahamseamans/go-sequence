package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Metropolix Launchpad X mapping (full-parity port of the old UI).
//
// Page layout on the grid (8x8 main grid, col 8 = scene buttons, row 8 = top
// CC row). `spec.Page` (0..7) selects which page is active:
//
//	0: Main stage editor  — cols = stage, rows = Note (0..7). Tap sets
//	   stage[col].Note = row AND gate=true AND Selected=col.
//	1: Octave             — rows = stage[col].Octave (0..7).
//	2: Pulse count        — rows 0..7 = pulse count 1..8.
//	3: Ratchets           — rows 0..7 = ratchet count 1..8.
//	4: Probability        — rows 0..7 = probability value row*100/7.
//	5: Gate               — rows 7..2: gate length index 5..0,
//	                        row 1: gate on/off toggle,
//	                        row 0: slide on/off toggle.
//	6: Accumulator        — sub-page selected via spec.AccumSubPage:
//	                          0 = value   (rows 0..7 = accumulator -4..+3)
//	                          1 = reset   (rows 0..7 = reset count 0..7)
//	                          2 = mode    (rows 0..2 = mode 0..2)
//	                        Top row (row 8) col 1 = up, col 2 = down to
//	                        switch sub-pages.
//	7: Settings           — row 7: mode (cols 0..3),
//	                        row 6: scale (cols 0..3 — first 4 of 22),
//	                        row 5: length (cols 0..7 = 1..8),
//	                        row 4: root note within current octave (cols 0..7),
//	                        row 3: slide time (cols 0..7 = 1..8).
//
// Scene buttons (col 8, rows 0..7) always select the active page.
var (
	mxStageActive    = LED{R: 255, G: 100, B: 50}
	mxStageActiveOff = LED{R: 60, G: 25, B: 15}
	mxStageDim       = LED{R: 20, G: 10, B: 5}
	mxStageInactive  = LED{R: 0, G: 0, B: 0}
	mxSelectedCol    = LED{R: 255, G: 255, B: 255}

	mxValueActive = LED{R: 255, G: 100, B: 50}
	mxValueDim    = LED{R: 50, G: 30, B: 20}
	mxValueOff    = LED{R: 0, G: 0, B: 0}
	mxCenter      = LED{R: 100, G: 100, B: 100}

	mxGateLenActive = LED{R: 255, G: 100, B: 50}
	mxGateLenDim    = LED{R: 50, G: 30, B: 20}
	mxGateOn        = LED{R: 0, G: 255, B: 0}
	mxGateOff       = LED{R: 30, G: 50, B: 30}
	mxSlideOn       = LED{R: 0, G: 150, B: 255}
	mxSlideOff      = LED{R: 20, G: 40, B: 50}

	mxAccumModeColors = [3]LED{
		{R: 0, G: 255, B: 0},   // reset = green
		{R: 255, G: 200, B: 0}, // ping-pong = yellow
		{R: 255, G: 0, B: 0},   // hold = red
	}

	mxPageActive = LED{R: 200, G: 100, B: 255}
	mxPageDim    = LED{R: 50, G: 25, B: 60}
)

// metropolixPad handles a pad press on the Metropolix view. Dispatches based
// on spec.Page; see the page layout comment above.
//
// Caller (HandlePad in launchpad.go) holds track.Lock and calls
// MarkTrackDirty after this returns; we just mutate spec.
func metropolixPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
		return
	}
	if track == nil || track.Metropolix == nil {
		return
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
		return
	}

	// Top row (row 8): only meaningful on the accumulator page for sub-page
	// navigation (col 1 = up, col 2 = down). Everywhere else it's a no-op.
	if row == 8 {
		if spec.Page == 6 {
			switch col {
			case 1:
				if spec.AccumSubPage > 0 {
					spec.AccumSubPage--
				}
			case 2:
				if spec.AccumSubPage < 2 {
					spec.AccumSubPage++
				}
			}
		}
		return
	}

	// Scene buttons (col 8, rows 0..7): select page.
	if col == 8 {
		if row >= 0 && row <= 7 {
			spec.Page = row
		}
		return
	}

	// Main grid: page-specific behavior. Most pages no-op on columns
	// >= pat.Length (the stage doesn't exist).
	if row < 0 || row > 7 || col < 0 || col > 7 {
		return
	}

	switch spec.Page {
	case 0: // Main stage editor — note + gate + select.
		if col >= pat.Length {
			return
		}
		stage := &pat.Stages[col]
		stage.Note = row
		stage.Gate = true
		spec.Selected = col
	case 1: // Octave
		if col >= pat.Length {
			return
		}
		pat.Stages[col].Octave = row
		spec.Selected = col
	case 2: // Pulse count (row 0..7 → 1..8)
		if col >= pat.Length {
			return
		}
		pat.Stages[col].PulseCount = row + 1
		spec.Selected = col
	case 3: // Ratchets (row 0..7 → 1..8)
		if col >= pat.Length {
			return
		}
		pat.Stages[col].Ratchets = row + 1
		spec.Selected = col
	case 4: // Probability (row * 100 / 7)
		if col >= pat.Length {
			return
		}
		pat.Stages[col].Probability = row * 100 / 7
		spec.Selected = col
	case 5: // Gate length + gate + slide.
		if col >= pat.Length {
			return
		}
		stage := &pat.Stages[col]
		switch {
		case row >= 2 && row <= 7:
			stage.GateLength = row - 2
		case row == 1:
			stage.Gate = !stage.Gate
		case row == 0:
			stage.Slide = !stage.Slide
		}
		spec.Selected = col
	case 6: // Accumulator — behavior depends on AccumSubPage.
		if col >= pat.Length {
			return
		}
		stage := &pat.Stages[col]
		switch spec.AccumSubPage {
		case 0:
			stage.Accumulator = row - 4
		case 1:
			stage.AccumReset = row
		case 2:
			if row < 3 {
				stage.AccumMode = row
			}
		}
		spec.Selected = col
	case 7: // Settings: per-row global parameters.
		metropolixSettingsPad(pat, spec, row, col)
	}
}

// metropolixSettingsPad handles a pad press on the settings page. Row 7 =
// mode (cols 0..3), row 6 = scale (cols 0..3 — only first 4 scales reachable
// from pads; the TUI exposes all 22), row 5 = length, row 4 = root-note-
// within-octave, row 3 = slide time. Rows 0..2 are unused.
func metropolixSettingsPad(pat *devices.MetropolixPattern, spec *devices.Metropolix, row, col int) {
	switch row {
	case 7:
		if col < 4 {
			pat.Mode = devices.PlaybackMode(col)
		}
	case 6:
		if col < 4 {
			pat.Scale = devices.ScaleType(col)
		}
	case 5:
		pat.Length = col + 1
		if spec.Selected >= pat.Length {
			spec.Selected = pat.Length - 1
		}
	case 4:
		currentOctave := int(pat.RootNote) / 12
		pat.RootNote = uint8(currentOctave*12 + col)
	case 3:
		pat.SlideTime = col + 1
	}
}

// metropolixLEDs renders the 8x8 LED frame + col 8 (page indicator) + row 8
// (page-specific, e.g. accumulator sub-page arrows) for the Metropolix view.
func metropolixLEDs(track *model.Track, project *model.Project) []LED {
	if track == nil || track.Metropolix == nil {
		return nil
	}
	spec := track.Metropolix
	pat := spec.Patterns[spec.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	leds := make([]LED, 0, 8*9+8)

	// Main grid (rows 0..7, cols 0..7) depending on active page.
	switch spec.Page {
	case 0:
		leds = append(leds, metropolixPage0LEDs(pat, spec)...)
	case 1:
		leds = append(leds, metropolixValuePageLEDs(pat, func(s *devices.MetropolixStage) int { return s.Octave })...)
	case 2:
		leds = append(leds, metropolixValuePageLEDs(pat, func(s *devices.MetropolixStage) int { return s.PulseCount - 1 })...)
	case 3:
		leds = append(leds, metropolixValuePageLEDs(pat, func(s *devices.MetropolixStage) int { return s.Ratchets - 1 })...)
	case 4:
		leds = append(leds, metropolixValuePageLEDs(pat, func(s *devices.MetropolixStage) int { return s.Probability * 7 / 100 })...)
	case 5:
		leds = append(leds, metropolixGatePageLEDs(pat)...)
	case 6:
		leds = append(leds, metropolixAccumulatorLEDs(pat, spec)...)
	case 7:
		leds = append(leds, metropolixSettingsLEDs(pat)...)
	}

	// Scene buttons (col 8, rows 0..7): dim purple; active page = bright.
	for row := 0; row < 8; row++ {
		c := mxPageDim
		if row == spec.Page {
			c = mxPageActive
		}
		leds = append(leds, LED{Row: row, Col: 8, R: c.R, G: c.G, B: c.B})
	}

	// Top row (row 8): accumulator sub-page arrows on the accumulator page;
	// off everywhere else.
	for col := 0; col < 8; col++ {
		c := mxValueOff
		if spec.Page == 6 {
			switch col {
			case 1:
				if spec.AccumSubPage > 0 {
					c = mxValueActive
				} else {
					c = mxValueDim
				}
			case 2:
				if spec.AccumSubPage < 2 {
					c = mxValueActive
				} else {
					c = mxValueDim
				}
			}
		}
		leds = append(leds, LED{Row: 8, Col: col, R: c.R, G: c.G, B: c.B})
	}

	return leds
}

// metropolixPage0LEDs renders the main stage editor grid (page 0).
//
// Per column (stage):
//
//	col >= pat.Length     — all rows off.
//	Gate == true          — noteRow bright, other rows in col dim (indicates
//	                        stage is "live" even on non-note rows).
//	Gate == false         — noteRow dim-muted, other rows off.
//
// Selected stage gets a white marker on row 7 of its column (overwrites
// whatever was there — the user cares more about "which stage is selected"
// than the note indicator on that column).
func metropolixPage0LEDs(pat *devices.MetropolixPattern, spec *devices.Metropolix) []LED {
	leds := make([]LED, 0, 64+1)
	for col := 0; col < 8; col++ {
		if col >= pat.Length {
			for row := 0; row < 8; row++ {
				leds = append(leds, LED{Row: row, Col: col,
					R: mxStageInactive.R, G: mxStageInactive.G, B: mxStageInactive.B})
			}
			continue
		}
		stage := &pat.Stages[col]
		noteRow := stage.Note
		if noteRow < 0 {
			noteRow = 0
		}
		if noteRow > 7 {
			noteRow = 7
		}
		for row := 0; row < 8; row++ {
			var c LED
			switch {
			case row == noteRow && stage.Gate:
				c = mxStageActive
			case row == noteRow && !stage.Gate:
				c = mxStageActiveOff
			case stage.Gate:
				c = mxStageDim
			default:
				c = mxStageInactive
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}
	if spec.Selected >= 0 && spec.Selected < pat.Length {
		leds = append(leds, LED{Row: 7, Col: spec.Selected,
			R: mxSelectedCol.R, G: mxSelectedCol.G, B: mxSelectedCol.B})
	}
	return leds
}

// metropolixValuePageLEDs renders a "value per stage" page: for each stage
// column, the row matching the current value is bright; other rows dim.
// getValue maps a stage → its 0..7 value.
func metropolixValuePageLEDs(pat *devices.MetropolixPattern, getValue func(*devices.MetropolixStage) int) []LED {
	leds := make([]LED, 0, 64)
	for col := 0; col < 8; col++ {
		if col >= pat.Length {
			for row := 0; row < 8; row++ {
				leds = append(leds, LED{Row: row, Col: col,
					R: mxValueOff.R, G: mxValueOff.G, B: mxValueOff.B})
			}
			continue
		}
		value := getValue(&pat.Stages[col])
		if value < 0 {
			value = 0
		}
		if value > 7 {
			value = 7
		}
		for row := 0; row < 8; row++ {
			c := mxValueDim
			if row == value {
				c = mxValueActive
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}
	return leds
}

// metropolixGatePageLEDs renders page 5 (gate length + gate + slide).
// Rows 7..2: gate length values (row 7 = index 5 / full, row 2 = index 0 /
// trigger). Row 1: gate on/off. Row 0: slide on/off.
func metropolixGatePageLEDs(pat *devices.MetropolixPattern) []LED {
	leds := make([]LED, 0, 64)
	for col := 0; col < 8; col++ {
		if col >= pat.Length {
			for row := 0; row < 8; row++ {
				leds = append(leds, LED{Row: row, Col: col,
					R: mxValueOff.R, G: mxValueOff.G, B: mxValueOff.B})
			}
			continue
		}
		stage := &pat.Stages[col]
		for row := 7; row >= 2; row-- {
			lengthIdx := row - 2
			c := mxGateLenDim
			if stage.GateLength == lengthIdx {
				c = mxGateLenActive
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
		gc := mxGateOff
		if stage.Gate {
			gc = mxGateOn
		}
		leds = append(leds, LED{Row: 1, Col: col, R: gc.R, G: gc.G, B: gc.B})
		sc := mxSlideOff
		if stage.Slide {
			sc = mxSlideOn
		}
		leds = append(leds, LED{Row: 0, Col: col, R: sc.R, G: sc.G, B: sc.B})
	}
	return leds
}

// metropolixAccumulatorLEDs renders page 6's main grid based on AccumSubPage.
// Sub-page arrows in the top row are drawn by the caller.
func metropolixAccumulatorLEDs(pat *devices.MetropolixPattern, spec *devices.Metropolix) []LED {
	leds := make([]LED, 0, 64)

	switch spec.AccumSubPage {
	case 0: // Value: -4..+3 mapped to rows 0..7; row 4 = 0 (center marker).
		for col := 0; col < 8; col++ {
			if col >= pat.Length {
				for row := 0; row < 8; row++ {
					leds = append(leds, LED{Row: row, Col: col,
						R: mxValueOff.R, G: mxValueOff.G, B: mxValueOff.B})
				}
				continue
			}
			value := pat.Stages[col].Accumulator + 4
			for row := 0; row < 8; row++ {
				var c LED
				switch {
				case row == value:
					c = mxValueActive
				case row == 4:
					c = mxCenter
				default:
					c = mxValueDim
				}
				leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
			}
		}
	case 1: // Reset: rows 0..7 (row 0 = never / 0).
		for col := 0; col < 8; col++ {
			if col >= pat.Length {
				for row := 0; row < 8; row++ {
					leds = append(leds, LED{Row: row, Col: col,
						R: mxValueOff.R, G: mxValueOff.G, B: mxValueOff.B})
				}
				continue
			}
			value := pat.Stages[col].AccumReset
			if value > 7 {
				value = 7
			}
			for row := 0; row < 8; row++ {
				var c LED
				switch {
				case row == value:
					c = mxValueActive
				case row == 0:
					c = mxCenter
				default:
					c = mxValueDim
				}
				leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
			}
		}
	case 2: // Mode: rows 0..2 only; rest of the column off.
		for col := 0; col < 8; col++ {
			if col >= pat.Length {
				for row := 0; row < 8; row++ {
					leds = append(leds, LED{Row: row, Col: col,
						R: mxValueOff.R, G: mxValueOff.G, B: mxValueOff.B})
				}
				continue
			}
			mode := pat.Stages[col].AccumMode
			for row := 0; row < 8; row++ {
				var c LED
				if row < 3 {
					if row == mode {
						c = mxAccumModeColors[row]
					} else {
						mc := mxAccumModeColors[row]
						c = LED{R: mc.R / 5, G: mc.G / 5, B: mc.B / 5}
					}
				} else {
					c = mxValueOff
				}
				leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
			}
		}
	}
	return leds
}

// metropolixSettingsLEDs renders page 7 (global settings). Rows are static
// regardless of pat.Length.
func metropolixSettingsLEDs(pat *devices.MetropolixPattern) []LED {
	leds := make([]LED, 0, 64)

	// Row 7: mode (cols 0..3).
	for col := 0; col < 8; col++ {
		var c LED
		switch {
		case col < 4:
			c = mxValueDim
			if col == int(pat.Mode) {
				c = mxValueActive
			}
		default:
			c = mxValueOff
		}
		leds = append(leds, LED{Row: 7, Col: col, R: c.R, G: c.G, B: c.B})
	}

	// Row 6: scale — only first 4 reachable from pads.
	for col := 0; col < 8; col++ {
		var c LED
		switch {
		case col < 4:
			c = mxValueDim
			if col == int(pat.Scale) {
				c = mxValueActive
			}
		default:
			c = mxValueOff
		}
		leds = append(leds, LED{Row: 6, Col: col, R: c.R, G: c.G, B: c.B})
	}

	// Row 5: length (1..8 — lit up to pat.Length).
	for col := 0; col < 8; col++ {
		c := mxValueDim
		if col < pat.Length {
			c = mxValueActive
		}
		leds = append(leds, LED{Row: 5, Col: col, R: c.R, G: c.G, B: c.B})
	}

	// Row 4: root note within current octave (mod 12, wrapped into 8 cols).
	rootNoteInOctave := int(pat.RootNote) % 12
	for col := 0; col < 8; col++ {
		c := mxValueDim
		if col == rootNoteInOctave%8 {
			c = mxValueActive
		}
		leds = append(leds, LED{Row: 4, Col: col, R: c.R, G: c.G, B: c.B})
	}

	// Row 3: slide time (1..8).
	for col := 0; col < 8; col++ {
		c := mxValueDim
		if col < pat.SlideTime {
			c = mxValueActive
		}
		leds = append(leds, LED{Row: 3, Col: col, R: c.R, G: c.G, B: c.B})
	}

	// Rows 0..2: reserved/off.
	for row := 0; row < 3; row++ {
		for col := 0; col < 8; col++ {
			leds = append(leds, LED{Row: row, Col: col,
				R: mxValueOff.R, G: mxValueOff.G, B: mxValueOff.B})
		}
	}

	return leds
}
