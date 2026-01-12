package midi

import (
	"fmt"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// LaunchpadController handles a Novation Launchpad X
type LaunchpadController struct {
	id       string
	outPort  drivers.Out
	inPort   drivers.In
	send     func(msg gomidi.Message) error
	stopFunc func()

	padChan  chan PadEvent
	noteChan chan NoteEvent
}

// NewLaunchpadController creates and configures a Launchpad
func NewLaunchpadController(id string, inPort drivers.In, outPort drivers.Out) (*LaunchpadController, error) {
	lp := &LaunchpadController{
		id:       id,
		inPort:   inPort,
		outPort:  outPort,
		padChan:  make(chan PadEvent, 32),
		noteChan: make(chan NoteEvent, 32),
	}

	// Open output
	if outPort != nil {
		send, err := gomidi.SendTo(outPort)
		if err != nil {
			return nil, fmt.Errorf("open output: %w", err)
		}
		lp.send = send

		// Send SysEx to switch to Programmer mode
		// F0 00 20 29 02 0C 00 7F F7
		lp.send(gomidi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x00, 0x7F}))

		// Enable external LED feedback
		// F0 00 20 29 02 0C 0A 01 01 F7
		lp.send(gomidi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x0A, 0x01, 0x01}))
	}

	// Open input
	if inPort != nil {
		stop, err := gomidi.ListenTo(inPort, func(msg gomidi.Message, timestampms int32) {
			var channel, note, velocity uint8
			if msg.GetNoteOn(&channel, &note, &velocity) && velocity > 0 {
				row, col := noteToRowCol(note)
				if row >= 0 {
					select {
					case lp.padChan <- PadEvent{Row: row, Col: col, Velocity: velocity}:
					default:
					}
				}
			}
		})
		if err != nil {
			return nil, fmt.Errorf("open input: %w", err)
		}
		lp.stopFunc = stop
	}

	// Clear all LEDs
	lp.ClearLEDs()

	return lp, nil
}

func (lp *LaunchpadController) ID() string {
	return lp.id
}

func (lp *LaunchpadController) Type() ControllerType {
	return ControllerLaunchpad
}

func (lp *LaunchpadController) PadEvents() <-chan PadEvent {
	return lp.padChan
}

func (lp *LaunchpadController) NoteEvents() <-chan NoteEvent {
	return lp.noteChan // Launchpad doesn't send note events in the keyboard sense
}

func (lp *LaunchpadController) SetLED(row, col int, color uint8, channel uint8) error {
	if lp.send == nil {
		return nil
	}
	note := rowColToNote(row, col)
	return lp.send(gomidi.NoteOn(channel, note, color))
}

func (lp *LaunchpadController) ClearLEDs() error {
	if lp.send == nil {
		return nil
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			lp.SetLED(row, col, ColorOff, 0)
		}
	}
	return nil
}

func (lp *LaunchpadController) Close() error {
	if lp.send != nil {
		lp.ClearLEDs()
	}
	if lp.stopFunc != nil {
		lp.stopFunc()
	}
	close(lp.padChan)
	close(lp.noteChan)
	return nil
}

// Launchpad X note mapping
// Row 0 (bottom) = notes 11-18
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
