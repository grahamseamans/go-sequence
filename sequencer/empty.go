package sequencer

import (
	"fmt"

	"go-sequence/midi"
	"go-sequence/widgets"
)

// EmptyDevice is a placeholder for tracks with no sequencer assigned
type EmptyDevice struct {
	trackNum int // 1-8, for display
}

// NewEmptyDevice creates an empty device placeholder
func NewEmptyDevice(trackNum int) *EmptyDevice {
	return &EmptyDevice{trackNum: trackNum}
}

// Device interface implementation - queue-based (stubs for non-music device)

func (e *EmptyDevice) FillUntil(tick int64)           {}
func (e *EmptyDevice) PeekNextEvent() *midi.Event     { return nil }
func (e *EmptyDevice) PopNextEvent() *midi.Event      { return nil }
func (e *EmptyDevice) ClearQueue()                    {}
func (e *EmptyDevice) QueuePattern(p int, atTick int64) {}
func (e *EmptyDevice) CurrentPattern() int            { return 0 }
func (e *EmptyDevice) NextPattern() int               { return -1 }
func (e *EmptyDevice) ContentMask() []bool            { return make([]bool, NumPatterns) }

func (e *EmptyDevice) HandleMIDI(event midi.Event) {}

func (e *EmptyDevice) ToggleRecording() {}
func (e *EmptyDevice) TogglePreview()   {}
func (e *EmptyDevice) IsRecording() bool { return false }
func (e *EmptyDevice) IsPreviewing() bool { return false }

func (e *EmptyDevice) View() string {
	out := fmt.Sprintf("TRACK %d  (empty)\n\n", e.trackNum)
	out += "No device assigned to this track.\n\n"
	out += "Press , to open settings and assign a device.\n"

	// Key help
	out += "\n"
	out += widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: ",", Desc: "open settings to assign device"},
			{Key: "0", Desc: "back to session"},
			{Key: "1-8", Desc: "switch to another track"},
		}},
	})

	// Launchpad
	out += "\n\n"
	out += e.renderLaunchpadHelp()

	return out
}

func (e *EmptyDevice) renderLaunchpadHelp() string {
	dimColor := [3]uint8{40, 40, 40}

	var grid [8][8][3]uint8
	var rightCol [8][3]uint8
	topRow := make([][3]uint8, 8)

	for i := 0; i < 8; i++ {
		topRow[i] = dimColor
		rightCol[i] = dimColor
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = dimColor
		}
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, &rightCol) + "\n\n"
	out += widgets.RenderLegendItem(dimColor, "Empty", "no device assigned")

	return out
}

func (e *EmptyDevice) RenderLEDs() []LEDState {
	var leds []LEDState
	dimColor := [3]uint8{20, 20, 20}

	// All pads dim
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			leds = append(leds, LEDState{Row: row, Col: col, Color: dimColor, Channel: midi.ChannelStatic})
		}
	}

	return leds
}

func (e *EmptyDevice) HandleKey(key string) {
	// Nothing to do - empty device has no controls
}

func (e *EmptyDevice) HandlePad(row, col int) {
	// Nothing to do
}
