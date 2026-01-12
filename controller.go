package main

import (
	"strings"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// Launchpad X color palette indices
const (
	ColorOff         = 0
	ColorRed         = 5
	ColorGreen       = 13
	ColorBrightGreen = 19
	ColorBlue        = 45
	ColorYellow      = 69
	ColorOrange      = 9
	ColorWhite       = 127
	ColorDim         = 1
)

type Controller struct {
	outPort drivers.Out
	inPort  drivers.In
	send    func(msg midi.Message) error
	stop    func()

	// Callback for pad presses - set by Manager
	OnPad func(row, col int)
}

func NewController() *Controller {
	return &Controller{}
}

func (c *Controller) Open() error {
	// Find Launchpad output port (for LEDs)
	outIdx := findLaunchpadOutPort()
	if outIdx >= 0 {
		outs := midi.GetOutPorts()
		c.outPort = outs[outIdx]
		send, err := midi.SendTo(c.outPort)
		if err != nil {
			return err
		}
		c.send = send

		// Enable external LED feedback (SysEx)
		c.send(midi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x0A, 0x01, 0x01}))
	}

	// Find Launchpad input port (for pad presses)
	inIdx := findLaunchpadInPort()
	if inIdx >= 0 {
		ins := midi.GetInPorts()
		c.inPort = ins[inIdx]

		stop, err := midi.ListenTo(c.inPort, func(msg midi.Message, timestampms int32) {
			var channel, note, velocity uint8
			if msg.GetNoteOn(&channel, &note, &velocity) {
				if velocity > 0 {
					row, col := noteToRowCol(note)
					if row >= 0 && c.OnPad != nil {
						c.OnPad(row, col)
					}
				}
			}
		})
		if err != nil {
			return err
		}
		c.stop = stop
	}

	return nil
}

func (c *Controller) Close() {
	if c.send != nil {
		c.ClearGrid()
	}
	if c.stop != nil {
		c.stop()
	}
}

// SetPad sets a pad LED color
// channel: 0=static, 2=pulsing
func (c *Controller) SetPad(row, col int, color uint8, channel uint8) {
	if c.send == nil {
		return
	}
	note := rowColToNote(row, col)
	c.send(midi.NoteOn(channel, note, color))
}

// ClearGrid turns off all pads
func (c *Controller) ClearGrid() {
	if c.send == nil {
		return
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			c.SetPad(row, col, ColorOff, 0)
		}
	}
}

// Launchpad X note mapping
// Row 0 (bottom) = notes 11-18
// Row 1 = notes 21-28
// ...
// Row 7 (top) = notes 81-88

func rowColToNote(row, col int) uint8 {
	return uint8((row+1)*10 + col + 1)
}

func noteToRowCol(note uint8) (row, col int) {
	row = int(note/10) - 1
	col = int(note%10) - 1
	if row < 0 || row > 7 || col < 0 || col > 7 {
		return -1, -1
	}
	return row, col
}

func findLaunchpadOutPort() int {
	outs := midi.GetOutPorts()
	for i, port := range outs {
		name := strings.ToLower(port.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			return i
		}
	}
	return -1
}

func findLaunchpadInPort() int {
	ins := midi.GetInPorts()
	for i, port := range ins {
		name := strings.ToLower(port.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			return i
		}
	}
	return -1
}
