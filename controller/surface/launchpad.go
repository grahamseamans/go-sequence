package surface

import (
	"fmt"

	"go-sequence/debug"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// Launchpad is a Novation Launchpad X control surface.
type Launchpad struct {
	id       string
	outPort  drivers.Out
	inPort   drivers.In
	send     func(msg gomidi.Message) error
	stopFunc func()
	padChan  chan PadEvent
}

// NewLaunchpad opens and configures a Launchpad X.
func NewLaunchpad(id string, inPort drivers.In, outPort drivers.Out) (*Launchpad, error) {
	lp := &Launchpad{
		id:      id,
		inPort:  inPort,
		outPort: outPort,
		padChan: make(chan PadEvent, 32),
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

			// Note messages (8x8 grid + side buttons)
			if msg.GetNoteOn(&channel, &note, &velocity) {
				row, col := noteToRowCol(note)
				if row < 0 {
					return
				}
				down := velocity > 0
				debug.Log("lp-in", "NoteOn note=%d vel=%d -> row=%d col=%d down=%v", note, velocity, row, col, down)
				select {
				case lp.padChan <- PadEvent{Row: row, Col: col, Down: down}:
				default:
				}
				return
			}
			if msg.GetNoteOff(&channel, &note, &velocity) {
				row, col := noteToRowCol(note)
				if row < 0 {
					return
				}
				debug.Log("lp-in", "NoteOff note=%d -> row=%d col=%d", note, row, col)
				select {
				case lp.padChan <- PadEvent{Row: row, Col: col, Down: false}:
				default:
				}
				return
			}

			// CC messages (top row buttons CC 91-98, scene buttons)
			if msg.GetControlChange(&channel, &cc, &value) {
				row, col := ccToRowCol(cc)
				if row < 0 {
					return
				}
				down := value > 0
				debug.Log("lp-in", "CC cc=%d value=%d -> row=%d col=%d down=%v", cc, value, row, col, down)
				select {
				case lp.padChan <- PadEvent{Row: row, Col: col, Down: down}:
				default:
				}
			}
		})
		if err != nil {
			return nil, fmt.Errorf("open input: %w", err)
		}
		lp.stopFunc = stop
	}

	return lp, nil
}

// PadEvents returns the channel of pad press/release events.
func (lp *Launchpad) PadEvents() <-chan PadEvent { return lp.padChan }

// SetLEDs sends LED updates to the Launchpad. Each LED's RGB is mapped to
// the closest Launchpad X palette color and sent as a channel-0 NoteOn.
func (lp *Launchpad) SetLEDs(leds []LED) {
	if lp.send == nil || len(leds) == 0 {
		return
	}
	for _, led := range leds {
		note := rowColToNote(led.Row, led.Col)
		color := mapRGBToLaunchpad(led.R, led.G, led.B)
		lp.send(gomidi.NoteOn(0, note, color))
	}
}

// Close clears all LEDs, stops the input listener, and closes the pad channel.
func (lp *Launchpad) Close() error {
	if lp.send != nil {
		var clear []LED
		for row := 0; row < 9; row++ {
			for col := 0; col < 9; col++ {
				if row == 8 && col == 8 {
					continue // no LED at 8,8
				}
				clear = append(clear, LED{Row: row, Col: col})
			}
		}
		lp.SetLEDs(clear)
	}
	if lp.stopFunc != nil {
		lp.stopFunc()
	}
	close(lp.padChan)
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

// ccToRowCol converts CC messages to row/col (for top row and scene buttons).
func ccToRowCol(cc uint8) (row, col int) {
	// Top row: CC 91-98 -> row 8, col 0-7
	if cc >= 91 && cc <= 98 {
		return 8, int(cc - 91)
	}
	// Scene buttons (right column): CC 19,29,39,49,59,69,79,89 -> row 0-7, col 8
	if cc%10 == 9 && cc >= 19 && cc <= 89 {
		return int(cc/10) - 1, 8
	}
	return -1, -1
}

// mapRGBToLaunchpad finds the nearest Launchpad X palette color for an RGB value.
func mapRGBToLaunchpad(r, g, b uint8) uint8 {
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

	ri, gi, bi := int(r), int(g), int(b)

	for _, p := range palette {
		pr, pg, pb := int(p[1]), int(p[2]), int(p[3])
		// Simple Euclidean distance
		dist := (ri-pr)*(ri-pr) + (gi-pg)*(gi-pg) + (bi-pb)*(bi-pb)
		if dist < bestDist {
			bestDist = dist
			bestMatch = p[0]
		}
	}

	return bestMatch
}
