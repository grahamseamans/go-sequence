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
	PopupKit
	PopupConfirm
	PopupNoteInput
)

// PopupState holds the state of an open popup
type PopupState struct {
	Type        PopupType
	Options     []string
	Selected    int
	TrackIndex  int        // which track this popup is for
	PendingType DeviceType // for confirmation dialogs
}

// SettingsDevice manages track and MIDI configuration
type SettingsDevice struct {
	manager *Manager // reference for device access and creation

	// Cursor position
	cursorRow int // 0-7 for tracks, 8 for note input
	cursorCol int // 0=device, 1=channel, 2=output

	// Popup state
	popup *PopupState

	// Available MIDI ports (cached from last scan)
	midiInputs  []string
	midiOutputs []string

	// Flag to signal TUI that note input changed (checked after HandleKey)
	NoteInputChanged bool
}

// NewSettingsDevice creates a settings device
func NewSettingsDevice(manager *Manager) *SettingsDevice {
	return &SettingsDevice{
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

// Device interface implementation - queue-based (stubs for non-music device)

func (s *SettingsDevice) FillUntil(tick int64)           {}
func (s *SettingsDevice) PeekNextEvent() *midi.Event     { return nil }
func (s *SettingsDevice) PopNextEvent() *midi.Event      { return nil }
func (s *SettingsDevice) ClearQueue()                    {}
func (s *SettingsDevice) QueuePattern(p int, atTick int64) {}
func (s *SettingsDevice) CurrentPattern() int            { return 0 }
func (s *SettingsDevice) NextPattern() int               { return -1 }
func (s *SettingsDevice) ContentMask() []bool            { return make([]bool, NumPatterns) }

func (s *SettingsDevice) HandleMIDI(event midi.Event) {
	// Could use this for "learn" functionality later
}

func (s *SettingsDevice) ToggleRecording() {}
func (s *SettingsDevice) TogglePreview()   {}
func (s *SettingsDevice) IsRecording() bool { return false }
func (s *SettingsDevice) IsPreviewing() bool { return false }

func (s *SettingsDevice) View() string {
	var out strings.Builder

	out.WriteString("SETTINGS  Track & MIDI Configuration\n\n")

	// Track table header
	out.WriteString("Track   Device       Channel   Output         Kit\n")
	out.WriteString("────────────────────────────────────────────────────────────\n")

	// Track rows
	for i := 0; i < 8; i++ {
		ts := S.Tracks[i]
		dev := s.manager.GetDevice(i)

		// Track number
		out.WriteString(fmt.Sprintf("  %d     ", i+1))

		// Device type cell
		deviceStr := s.getDeviceTypeName(i)
		if s.cursorRow == i && s.cursorCol == 0 {
			out.WriteString(fmt.Sprintf("[%-8s]  ", deviceStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-8s   ", deviceStr))
		}

		// Channel cell
		channelStr := fmt.Sprintf("ch %d", ts.Channel)
		if s.cursorRow == i && s.cursorCol == 1 {
			out.WriteString(fmt.Sprintf("[%-5s]  ", channelStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-5s   ", channelStr))
		}

		// Output cell
		outputStr := "(default)"
		if ts.PortName != "" {
			// Truncate long port names
			outputStr = ts.PortName
			if len(outputStr) > 12 {
				outputStr = outputStr[:12]
			}
		} else if dev == nil || ts.Type == DeviceTypeNone {
			outputStr = "-"
		}
		if s.cursorRow == i && s.cursorCol == 2 {
			out.WriteString(fmt.Sprintf("[%-12s]  ", outputStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-12s   ", outputStr))
		}

		// Kit cell (only for drum devices)
		kitStr := "-"
		if ts.Type == DeviceTypeDrum {
			kit := GetKit(ts.Kit)
			kitStr = kit.Name
			if len(kitStr) > 12 {
				kitStr = kitStr[:12]
			}
		}
		if s.cursorRow == i && s.cursorCol == 3 {
			out.WriteString(fmt.Sprintf("[%-12s]", kitStr))
		} else {
			out.WriteString(fmt.Sprintf(" %-12s", kitStr))
		}

		out.WriteString("\n")
	}

	// Note Input selection row
	out.WriteString("\n")
	out.WriteString("─────────────────────────────────────────────────\n")
	noteInputStr := "(none)"
	if S.NoteInputPort != "" {
		noteInputStr = S.NoteInputPort
		if len(noteInputStr) > 30 {
			noteInputStr = noteInputStr[:30]
		}
	}
	if s.cursorRow == 8 {
		out.WriteString(fmt.Sprintf("Note Input:  [%-30s]\n", noteInputStr))
	} else {
		out.WriteString(fmt.Sprintf("Note Input:   %-30s\n", noteInputStr))
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
	out.WriteString(s.renderLaunchpadHelp())

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
	case PopupKit:
		title = "Drum Kit"
	case PopupConfirm:
		title = "Confirm"
	case PopupNoteInput:
		title = "Note Input"
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

func (s *SettingsDevice) getDeviceTypeName(trackIdx int) string {
	ts := S.Tracks[trackIdx]
	switch ts.Type {
	case DeviceTypeDrum:
		return "Drum"
	case DeviceTypePiano:
		return "Piano"
	case DeviceTypeMetropolix:
		return "Metropolix"
	default:
		return "(empty)"
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
		} else if S.Tracks[row].Type != DeviceTypeNone {
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
		if s.cursorRow < 8 && s.cursorCol > 0 {
			s.cursorCol--
		}
	case "l", "right":
		if s.cursorRow < 8 && s.cursorCol < 3 {
			s.cursorCol++
		}
	case "j", "down":
		if s.cursorRow < 8 {
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
	// Note Input row (row 8)
	if s.cursorRow == 8 {
		options := []string{"(none)"}
		options = append(options, s.midiInputs...)
		selected := 0
		// Find current port in list
		for i, port := range s.midiInputs {
			if port == S.NoteInputPort {
				selected = i + 1 // +1 because "(none)" is at index 0
				break
			}
		}
		s.popup = &PopupState{
			Type:     PopupNoteInput,
			Options:  options,
			Selected: selected,
		}
		return
	}

	// Track rows (0-7)
	switch s.cursorCol {
	case 0: // Device type
		s.popup = &PopupState{
			Type:       PopupDeviceType,
			Options:    []string{"Drum", "Piano", "Metropolix", "(empty)"},
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
			Selected:   int(S.Tracks[s.cursorRow].Channel) - 1,
			TrackIndex: s.cursorRow,
		}
		if s.popup.Selected < 0 {
			s.popup.Selected = 0
		}
	case 2: // Output
		options := []string{"(default)"}
		options = append(options, s.midiOutputs...)
		selected := 0
		// Find current port in list
		for i, port := range s.midiOutputs {
			if port == S.Tracks[s.cursorRow].PortName {
				selected = i + 1 // +1 because "(default)" is at index 0
				break
			}
		}
		s.popup = &PopupState{
			Type:       PopupOutput,
			Options:    options,
			Selected:   selected,
			TrackIndex: s.cursorRow,
		}
	case 3: // Kit (only for drum devices)
		ts := S.Tracks[s.cursorRow]
		if ts.Type != DeviceTypeDrum {
			return // Can't select kit for non-drum devices
		}
		kitNames := KitNames()
		options := make([]string, len(kitNames))
		selected := 0
		for i, name := range kitNames {
			kit := GetKit(name)
			options[i] = kit.Name
			if name == ts.Kit || (ts.Kit == "" && name == DefaultKit) {
				selected = i
			}
		}
		s.popup = &PopupState{
			Type:       PopupKit,
			Options:    options,
			Selected:   selected,
			TrackIndex: s.cursorRow,
		}
	}
}

func (s *SettingsDevice) confirmPopupSelection() {
	if s.popup == nil {
		return
	}

	switch s.popup.Type {
	case PopupDeviceType:
		trackIdx := s.popup.TrackIndex
		// Check if track has content and we're changing type
		currentType := s.getDeviceTypeName(trackIdx)
		newType := s.popup.Options[s.popup.Selected]

		if currentType != "(empty)" && currentType != newType {
			// Need confirmation - check if device has content
			hasContent := false
			dev := s.manager.GetDevice(trackIdx)
			if dev != nil {
				mask := dev.ContentMask()
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
		ts := S.Tracks[s.popup.TrackIndex]
		ts.Channel = uint8(s.popup.Selected + 1)

	case PopupOutput:
		ts := S.Tracks[s.popup.TrackIndex]
		if s.popup.Selected == 0 {
			ts.PortName = "" // default
		} else {
			ts.PortName = s.midiOutputs[s.popup.Selected-1]
		}

	case PopupKit:
		ts := S.Tracks[s.popup.TrackIndex]
		kitNames := KitNames()
		if s.popup.Selected >= 0 && s.popup.Selected < len(kitNames) {
			ts.Kit = kitNames[s.popup.Selected]
		}

	case PopupNoteInput:
		var portName string
		if s.popup.Selected == 0 {
			portName = "" // none
		} else {
			portName = s.midiInputs[s.popup.Selected-1]
		}
		S.NoteInputPort = portName
		// Signal TUI to connect (TUI checks this flag after HandleKey)
		s.NoteInputChanged = true
	}

	s.popup = nil
}

func (s *SettingsDevice) optionToDeviceType(opt string) DeviceType {
	switch opt {
	case "Drum":
		return DeviceTypeDrum
	case "Piano":
		return DeviceTypePiano
	case "Metropolix":
		return DeviceTypeMetropolix
	default:
		return DeviceTypeNone
	}
}

func (s *SettingsDevice) changeDeviceType(trackIdx int, deviceType DeviceType) {
	var dev Device
	switch deviceType {
	case DeviceTypeDrum:
		dev = s.manager.CreateDrumDevice(trackIdx)
	case DeviceTypePiano:
		dev = s.manager.CreatePianoDevice(trackIdx)
	case DeviceTypeMetropolix:
		dev = s.manager.CreateMetropolixDevice(trackIdx)
	case DeviceTypeNone:
		dev = s.manager.CreateEmptyDevice(trackIdx)
	}
	s.manager.SetDevice(trackIdx, dev)
}

func (s *SettingsDevice) HandlePad(row, col int) {
	// Could use pads to select tracks
	if col == 0 && row < 8 {
		s.cursorRow = 7 - row
	}
}

func (s *SettingsDevice) renderLaunchpadHelp() string {
	trackColor := [3]uint8{100, 100, 200}
	dimColor := [3]uint8{30, 30, 50}

	var grid [8][8][3]uint8
	topRow := make([][3]uint8, 8)

	// All dim by default
	for i := 0; i < 8; i++ {
		topRow[i] = dimColor
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = dimColor
		}
	}

	// Left column shows tracks
	for row := 0; row < 8; row++ {
		if S.Tracks[row].Type != DeviceTypeNone {
			grid[row][0] = trackColor
		}
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, nil) + "\n\n"
	out += widgets.RenderLegendItem(trackColor, "Tracks", "select track to configure")

	return out
}
