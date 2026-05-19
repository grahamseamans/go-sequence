package launchpad

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

var (
	looperRecOff  = LED{R: 40, G: 0, B: 0}
	looperRecOn   = LED{R: 255, G: 0, B: 0}
	looperClear   = LED{R: 200, G: 60, B: 40}
	looperOff     = LED{R: 0, G: 0, B: 0}
	looperPlaying = LED{R: 0, G: 200, B: 0}
	looperHasData = LED{R: 30, G: 60, B: 100}
	looperDim     = LED{R: 10, G: 10, B: 20}
	looperNav     = LED{R: 60, G: 40, B: 120}
)

func looperPad(track *model.Track, project *model.Project, out midi.ToExternal, row, col int, down bool) {
	if !down {
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

	// Command pads on bottom-right 4x4.
	if row < 4 && col >= 4 {
		switch {
		case row == 3 && col == 4: // record toggle
			d.Recording = !d.Recording
			if d.Recording {
				pat.Events = nil
			}
		case row == 3 && col == 5: // clear
			pat.Events = nil
			d.Recording = false
		case row == 0 && col == 4: // prev pattern
			if d.EditingPatternIdx > 0 {
				d.EditingPatternIdx--
			}
		case row == 0 && col == 5: // next pattern
			if d.EditingPatternIdx < devices.NumPatterns-1 {
				d.EditingPatternIdx++
			}
		case row == 1 && col == 4: // shorten
			stepTicks := int64(devices.PPQ / 4)
			if pat.Length > stepTicks {
				pat.Length -= stepTicks
			}
		case row == 1 && col == 5: // lengthen
			stepTicks := int64(devices.PPQ / 4)
			if pat.Length < 32*stepTicks {
				pat.Length += stepTicks
			}
		}
	}
}

func looperLEDs(track *model.Track, project *model.Project) []LED {
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return nil
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return nil
	}

	leds := make([]LED, 0, 64)

	// Fill the grid — dim with a few status indicators.
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			c := looperDim
			// Command pads bottom-right.
			if row < 4 && col >= 4 {
				switch {
				case row == 3 && col == 4:
					if d.Recording {
						c = looperRecOn
					} else {
						c = looperRecOff
					}
				case row == 3 && col == 5:
					c = looperClear
				case row == 0 && col == 4:
					c = looperNav
				case row == 0 && col == 5:
					c = looperNav
				case row == 1 && col == 4:
					c = looperNav
				case row == 1 && col == 5:
					c = looperNav
				default:
					c = looperOff
				}
			} else if len(pat.Events) > 0 {
				// Show content indicator on the left column.
				if col == 0 {
					c = looperHasData
				}
			}
			leds = append(leds, LED{Row: row, Col: col, R: c.R, G: c.G, B: c.B})
		}
	}

	return leds
}