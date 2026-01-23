package sequencer

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/widgets"
)

// InputMode for text input
type InputMode int

const (
	InputNone InputMode = iota
	InputNewProject
	InputRenameProject
	InputRenameSave
)

// SaveDevice manages project save/load
type SaveDevice struct {
	manager *Manager

	// Cached data
	projects []string
	saves    []SaveInfo

	// Selection state
	projectIdx int // selected project
	saveIdx    int // selected save
	column     int // 0=projects, 1=saves

	// Input mode (for new project / rename)
	inputMode   InputMode
	inputBuffer string

	// Confirmation dialog
	confirmMode   bool
	confirmMsg    string
	confirmAction func()
}

// NewSaveDevice creates a save device
func NewSaveDevice(manager *Manager) *SaveDevice {
	s := &SaveDevice{
		manager:    manager,
		projectIdx: 0,
		saveIdx:    0,
		column:     0,
	}
	s.Refresh()
	return s
}

// IsInputMode returns true if the device is accepting text input
func (s *SaveDevice) IsInputMode() bool {
	return s.inputMode != InputNone || s.confirmMode
}

// Refresh reloads project and save lists
func (s *SaveDevice) Refresh() {
	projects, _ := ListProjects()
	s.projects = projects

	// Clamp selection
	if s.projectIdx >= len(s.projects) {
		s.projectIdx = max(0, len(s.projects)-1)
	}

	// Load saves for selected project
	if len(s.projects) > 0 && s.projectIdx < len(s.projects) {
		saves, _ := ListSaves(s.projects[s.projectIdx])
		s.saves = saves
	} else {
		s.saves = nil
	}

	// Clamp save selection
	if s.saveIdx >= len(s.saves) {
		s.saveIdx = max(0, len(s.saves)-1)
	}
}

// Device interface implementation

func (s *SaveDevice) Tick(step int) []midi.Event {
	return nil // Save device doesn't produce MIDI
}

func (s *SaveDevice) QueuePattern(p int) (pattern, next int) {
	return 0, 0
}

func (s *SaveDevice) ContentMask() []bool {
	return make([]bool, NumPatterns)
}

func (s *SaveDevice) HandleMIDI(event midi.Event) {}

func (s *SaveDevice) ToggleRecording() {}
func (s *SaveDevice) TogglePreview()   {}
func (s *SaveDevice) IsRecording() bool  { return false }
func (s *SaveDevice) IsPreviewing() bool { return false }

func (s *SaveDevice) View() string {
	var out strings.Builder

	// Header
	projectName := "(none)"
	if S.ProjectName != "" {
		projectName = S.ProjectName
	}
	out.WriteString(fmt.Sprintf("SAVE  Project: %s\n\n", projectName))

	// Confirmation dialog takes over
	if s.confirmMode {
		out.WriteString("─────────────────────────────────────────────────\n")
		out.WriteString(fmt.Sprintf("\n%s\n\n", s.confirmMsg))
		out.WriteString("  [y] Yes    [n] No\n")
		out.WriteString("\n─────────────────────────────────────────────────\n")
		return out.String()
	}

	// Input mode takes over
	if s.inputMode != InputNone {
		var label string
		switch s.inputMode {
		case InputNewProject:
			label = "New project name"
		case InputRenameProject:
			label = "Rename project to"
		case InputRenameSave:
			label = "Name this save"
		}
		out.WriteString("─────────────────────────────────────────────────\n")
		out.WriteString(fmt.Sprintf("\n%s: %s_\n", label, s.inputBuffer))
		out.WriteString("\n[enter] confirm  [esc] cancel\n")
		out.WriteString("\n─────────────────────────────────────────────────\n")
		return out.String()
	}

	// Two column layout
	out.WriteString("Projects                    Saves\n")
	out.WriteString("─────────────────────────────────────────────────\n")

	// Calculate max rows to display
	maxRows := 12
	projectRows := min(maxRows, max(1, len(s.projects)))
	saveRows := min(maxRows, max(1, len(s.saves)))
	rows := max(projectRows, saveRows)

	for row := 0; row < rows; row++ {
		// Project column
		if row < len(s.projects) {
			prefix := "  "
			if row == s.projectIdx {
				if s.column == 0 {
					prefix = "> "
				} else {
					prefix = "* "
				}
			}
			name := s.projects[row]
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			out.WriteString(fmt.Sprintf("%s%-20s", prefix, name))
		} else {
			out.WriteString("                      ")
		}

		out.WriteString("    ")

		// Saves column
		if row < len(s.saves) {
			prefix := "  "
			if row == s.saveIdx {
				if s.column == 1 {
					prefix = "> "
				} else {
					prefix = "* "
				}
			}
			// Format: timestamp + name if present
			save := s.saves[row]
			display := save.Timestamp.Format("01-02 15:04")
			if save.Name != "" {
				display += " " + save.Name
			}
			if len(display) > 24 {
				display = display[:21] + "..."
			}
			out.WriteString(fmt.Sprintf("%s%s", prefix, display))
		}

		out.WriteString("\n")
	}

	if len(s.projects) == 0 {
		out.WriteString("  (no projects yet)\n")
	}

	// Key help
	out.WriteString("\n")
	out.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: "h / l", Desc: "switch columns"},
			{Key: "j / k", Desc: "navigate list"},
			{Key: "enter", Desc: "load selected"},
			{Key: "n", Desc: "new project"},
			{Key: "r", Desc: "rename project"},
			{Key: "d", Desc: "delete"},
		}},
	}))

	// Launchpad
	out.WriteString("\n\n")
	out.WriteString(s.renderLaunchpadHelp())

	return out.String()
}

func (s *SaveDevice) RenderLEDs() []LEDState {
	var leds []LEDState

	projectColor := [3]uint8{100, 200, 100}
	saveColor := [3]uint8{100, 100, 200}
	selectedColor := [3]uint8{255, 255, 255}
	emptyColor := [3]uint8{30, 30, 30}

	// Left half: projects (cols 0-3)
	for row := 0; row < 8; row++ {
		for col := 0; col < 4; col++ {
			idx := row*4 + col
			var color [3]uint8
			if idx < len(s.projects) {
				if idx == s.projectIdx && s.column == 0 {
					color = selectedColor
				} else {
					color = projectColor
				}
			} else {
				color = emptyColor
			}
			leds = append(leds, LEDState{Row: 7 - row, Col: col, Color: color, Channel: midi.ChannelStatic})
		}
	}

	// Right half: saves (cols 4-7)
	for row := 0; row < 8; row++ {
		for col := 4; col < 8; col++ {
			idx := row*4 + (col - 4)
			var color [3]uint8
			if idx < len(s.saves) {
				if idx == s.saveIdx && s.column == 1 {
					color = selectedColor
				} else {
					color = saveColor
				}
			} else {
				color = emptyColor
			}
			leds = append(leds, LEDState{Row: 7 - row, Col: col, Color: color, Channel: midi.ChannelStatic})
		}
	}

	return leds
}

func (s *SaveDevice) HandleKey(key string) {
	// Confirmation mode
	if s.confirmMode {
		switch key {
		case "y", "Y":
			if s.confirmAction != nil {
				s.confirmAction()
			}
			s.confirmMode = false
			s.confirmAction = nil
			s.Refresh()
		case "n", "N", "esc", "q":
			s.confirmMode = false
			s.confirmAction = nil
		}
		return
	}

	// Input mode
	if s.inputMode != InputNone {
		switch key {
		case "enter":
			s.commitInput()
		case "esc":
			s.inputMode = InputNone
			s.inputBuffer = ""
		case "backspace":
			if len(s.inputBuffer) > 0 {
				s.inputBuffer = s.inputBuffer[:len(s.inputBuffer)-1]
			}
		default:
			// Only accept printable characters
			if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
				// Don't allow path separators
				if key != "/" && key != "\\" {
					s.inputBuffer += key
				}
			}
		}
		return
	}

	// Normal navigation
	switch key {
	case "h", "left":
		s.column = 0
	case "l", "right":
		if len(s.projects) > 0 {
			s.column = 1
		}
	case "j", "down":
		if s.column == 0 {
			if s.projectIdx < len(s.projects)-1 {
				s.projectIdx++
				s.Refresh() // reload saves for new project
			}
		} else {
			if s.saveIdx < len(s.saves)-1 {
				s.saveIdx++
			}
		}
	case "k", "up":
		if s.column == 0 {
			if s.projectIdx > 0 {
				s.projectIdx--
				s.Refresh() // reload saves for new project
			}
		} else {
			if s.saveIdx > 0 {
				s.saveIdx--
			}
		}
	case "enter", " ":
		s.loadSelected()
	case "n":
		s.inputMode = InputNewProject
		s.inputBuffer = ""
	case "r":
		if s.column == 0 && len(s.projects) > 0 {
			// Rename project
			s.inputMode = InputRenameProject
			s.inputBuffer = s.projects[s.projectIdx]
		} else if s.column == 1 && len(s.saves) > 0 {
			// Rename save
			s.inputMode = InputRenameSave
			s.inputBuffer = s.saves[s.saveIdx].Name
		}
	case "d":
		s.deleteSelected()
	}
}

func (s *SaveDevice) commitInput() {
	name := strings.TrimSpace(s.inputBuffer)

	switch s.inputMode {
	case InputNewProject:
		if name != "" {
			CreateProject(name)
			S.ProjectName = name
		}
	case InputRenameProject:
		if name != "" && len(s.projects) > 0 {
			oldName := s.projects[s.projectIdx]
			RenameProject(oldName, name)
		}
	case InputRenameSave:
		// Empty name is allowed (removes the name)
		if len(s.saves) > 0 {
			oldFilename := s.saves[s.saveIdx].Filename
			RenameSave(s.projects[s.projectIdx], oldFilename, name)
		}
	}

	s.inputMode = InputNone
	s.inputBuffer = ""
	s.Refresh()
}

func (s *SaveDevice) loadSelected() {
	if len(s.projects) == 0 {
		return
	}

	projectName := s.projects[s.projectIdx]
	filename := ""

	if s.column == 1 && len(s.saves) > 0 {
		filename = s.saves[s.saveIdx].Filename
	}

	if err := LoadProject(projectName, filename); err != nil {
		return // TODO: show error
	}

	// Recreate devices from loaded state
	s.manager.recreateDevicesFromState()
}

func (s *SaveDevice) deleteSelected() {
	if s.column == 0 {
		// Delete project
		if len(s.projects) == 0 {
			return
		}
		name := s.projects[s.projectIdx]
		s.confirmMsg = fmt.Sprintf("Delete project '%s' and all saves?", name)
		s.confirmAction = func() {
			DeleteProject(name)
			if S.ProjectName == name {
				S.ProjectName = ""
			}
		}
		s.confirmMode = true
	} else {
		// Delete save
		if len(s.saves) == 0 {
			return
		}
		save := s.saves[s.saveIdx]
		s.confirmMsg = fmt.Sprintf("Delete save '%s'?", save.Timestamp.Format("2006-01-02 15:04:05"))
		s.confirmAction = func() {
			DeleteSave(s.projects[s.projectIdx], save.Filename)
		}
		s.confirmMode = true
	}
}

func (s *SaveDevice) HandlePad(row, col int) {
	// Left half: select project
	if col < 4 {
		idx := (7-row)*4 + col
		if idx < len(s.projects) {
			s.projectIdx = idx
			s.column = 0
			s.Refresh()
		}
	} else {
		// Right half: select save
		idx := (7-row)*4 + (col - 4)
		if idx < len(s.saves) {
			s.saveIdx = idx
			s.column = 1
		}
	}
}

func (s *SaveDevice) renderLaunchpadHelp() string {
	projectColor := [3]uint8{100, 200, 100}
	saveColor := [3]uint8{100, 100, 200}
	dimColor := [3]uint8{30, 30, 30}

	var grid [8][8][3]uint8
	topRow := make([][3]uint8, 8)

	for i := 0; i < 8; i++ {
		topRow[i] = dimColor
	}

	// Left half: projects
	for row := 0; row < 8; row++ {
		for col := 0; col < 4; col++ {
			idx := row*4 + col
			if idx < len(s.projects) {
				grid[row][col] = projectColor
			} else {
				grid[row][col] = dimColor
			}
		}
	}

	// Right half: saves
	for row := 0; row < 8; row++ {
		for col := 4; col < 8; col++ {
			idx := row*4 + (col - 4)
			if idx < len(s.saves) {
				grid[row][col] = saveColor
			} else {
				grid[row][col] = dimColor
			}
		}
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, nil) + "\n\n"
	out += widgets.RenderLegendItem(projectColor, "Projects", "select project") + "\n"
	out += widgets.RenderLegendItem(saveColor, "Saves", "select save")

	return out
}
