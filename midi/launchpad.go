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

		// Set brightness to maximum (0-127)
		// F0 00 20 29 02 0C 08 <brightness> F7
		lp.send(gomidi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x08, 0x7F}))

		// Enable external LED feedback
		// F0 00 20 29 02 0C 0A 01 01 F7
		lp.send(gomidi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x0A, 0x01, 0x01}))
	}

	// Open input
	if inPort != nil {
		stop, err := gomidi.ListenTo(inPort, func(msg gomidi.Message, timestampms int32) {
			var channel, note, velocity uint8
			var cc, value uint8

			// Handle note messages (8x8 grid + side buttons)
			if msg.GetNoteOn(&channel, &note, &velocity) && velocity > 0 {
				row, col := noteToRowCol(note)
				if row >= 0 {
					select {
					case lp.padChan <- PadEvent{Row: row, Col: col, Velocity: velocity}:
					default:
					}
				}
			}

			// Handle CC messages (top row buttons CC 91-98)
			if msg.GetControlChange(&channel, &cc, &value) && value > 0 {
				row, col := ccToRowCol(cc)
				if row >= 0 {
					select {
					case lp.padChan <- PadEvent{Row: row, Col: col, Velocity: value}:
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

func (lp *LaunchpadController) SetLEDRGB(row, col int, rgb [3]uint8, channel uint8) error {
	color := mapRGBToLaunchpad(rgb)
	return lp.SetLED(row, col, color, channel)
}

// mapRGBToLaunchpad finds the nearest Launchpad X palette color for an RGB value
func mapRGBToLaunchpad(rgb [3]uint8) uint8 {
	// Launchpad X palette - approximate RGB values for key colors
	// Format: {velocity, R, G, B}
	palette := [][4]uint8{
		{0, 0, 0, 0},         // off
		{5, 255, 0, 0},       // red
		{6, 255, 80, 80},     // bright red
		{7, 180, 60, 60},     // dim red
		{9, 255, 100, 0},     // orange
		{11, 180, 80, 40},    // dim orange
		{13, 255, 200, 0},    // yellow
		{17, 0, 180, 0},      // green
		{19, 0, 100, 0},      // dim green
		{21, 0, 255, 0},      // bright green
		{37, 0, 200, 200},    // cyan
		{43, 40, 60, 120},    // dim blue
		{45, 0, 100, 255},    // blue
		{47, 80, 150, 255},   // bright blue
		{49, 150, 0, 200},    // purple
		{53, 255, 80, 180},   // pink
		{78, 100, 100, 255},  // light blue
		{84, 255, 150, 50},   // bright orange
		{87, 150, 255, 100},  // lime
		{97, 180, 180, 60},   // dim yellow
		{119, 255, 255, 255}, // white
	}

	bestMatch := uint8(0)
	bestDist := 999999

	r, g, b := int(rgb[0]), int(rgb[1]), int(rgb[2])

	for _, p := range palette {
		pr, pg, pb := int(p[1]), int(p[2]), int(p[3])
		// Simple Euclidean distance
		dist := (r-pr)*(r-pr) + (g-pg)*(g-pg) + (b-pb)*(b-pb)
		if dist < bestDist {
			bestDist = dist
			bestMatch = p[0]
		}
	}

	return bestMatch
}

func (lp *LaunchpadController) ClearLEDs() error {
	if lp.send == nil {
		return nil
	}
	// Clear 8x8 main grid
	for row := range 8 {
		for col := range 8 {
			lp.SetLED(row, col, ColorOff, 0)
		}
	}
	// Clear right side column (col 8)
	for row := range 8 {
		lp.SetLED(row, 8, ColorOff, 0)
	}
	// Clear top row (row 8)
	for col := range 8 {
		lp.SetLED(8, col, ColorOff, 0)
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
// 8x8 Grid:  Row 0 (bottom) = notes 11-18, Row 7 = notes 81-88
// Side col:  Col 8 (right side scene buttons) = notes 19, 29, 39, 49, 59, 69, 79, 89
// Top row:   Row 8 (top control row) = CC 91-98 (handled via CC messages)

func rowColToNote(row, col int) uint8 {
	// Top row uses CC, but for LED control we use notes 91-98
	if row == 8 {
		return uint8(91 + col)
	}
	return uint8((row+1)*10 + col + 1)
}

func noteToRowCol(note uint8) (row, col int) {
	// Top row notes (91-98)
	if note >= 91 && note <= 98 {
		return 8, int(note - 91)
	}
	row = int(note/10) - 1
	col = int(note%10) - 1
	// Accept 8x8 grid (rows 0-7, cols 0-7) plus side column (col 8)
	if row < 0 || row > 7 || col < 0 || col > 8 {
		return -1, -1
	}
	return row, col
}

// ccToRowCol converts CC messages to row/col (for top row buttons)
func ccToRowCol(cc uint8) (row, col int) {
	if cc >= 91 && cc <= 98 {
		return 8, int(cc - 91)
	}
	return -1, -1
}
