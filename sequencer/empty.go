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

// Device interface implementation

func (e *EmptyDevice) Tick(step int) []midi.Event {
	return nil
}

func (e *EmptyDevice) QueuePattern(p int) (pattern, next int) {
	return 0, 0
}

func (e *EmptyDevice) GetState() (pattern, next int) {
	return 0, 0
}

func (e *EmptyDevice) ContentMask() []bool {
	return make([]bool, NumPatterns)
}

func (e *EmptyDevice) HandleMIDI(event midi.Event) {}

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
	out += widgets.RenderLaunchpad(e.HelpLayout())
	out += "\n"
	out += widgets.RenderLegend([]widgets.Zone{
		{Name: "Empty", Color: [3]uint8{40, 40, 40}, Desc: "no device assigned"},
	})

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

func (e *EmptyDevice) HelpLayout() widgets.LaunchpadLayout {
	dimColor := [3]uint8{40, 40, 40}

	var layout widgets.LaunchpadLayout

	for i := 0; i < 8; i++ {
		layout.TopRow[i] = widgets.PadConfig{Color: dimColor, Tooltip: "Empty"}
	}

	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			layout.Grid[row][col] = widgets.PadConfig{Color: dimColor, Tooltip: "Empty"}
		}
	}

	for i := 0; i < 8; i++ {
		layout.RightCol[i] = widgets.PadConfig{Color: dimColor, Tooltip: "Empty"}
	}

	return layout
}
