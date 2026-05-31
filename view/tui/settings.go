package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
	"go-sequence/model/system"
	"go-sequence/theme"
	"go-sequence/widgets"
)

// settingsKey handles keyboard input on the settings view.
//
// Outside the popup:
//
//	h / left              cursor left  (col --)
//	l / right             cursor right (col ++)
//	j / down              cursor down  (track ++)
//	k / up                cursor up    (track --)
//	enter                 open the picker for the cell under the cursor
//	t / T                 cycle device type forward / backward (still works
//	                      without opening the popup, matches old shortcut)
//	[ / ]                 channel -- / ++ (no popup needed)
//	K                     cycle drum kit  (only when Type == DeviceDrum)
//	r                     rescan MIDI devices (no-op for now — gomidi
//	                      enumerates fresh on every popup open)
//
// Inside the popup (Popup.Open == true):
//
//	j / down              next option
//	k / up                previous option
//	enter                 commit selected option
//	esc / n / N           cancel and close the popup
//
// The settings cursor's row IS project.UI.Focus.Track — j/k advances
// Focus.Track directly so the dispatcher's track-lock and the cursor row
// stay synchronized. Col lives on project.UI.SystemSettings so it
// survives navigation away and back.
func settingsKey(project *model.Project, out midi.ToExternal, focusedTrack int, key string) {
	if project == nil {
		return
	}
	st := &project.UI.SystemSettings

	if st.Popup.Open {
		settingsPopupKey(project, st, key)
		return
	}

	switch key {
	case "h", "left":
		if st.Col > 0 {
			st.Col--
		}
	case "l", "right":
		if st.Col < system.SettingsColCount-1 {
			st.Col++
		}
	case "j", "down":
		if project.UI.Focus.Track < 7 {
			project.UI.Focus.Track++
		}
		return
	case "k", "up":
		if project.UI.Focus.Track > 0 {
			project.UI.Focus.Track--
		}
		return
	case "enter", "return":
		settingsOpenPopup(project, st)
	case "esc":
		project.UI.Focus.Kind = model.FocusSession
		return
	case "t":
		track := project.Tracks[project.UI.Focus.Track]
		if track != nil {
			settingsSetDeviceType(track, settingsNextDeviceKind(track.Type))
		}
	case "T":
		track := project.Tracks[project.UI.Focus.Track]
		if track != nil {
			settingsSetDeviceType(track, settingsPrevDeviceKind(track.Type))
		}
	case "[":
		track := project.Tracks[project.UI.Focus.Track]
		if track != nil && track.Channel > 0 {
			track.Channel--
		}
	case "]":
		track := project.Tracks[project.UI.Focus.Track]
		if track != nil && track.Channel < 15 {
			track.Channel++
		}
	case "K":
		track := project.Tracks[project.UI.Focus.Track]
		if track != nil && track.Type == model.DeviceDrum {
			track.Kit = settingsNextKit(track.Kit)
		}
	}
	_ = focusedTrack // dispatcher passes this; cursor row IS Focus.Track.
}

// settingsPopupKey handles keys while the modal picker is open.
func settingsPopupKey(project *model.Project, st *system.Settings, key string) {
	switch key {
	case "j", "down":
		if st.Popup.Selected < len(st.Popup.Options)-1 {
			st.Popup.Selected++
		}
	case "k", "up":
		if st.Popup.Selected > 0 {
			st.Popup.Selected--
		}
	case "enter", "return":
		settingsCommitPopup(project, st)
		settingsClosePopup(st)
	case "esc", "n", "N":
		settingsClosePopup(st)
	}
}

// settingsOpenPopup populates the popup with the right options for the
// cell under the cursor and opens it.
func settingsOpenPopup(project *model.Project, st *system.Settings) {
	track := project.Tracks[project.UI.Focus.Track]
	if track == nil {
		return
	}
	st.Popup.TrackIndex = project.UI.Focus.Track
	st.Popup.Open = true
	switch system.SettingsCol(st.Col) {
	case system.SettingsColType:
		st.Popup.Kind = system.PopupDeviceType
		st.Popup.Options = settingsDeviceTypeOptions()
		st.Popup.Selected = settingsDeviceKindIndex(track.Type)
	case system.SettingsColChannel:
		st.Popup.Kind = system.PopupChannel
		st.Popup.Options = settingsChannelOptions()
		st.Popup.Selected = int(track.Channel)
	case system.SettingsColPort:
		st.Popup.Kind = system.PopupOutput
		st.Popup.Options = settingsPortOptions()
		st.Popup.Selected = settingsPortIndex(st.Popup.Options, track.PortName)
	case system.SettingsColKit:
		if track.Type != model.DeviceDrum {
			// Kit is only meaningful for drum tracks.
			st.Popup.Open = false
			return
		}
		st.Popup.Kind = system.PopupKit
		st.Popup.Options = devices.KitNames()
		st.Popup.Selected = settingsKitIndex(st.Popup.Options, track.Kit)
	}
}

// settingsCommitPopup writes the selected option into the targeted track.
func settingsCommitPopup(project *model.Project, st *system.Settings) {
	if !st.Popup.Open {
		return
	}
	track := project.Tracks[st.Popup.TrackIndex]
	if track == nil {
		return
	}
	if st.Popup.Selected < 0 || st.Popup.Selected >= len(st.Popup.Options) {
		return
	}
	switch st.Popup.Kind {
	case system.PopupDeviceType:
		settingsSetDeviceType(track, settingsDeviceKindFromIndex(st.Popup.Selected))
	case system.PopupChannel:
		track.Channel = uint8(st.Popup.Selected)
	case system.PopupOutput:
		// First option is "(none)" — selecting it clears the port.
		if st.Popup.Selected == 0 {
			track.PortName = ""
		} else {
			track.PortName = st.Popup.Options[st.Popup.Selected]
		}
	case system.PopupKit:
		track.Kit = st.Popup.Options[st.Popup.Selected]
	}
}

func settingsClosePopup(st *system.Settings) {
	st.Popup.Open = false
	st.Popup.Kind = system.PopupNone
	st.Popup.Options = nil
	st.Popup.Selected = 0
}

// settingsRender returns the TUI view for the settings page.
//
// Layout (ported from old sequencer/settings.go):
//
//	header:  "SETTINGS  Track & MIDI Configuration"
//	table:   8 rows × 4 cols (Device | Channel | Output | Kit). Cursor cell
//	         marked with '[…]' brackets; the row prefix shows mute/solo flags.
//	midi:    available MIDI input + output ports listed below the table.
//	help:    key bindings.
//	pads:    Launchpad reference (left column lights up active tracks).
//	popup:   when Popup.Open, a modal box overlays the table area with a
//	         scrollable option list.
func settingsRender(project *model.Project, th *theme.Theme) string {
	if project == nil {
		return ""
	}
	st := &project.UI.SystemSettings
	cursorRow := project.UI.Focus.Track
	clampSettings(st, &project.UI.Focus.Track)

	titleStyle := themeStyle(th, roleFG).Bold(true)
	headerStyle := themeStyle(th, roleMuted)
	cursorStyle := themeStyle(th, roleCursor).Bold(true)
	mutedStyle := themeStyle(th, roleMuted)

	var b strings.Builder
	b.WriteString(titleStyle.Render("SETTINGS  Track & MIDI Configuration"))
	b.WriteString("\n\n")

	// Column headers.
	b.WriteString(headerStyle.Render(fmt.Sprintf("  %-4s  %-12s  %-7s  %-20s  %s",
		"Trk", "Type", "Channel", "Output", "Kit")))
	b.WriteString("\n")

	for row := 0; row < 8; row++ {
		track := project.Tracks[row]
		fmt.Fprintf(&b, "  %2d  ", row+1)

		typeText := sessionDeviceLabel(model.DeviceNone)
		channelText := "-"
		portText := "-"
		kitText := "-"
		if track != nil {
			typeText = sessionDeviceLabel(track.Type)
			channelText = fmt.Sprintf("%d", track.Channel+1)
			if track.PortName != "" {
				portText = track.PortName
			}
			if track.Type == model.DeviceDrum {
				kitText = settingsKitLabel(track.Kit)
			}
		}

		settingsWriteCell(&b, typeText, 12, row, 0, cursorRow, st, cursorStyle)
		b.WriteString("  ")
		settingsWriteCell(&b, channelText, 7, row, 1, cursorRow, st, cursorStyle)
		b.WriteString("  ")
		settingsWriteCell(&b, portText, 20, row, 2, cursorRow, st, cursorStyle)
		b.WriteString("  ")
		settingsWriteCell(&b, kitText, 10, row, 3, cursorRow, st, cursorStyle)
		b.WriteString("\n")
	}

	// MIDI device lists.
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("MIDI inputs:"))
	b.WriteString("\n")
	inputs := midi.InputPortNames()
	if len(inputs) == 0 {
		b.WriteString(mutedStyle.Render("  (none)"))
		b.WriteString("\n")
	} else {
		for _, n := range inputs {
			fmt.Fprintf(&b, "  %s\n", n)
		}
	}
	b.WriteString(headerStyle.Render("MIDI outputs:"))
	b.WriteString("\n")
	outputs := midi.OutputPortNames()
	if len(outputs) == 0 {
		b.WriteString(mutedStyle.Render("  (none)"))
		b.WriteString("\n")
	} else {
		for _, n := range outputs {
			fmt.Fprintf(&b, "  %s\n", n)
		}
	}

	// Key help — different binding set when popup is open vs closed so the
	// user's not staring at irrelevant shortcuts.
	b.WriteString("\n")
	if st.Popup.Open {
		b.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
			{Title: "Picker", Keys: []widgets.KeyBinding{
				{Key: "j / k", Desc: "next / previous option"},
				{Key: "enter", Desc: "commit selection"},
				{Key: "esc / n", Desc: "cancel"},
			}},
		}))
	} else {
		b.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
			{Keys: []widgets.KeyBinding{
				{Key: "h / l", Desc: "move cursor between columns"},
				{Key: "j / k", Desc: "move cursor between tracks"},
				{Key: "enter", Desc: "open picker for selected cell"},
				{Key: "esc", Desc: "back to session"},
			}},
			{Title: "Quick edits", Keys: []widgets.KeyBinding{
				{Key: "t / T", Desc: "cycle device type"},
				{Key: "[ / ]", Desc: "channel -/+"},
				{Key: "K", Desc: "cycle kit (drum)"},
			}},
		}))
	}

	// Launchpad reference.
	b.WriteString("\n\n")
	b.WriteString(settingsRenderLaunchpadHelp(project))
	b.WriteString("\n")

	// Popup overlay — append below the rest of the view rather than truly
	// floating, since lipgloss doesn't compose layered output cheaply and
	// the alt-screen TUI has plenty of vertical room.
	if st.Popup.Open {
		b.WriteString("\n")
		b.WriteString(settingsRenderPopup(st, th))
		b.WriteString("\n")
	}

	return b.String()
}

// settingsWriteCell emits a single table cell, wrapping it in cursor
// brackets when (row, col) matches the settings cursor. Width is the
// minimum field width for non-cursor cells.
func settingsWriteCell(b *strings.Builder, text string, width, row, col, cursorRow int, st *system.Settings, cursorStyle lipgloss.Style) {
	if row == cursorRow && col == st.Col {
		// Cursor cell: brackets eat 2 chars of the field width so things stay
		// aligned with non-cursor rows.
		inner := text
		if len(inner) > width-2 {
			inner = inner[:width-2]
		}
		fmt.Fprintf(b, "%s%-*s%s",
			cursorStyle.Render("["),
			width-2, inner,
			cursorStyle.Render("]"),
		)
		return
	}
	if len(text) > width {
		text = text[:width]
	}
	fmt.Fprintf(b, " %-*s ", width-2, text)
}

// settingsRenderPopup renders the modal picker as a bordered box containing
// the option list with the current selection highlighted.
func settingsRenderPopup(st *system.Settings, th *theme.Theme) string {
	if !st.Popup.Open {
		return ""
	}
	titleStyle := themeStyle(th, roleAccent).Bold(true)
	selStyle := themeStyle(th, roleCursor).Bold(true)
	border := "─────────────────────────────────────────────────"

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Picker — %s (track %d)",
		settingsPopupTitle(st.Popup.Kind), st.Popup.TrackIndex+1)))
	b.WriteString("\n")
	b.WriteString(border)
	b.WriteString("\n")
	for i, opt := range st.Popup.Options {
		marker := "  "
		if i == st.Popup.Selected {
			marker = "> "
		}
		line := fmt.Sprintf("%s%s", marker, opt)
		if i == st.Popup.Selected {
			b.WriteString(selStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	b.WriteString(border)
	return b.String()
}

func settingsPopupTitle(kind system.PopupKind) string {
	switch kind {
	case system.PopupDeviceType:
		return "Device Type"
	case system.PopupChannel:
		return "MIDI Channel"
	case system.PopupOutput:
		return "MIDI Output"
	case system.PopupKit:
		return "Drum Kit"
	}
	return "?"
}

// clampSettings bounds the settings cursor + popup state into valid ranges.
// cursorRow is a pointer so we can clamp the global Focus.Track in place.
func clampSettings(st *system.Settings, cursorRow *int) {
	if *cursorRow < 0 {
		*cursorRow = 0
	}
	if *cursorRow > 7 {
		*cursorRow = 7
	}
	if st.Col < 0 {
		st.Col = 0
	}
	if st.Col > system.SettingsColCount-1 {
		st.Col = system.SettingsColCount - 1
	}
	if st.Popup.Open {
		if st.Popup.Selected < 0 {
			st.Popup.Selected = 0
		}
		if len(st.Popup.Options) > 0 && st.Popup.Selected >= len(st.Popup.Options) {
			st.Popup.Selected = len(st.Popup.Options) - 1
		}
	}
}

// settingsKitLabel returns the user-visible kit name, falling back to "-"
// for an empty kit string.
func settingsKitLabel(kit string) string {
	if kit == "" {
		return "-"
	}
	return kit
}

// --- mutation helpers (package-private; caller holds track.mu) ---

// settingsSetDeviceType changes a track's device Type and reshapes its device
// pointers so exactly one is non-nil (matching the new Type). Validate() is
// called on the new device so patterns get default lengths.
func settingsSetDeviceType(track *model.Track, kind model.DeviceKind) {
	track.Type = kind
	switch kind {
	case model.DeviceNone:
		track.Device = nil
	case model.DeviceDrum:
		if _, ok := track.Device.(*devices.Drum); !ok {
			track.Device = &devices.Drum{}
		}
		track.Device.(*devices.Drum).Validate()
	case model.DeviceLooper:
		if _, ok := track.Device.(*devices.Looper); !ok {
			track.Device = &devices.Looper{}
		}
		track.Device.(*devices.Looper).Validate()
	}
}

func settingsDeviceKindIndex(kind model.DeviceKind) int {
	for i, k := range model.DeviceCycle {
		if k == kind {
			return i
		}
	}
	return 0
}

func settingsDeviceKindFromIndex(idx int) model.DeviceKind {
	if idx < 0 || idx >= len(model.DeviceCycle) {
		return model.DeviceNone
	}
	return model.DeviceCycle[idx]
}

func settingsDeviceTypeOptions() []string {
	out := make([]string, len(model.DeviceCycle))
	for i, k := range model.DeviceCycle {
		out[i] = model.DeviceLabel(k)
	}
	return out
}

func settingsNextDeviceKind(kind model.DeviceKind) model.DeviceKind {
	i := settingsDeviceKindIndex(kind)
	return model.DeviceCycle[(i+1)%len(model.DeviceCycle)]
}

func settingsPrevDeviceKind(kind model.DeviceKind) model.DeviceKind {
	i := settingsDeviceKindIndex(kind)
	return model.DeviceCycle[(i-1+len(model.DeviceCycle))%len(model.DeviceCycle)]
}

func settingsChannelOptions() []string {
	out := make([]string, 16)
	for i := 0; i < 16; i++ {
		out[i] = fmt.Sprintf("Channel %d", i+1)
	}
	return out
}

// settingsPortOptions returns the OS-level MIDI output ports plus a
// "(none)" sentinel at index 0 so the user can clear a port assignment
// from the picker.
func settingsPortOptions() []string {
	ports := midi.OutputPortNames()
	out := make([]string, 0, len(ports)+1)
	out = append(out, "(none)")
	out = append(out, ports...)
	return out
}

func settingsPortIndex(options []string, current string) int {
	if current == "" {
		return 0
	}
	for i, o := range options {
		if o == current {
			return i
		}
	}
	return 0
}

func settingsKitIndex(options []string, current string) int {
	for i, o := range options {
		if o == current {
			return i
		}
	}
	return 0
}

// settingsNextKit returns the next kit name in devices.KitNames(), wrapping.
// An unknown current kit maps to the first kit in the list.
func settingsNextKit(current string) string {
	names := devices.KitNames()
	if len(names) == 0 {
		return current
	}
	for i, n := range names {
		if n == current {
			return names[(i+1)%len(names)]
		}
	}
	return names[0]
}

// settingsRenderLaunchpadHelp renders the Launchpad reference for the
// settings page: a fully-dim 8×8 grid except the left column, which lights
// up for any track that has a device assigned. Ports the old
// sequencer/settings.go renderLaunchpadHelp() to the new model API.
func settingsRenderLaunchpadHelp(project *model.Project) string {
	trackColor := [3]uint8{100, 100, 200}
	dimColor := [3]uint8{30, 30, 50}

	var grid [8][8][3]uint8
	topRow := make([][3]uint8, 8)
	for i := 0; i < 8; i++ {
		topRow[i] = dimColor
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = dimColor
		}
	}
	for row := 0; row < 8; row++ {
		track := project.Tracks[row]
		if track != nil && track.Type != model.DeviceNone {
			grid[row][0] = trackColor
		}
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, nil) + "\n\n"
	out += widgets.RenderLegendItem(trackColor, "Tracks", "select track to configure")
	return out
}
