package main

import (
	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// Launchpad X color palette indices
const (
	ColorOff        = 0
	ColorRed        = 5
	ColorGreen      = 13
	ColorBrightGreen = 19
	ColorBlue       = 45
	ColorYellow     = 69
	ColorOrange     = 9
	ColorWhite      = 127
	ColorDim        = 1
)

type Launchpad struct {
	outPort drivers.Out
	send    func(msg midi.Message) error
}

func NewLaunchpad() *Launchpad {
	return &Launchpad{}
}

func (lp *Launchpad) Open(portIndex int) error {
	outs := midi.GetOutPorts()
	if portIndex < 0 || portIndex >= len(outs) {
		return nil // No launchpad, that's OK
	}
	lp.outPort = outs[portIndex]
	send, err := midi.SendTo(lp.outPort)
	if err != nil {
		return err
	}
	lp.send = send

	// Enable external LED feedback (SysEx)
	// F0 00 20 29 02 0C 0A 01 01 F7
	lp.send(midi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x0A, 0x01, 0x01}))

	return nil
}

func (lp *Launchpad) Close() {
	// Clear all LEDs on close
	if lp.send != nil {
		lp.ClearGrid()
	}
}

// stepToNote converts step index (0-15) to Launchpad note number
// Bottom row: steps 0-7 = notes 11-18
// Second row: steps 8-15 = notes 21-28
func stepToNote(step int) uint8 {
	if step < 8 {
		return uint8(11 + step)
	}
	return uint8(21 + (step - 8))
}

// SetPad sets a pad LED color
// channel: 0=static, 2=pulsing
func (lp *Launchpad) SetPad(note uint8, color uint8, channel uint8) {
	if lp.send == nil {
		return
	}
	lp.send(midi.NoteOn(channel, note, color))
}

// ClearGrid turns off the bottom 2 rows (our sequencer area)
func (lp *Launchpad) ClearGrid() {
	if lp.send == nil {
		return
	}
	for i := 0; i < 16; i++ {
		lp.send(midi.NoteOn(0, stepToNote(i), ColorOff))
	}
}

// UpdateSequence updates the LED display for the full sequence
func (lp *Launchpad) UpdateSequence(steps [16]Step, playhead int, playing bool) {
	if lp.send == nil {
		return
	}

	for i, step := range steps {
		note := stepToNote(i)
		var color uint8

		if i == playhead && playing {
			// Playhead - bright yellow, pulsing
			color = ColorYellow
			lp.SetPad(note, color, 2) // Channel 2 = pulsing
		} else if step.Active {
			// Active step - green
			color = ColorGreen
			lp.SetPad(note, color, 0) // Channel 0 = static
		} else {
			// Inactive - dim
			color = ColorDim
			lp.SetPad(note, color, 0)
		}
	}
}

// HighlightCursor shows cursor position (for editing)
func (lp *Launchpad) HighlightCursor(cursor int, steps [16]Step) {
	if lp.send == nil {
		return
	}
	note := stepToNote(cursor)
	// White for cursor
	lp.SetPad(note, ColorWhite, 0)
}
