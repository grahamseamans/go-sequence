package sequencer

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/widgets"
)

// PopupType identifies what kind of popup is open
type PopupType int

const (
	PopupNone PopupType = iota
	PopupDeviceType
	PopupChannel
	PopupOutput
	PopupConfirm
)

// PopupState holds the state of an open popup
type PopupState struct {
	Type        PopupType
	Options     []string
	Selected    int
	TrackIndex  int    // which track this popup is for
	PendingType DeviceType // for confirmation dialogs
}

// SettingsDevice manages track and MIDI configuration
type SettingsDevice struct {
	tracks    []*Track
	manager   *Manager // need reference for device creation

	// Cursor position
	cursorRow int // 0-7 for tracks
	cursorCol int // 0=device, 1=channel, 2=output

	// Popup state
	popup *PopupState

	// Available MIDI ports (cached from last scan)
	midiInputs  []string
	midiOutputs []string
}

// NewSettingsDevice creates a settings device
func NewSettingsDevice(tracks []*Track, manager *Manager) *SettingsDevice {
	return &SettingsDevice{
		tracks:    tracks,
		manager:   manager,
		cursorRow: 0,
		cursorCol: 0,
	}
}

// SetMIDIPorts updates the list of available MIDI ports
func (s *SettingsDevice) SetMIDIPorts(inputs, outputs []string) {
	s.midiInputs = inputs
	s.midiOutputs = outputs
}

// Device interface implementation

func (s *SettingsDevice) Tick(step int) []midi.Event {
	return nil // Settings doesn't produce MIDI
}

func (s *SettingsDevice) QueuePattern(p int) (pattern, next int) {
	return 0, 0
}

func (s *SettingsDevice) GetState() (pattern, next int) {
	return 0, 0
}

func (s *SettingsDevice) ContentMask() []bool {
	return make([]bool, NumPatterns)
}

func (s *SettingsDevice) HandleMIDI(event midi.Event) {
	// Could use this for "learn" functionality later
}

func (s *SettingsDevice) View() string {
	var out strings.Builder

	out.WriteString("SETTINGS  Track & MIDI Configuration\n\n")

	// Track table header
	out.WriteString("Track   Device       Channel   Output\n")
	out.WriteString("─────────────────────────────────────────────────\n")

	// Track rows
	for i := 0; i < 8; i++ {
		var track *Track
		if i < len(s.tracks) {
			track = s.tracks[i]
		}

		// Track number
		out.WriteString(fmt.Sprintf("  %d     ", i+1))

		// Device type cell
		deviceStr := "(empty)"
		if track != nil && track.Device != nil {
			deviceStr = s.getDeviceTypeName(track)
		}
		if s.cursorRow == i && s.cursorCol == 0 {
			out.WriteString(fmt.Sprintf("[%-8s]  ", deviceStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-8s   ", deviceStr))
		}

		// Channel cell
		channelStr := "-"
		if track != nil {
			channelStr = fmt.Sprintf("ch %d", track.Channel)
		}
		if s.cursorRow == i && s.cursorCol == 1 {
			out.WriteString(fmt.Sprintf("[%-5s]  ", channelStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-5s   ", channelStr))
		}

		// Output cell (placeholder for now)
		outputStr := "-"
		if track != nil && track.Device != nil {
			outputStr = "default"
		}
		if s.cursorRow == i && s.cursorCol == 2 {
			out.WriteString(fmt.Sprintf("[%-12s]", outputStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-12s", outputStr))
		}

		out.WriteString("\n")
	}

	// MIDI Inputs section
	out.WriteString("\nMIDI Inputs")
	if len(s.midiInputs) == 0 {
		out.WriteString("  (press r to scan)")
	}
	out.WriteString("\n")
	out.WriteString("─────────────────────────────────────────────────\n")
	if len(s.midiInputs) == 0 {
		out.WriteString("  No MIDI inputs found\n")
	} else {
		for _, input := range s.midiInputs {
			out.WriteString(fmt.Sprintf("  %s\n", input))
		}
	}

	// MIDI Outputs section
	out.WriteString("\nMIDI Outputs\n")
	out.WriteString("─────────────────────────────────────────────────\n")
	if len(s.midiOutputs) == 0 {
		out.WriteString("  No MIDI outputs found\n")
	} else {
		for _, output := range s.midiOutputs {
			out.WriteString(fmt.Sprintf("  %s\n", output))
		}
	}

	// Popup overlay
	if s.popup != nil {
		out.WriteString("\n")
		out.WriteString(s.renderPopup())
	}

	// Key help
	out.WriteString("\n")
	if s.popup != nil {
		out.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
			{Keys: []widgets.KeyBinding{
				{Key: "j / k", Desc: "navigate options"},
				{Key: "enter", Desc: "confirm selection"},
				{Key: "esc", Desc: "cancel"},
			}},
		}))
	} else {
		out.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
			{Keys: []widgets.KeyBinding{
				{Key: "h / l", Desc: "move between columns"},
				{Key: "j / k", Desc: "move between tracks"},
				{Key: "enter", Desc: "edit selected cell"},
				{Key: "r", Desc: "rescan MIDI devices"},
			}},
		}))
	}

	// Launchpad
	out.WriteString("\n\n")
	out.WriteString(widgets.RenderLaunchpad(s.HelpLayout()))
	out.WriteString("\n")
	out.WriteString(widgets.RenderLegend([]widgets.Zone{
		{Name: "Tracks", Color: [3]uint8{100, 100, 200}, Desc: "select track to configure"},
	}))

	return out.String()
}

func (s *SettingsDevice) renderPopup() string {
	if s.popup == nil {
		return ""
	}

	var out strings.Builder

	// Box drawing
	width := 20
	title := ""
	switch s.popup.Type {
	case PopupDeviceType:
		title = "Device Type"
	case PopupChannel:
		title = "MIDI Channel"
	case PopupOutput:
		title = "MIDI Output"
	case PopupConfirm:
		title = "Confirm"
	}

	// Top border
	out.WriteString("┌" + strings.Repeat("─", width) + "┐\n")

	// Title
	padding := (width - len(title)) / 2
	out.WriteString("│" + strings.Repeat(" ", padding) + title + strings.Repeat(" ", width-padding-len(title)) + "│\n")

	// Separator
	out.WriteString("├" + strings.Repeat("─", width) + "┤\n")

	// Options
	for i, opt := range s.popup.Options {
		prefix := "  "
		if i == s.popup.Selected {
			prefix = "> "
		}
		optStr := prefix + opt
		if len(optStr) > width {
			optStr = optStr[:width]
		}
		out.WriteString("│" + optStr + strings.Repeat(" ", width-len(optStr)) + "│\n")
	}

	// Bottom border
	out.WriteString("└" + strings.Repeat("─", width) + "┘\n")

	return out.String()
}

func (s *SettingsDevice) getDeviceTypeName(track *Track) string {
	if track == nil || track.Device == nil {
		return "(empty)"
	}
	switch track.Device.(type) {
	case *DrumDevice:
		return "Drum"
	case *PianoRollDevice:
		return "Piano"
	case *EmptyDevice:
		return "(empty)"
	default:
		return "Unknown"
	}
}

func (s *SettingsDevice) RenderLEDs() []LEDState {
	var leds []LEDState

	trackColor := [3]uint8{100, 100, 200}
	selectedColor := [3]uint8{255, 255, 255}
	emptyColor := [3]uint8{30, 30, 50}

	// Show tracks in left column
	for row := 0; row < 8; row++ {
		var color [3]uint8
		if row == s.cursorRow {
			color = selectedColor
		} else if row < len(s.tracks) && s.tracks[row].Device != nil {
			color = trackColor
		} else {
			color = emptyColor
		}
		leds = append(leds, LEDState{Row: 7 - row, Col: 0, Color: color, Channel: midi.ChannelStatic})
	}

	return leds
}

func (s *SettingsDevice) HandleKey(key string) {
	// Handle popup navigation first
	if s.popup != nil {
		switch key {
		case "j", "down":
			if s.popup.Selected < len(s.popup.Options)-1 {
				s.popup.Selected++
			}
		case "k", "up":
			if s.popup.Selected > 0 {
				s.popup.Selected--
			}
		case "enter", " ":
			s.confirmPopupSelection()
		case "esc", "q":
			s.popup = nil
		}
		return
	}

	// Normal navigation
	switch key {
	case "h", "left":
		if s.cursorCol > 0 {
			s.cursorCol--
		}
	case "l", "right":
		if s.cursorCol < 2 {
			s.cursorCol++
		}
	case "j", "down":
		if s.cursorRow < 7 {
			s.cursorRow++
		}
	case "k", "up":
		if s.cursorRow > 0 {
			s.cursorRow--
		}
	case "enter", " ":
		s.openPopupForCurrentCell()
	}
}

func (s *SettingsDevice) openPopupForCurrentCell() {
	switch s.cursorCol {
	case 0: // Device type
		s.popup = &PopupState{
			Type:       PopupDeviceType,
			Options:    []string{"Drum", "Piano", "(empty)"},
			Selected:   0,
			TrackIndex: s.cursorRow,
		}
	case 1: // Channel
		options := make([]string, 16)
		for i := 0; i < 16; i++ {
			options[i] = fmt.Sprintf("Channel %d", i+1)
		}
		s.popup = &PopupState{
			Type:       PopupChannel,
			Options:    options,
			Selected:   0,
			TrackIndex: s.cursorRow,
		}
		// Pre-select current channel
		if s.cursorRow < len(s.tracks) {
			s.popup.Selected = int(s.tracks[s.cursorRow].Channel) - 1
			if s.popup.Selected < 0 {
				s.popup.Selected = 0
			}
		}
	case 2: // Output
		options := []string{"default"}
		options = append(options, s.midiOutputs...)
		s.popup = &PopupState{
			Type:       PopupOutput,
			Options:    options,
			Selected:   0,
			TrackIndex: s.cursorRow,
		}
	}
}

func (s *SettingsDevice) confirmPopupSelection() {
	if s.popup == nil {
		return
	}

	trackIdx := s.popup.TrackIndex
	if trackIdx >= len(s.tracks) {
		s.popup = nil
		return
	}
	track := s.tracks[trackIdx]

	switch s.popup.Type {
	case PopupDeviceType:
		// Check if track has content and we're changing type
		currentType := s.getDeviceTypeName(track)
		newType := s.popup.Options[s.popup.Selected]

		if currentType != "(empty)" && currentType != newType {
			// Need confirmation - check if device has content
			hasContent := false
			if track.Device != nil {
				mask := track.Device.ContentMask()
				for _, has := range mask {
					if has {
						hasContent = true
						break
					}
				}
			}

			if hasContent {
				// Show confirmation
				s.popup = &PopupState{
					Type:        PopupConfirm,
					Options:     []string{"Yes, change device", "No, cancel"},
					Selected:    1, // Default to cancel
					TrackIndex:  trackIdx,
					PendingType: s.optionToDeviceType(newType),
				}
				return
			}
		}

		// No confirmation needed, just change
		s.changeDeviceType(trackIdx, s.optionToDeviceType(newType))

	case PopupConfirm:
		if s.popup.Selected == 0 {
			// User confirmed
			s.changeDeviceType(s.popup.TrackIndex, s.popup.PendingType)
		}

	case PopupChannel:
		track.Channel = uint8(s.popup.Selected + 1)

	case PopupOutput:
		// TODO: implement per-track output
	}

	s.popup = nil
}

func (s *SettingsDevice) optionToDeviceType(opt string) DeviceType {
	switch opt {
	case "Drum":
		return DeviceTypeDrum
	case "Piano":
		return DeviceTypePiano
	default:
		return DeviceTypeNone
	}
}

func (s *SettingsDevice) changeDeviceType(trackIdx int, deviceType DeviceType) {
	if trackIdx >= len(s.tracks) {
		return
	}
	track := s.tracks[trackIdx]

	switch deviceType {
	case DeviceTypeDrum:
		track.Device = NewDrumDevice()
	case DeviceTypePiano:
		track.Device = NewPianoRollDevice()
	case DeviceTypeNone:
		track.Device = NewEmptyDevice(trackIdx + 1)
	}
}

func (s *SettingsDevice) HandlePad(row, col int) {
	// Could use pads to select tracks
	if col == 0 && row < 8 {
		s.cursorRow = 7 - row
	}
}

func (s *SettingsDevice) HelpLayout() widgets.LaunchpadLayout {
	trackColor := [3]uint8{100, 100, 200}
	dimColor := [3]uint8{30, 30, 50}

	var layout widgets.LaunchpadLayout

	// Left column shows tracks
	for row := 0; row < 8; row++ {
		if row < len(s.tracks) && s.tracks[row].Device != nil {
			layout.Grid[row][0] = widgets.PadConfig{Color: trackColor, Tooltip: fmt.Sprintf("Track %d", row+1)}
		} else {
			layout.Grid[row][0] = widgets.PadConfig{Color: dimColor, Tooltip: fmt.Sprintf("Track %d (empty)", row+1)}
		}
	}

	return layout
}
