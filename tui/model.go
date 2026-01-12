package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-sequence/config"
	"go-sequence/midi"
	"go-sequence/sequencer"
	"go-sequence/theme"
	"go-sequence/widgets"
)

// layoutBounds holds cached layout info
type layoutBounds struct {
	lpHelpTop    int
	lpHelpHeight int
}

type Model struct {
	Manager    *sequencer.Manager
	DeviceMgr  *midi.DeviceManager
	Config     *config.Config
	Theme      *theme.Theme
	lpHelp     *widgets.LaunchpadHelp
	quitting   bool
	mouseX     int
	mouseY     int
	tooltip    string
	bounds     *layoutBounds
	controller midi.Controller // current controller (may be nil)
	statusMsg  string          // temporary status message
}

type UpdateMsg struct{}

type RescanResultMsg struct {
	controller midi.Controller
	err        error
}

func NewModel(manager *sequencer.Manager, deviceMgr *midi.DeviceManager, cfg *config.Config, th *theme.Theme) Model {
	lp := widgets.NewLaunchpadHelp()
	if focused := manager.GetFocused(); focused != nil {
		lp.SetLayout(focused.HelpLayout())
	}
	// Get already-connected controller if any
	controller := deviceMgr.GetController()
	return Model{
		Manager:    manager,
		DeviceMgr:  deviceMgr,
		Config:     cfg,
		Theme:      th,
		lpHelp:     lp,
		bounds:     &layoutBounds{},
		controller: controller,
	}
}

func ListenForUpdates(manager *sequencer.Manager) tea.Cmd {
	return func() tea.Msg {
		<-manager.UpdateChan
		return UpdateMsg{}
	}
}

// RescanDevices attempts to connect to a controller (runs in background with timeout)
func RescanDevices(deviceMgr *midi.DeviceManager, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		err := deviceMgr.Connect(cfg)
		if err != nil {
			return RescanResultMsg{err: err}
		}
		return RescanResultMsg{controller: deviceMgr.GetController()}
	}
}

func (m Model) Init() tea.Cmd {
	// Start listening for pad events if controller already connected
	var cmds []tea.Cmd
	cmds = append(cmds, ListenForUpdates(m.Manager))

	if m.controller != nil {
		cmds = append(cmds, m.listenForPads())
	}

	return tea.Batch(cmds...)
}

// listenForPads creates a command that listens for pad events from the controller
func (m Model) listenForPads() tea.Cmd {
	if m.controller == nil {
		return nil
	}
	return func() tea.Msg {
		for pad := range m.controller.PadEvents() {
			m.Manager.HandlePad(pad.Row, pad.Col)
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.Manager.Stop()
			return m, tea.Quit

		case "p":
			_, playing, _ := m.Manager.GetState()
			if playing {
				m.Manager.Stop()
			} else {
				m.Manager.Play()
			}

		case "+", "=":
			_, _, tempo := m.Manager.GetState()
			m.Manager.SetTempo(tempo + 5)

		case "-", "_":
			_, _, tempo := m.Manager.GetState()
			m.Manager.SetTempo(tempo - 5)

		case "r":
			// Manual rescan for MIDI devices
			m.statusMsg = "Scanning..."
			return m, RescanDevices(m.DeviceMgr, m.Config)

		case "0":
			m.Manager.FocusSession()
			if focused := m.Manager.GetFocused(); focused != nil {
				m.lpHelp.SetLayout(focused.HelpLayout())
			}

		case "1", "2", "3", "4", "5", "6", "7", "8":
			idx := int(msg.String()[0] - '1')
			m.Manager.FocusDevice(idx)
			if focused := m.Manager.GetFocused(); focused != nil {
				m.lpHelp.SetLayout(focused.HelpLayout())
			}

		default:
			m.Manager.HandleKey(msg.String())
		}

	case tea.MouseMsg:
		m.mouseX, m.mouseY = msg.X, msg.Y
		m.tooltip = m.hitTest(msg.X, msg.Y)

	case UpdateMsg:
		return m, ListenForUpdates(m.Manager)

	case RescanResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("No device: %v", msg.err)
			m.controller = nil
			m.Manager.SetController(nil)
		} else if msg.controller != nil {
			m.statusMsg = fmt.Sprintf("Connected: %s", msg.controller.ID())
			m.controller = msg.controller
			m.Manager.SetController(msg.controller)
			// Start listening for pad events
			return m, m.listenForPads()
		}
	}

	return m, nil
}

func (m Model) hitTest(x, y int) string {
	if y >= m.bounds.lpHelpTop && y < m.bounds.lpHelpTop+m.bounds.lpHelpHeight {
		relX := x
		relY := y - m.bounds.lpHelpTop
		if hit, tooltip := m.lpHelp.HitTest(relX, relY); hit {
			return tooltip
		}
	}
	return ""
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	step, playing, tempo := m.Manager.GetState()

	// Styles
	headerStyle := lipgloss.NewStyle().Foreground(m.Theme.Accent())
	dimStyle := lipgloss.NewStyle().Foreground(m.Theme.Muted())
	tooltipStyle := lipgloss.NewStyle().
		Foreground(m.Theme.FG()).
		Background(m.Theme.Muted()).
		Padding(0, 1)

	// Header with device status
	playState := "STOP"
	if playing {
		playState = "PLAY"
	}

	deviceStatus := " [no ctrl - r:scan]"
	if m.controller != nil {
		deviceStatus = " [LP:X]"
	}

	header := headerStyle.Render(fmt.Sprintf("go-sequence  %s  %3dbpm  step:%02d%s", playState, tempo, step, deviceStatus))

	// Device view
	deviceView := m.Manager.View()

	// Launchpad help
	lpView := m.lpHelp.View()

	// Help line
	help := dimStyle.Render("0:session 1-8:device  hjkl:nav  space:toggle  p:play  +/-:tempo  r:scan  q:quit")

	// Compute layout bounds
	headerHeight := lipgloss.Height(header)
	deviceHeight := lipgloss.Height(deviceView)
	m.bounds.lpHelpTop = 1 + headerHeight + 1 + deviceHeight
	m.bounds.lpHelpHeight = lipgloss.Height(lpView)

	// Build output
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(header)
	out.WriteString("\n\n")
	out.WriteString(deviceView)
	out.WriteString(lpView)
	out.WriteString("\n\n")
	out.WriteString(help)

	if m.tooltip != "" {
		out.WriteString("\n")
		out.WriteString(tooltipStyle.Render(m.tooltip))
	}

	return out.String()
}
