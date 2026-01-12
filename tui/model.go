package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	Theme      *theme.Theme
	lpHelp     *widgets.LaunchpadHelp
	quitting   bool
	mouseX     int
	mouseY     int
	tooltip    string
	bounds     *layoutBounds
	controller midi.Controller // current controller (may be nil)
}

type UpdateMsg struct{}

type DeviceEventMsg midi.DeviceEvent

func NewModel(manager *sequencer.Manager, deviceMgr *midi.DeviceManager, th *theme.Theme) Model {
	lp := widgets.NewLaunchpadHelp()
	if focused := manager.GetFocused(); focused != nil {
		lp.SetLayout(focused.HelpLayout())
	}
	return Model{
		Manager:   manager,
		DeviceMgr: deviceMgr,
		Theme:     th,
		lpHelp:    lp,
		bounds:    &layoutBounds{},
	}
}

func ListenForUpdates(manager *sequencer.Manager) tea.Cmd {
	return func() tea.Msg {
		<-manager.UpdateChan
		return UpdateMsg{}
	}
}

func ListenForDevices(deviceMgr *midi.DeviceManager) tea.Cmd {
	return func() tea.Msg {
		event := <-deviceMgr.Events()
		return DeviceEventMsg(event)
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		ListenForUpdates(m.Manager),
		ListenForDevices(m.DeviceMgr),
	)
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

	case DeviceEventMsg:
		event := midi.DeviceEvent(msg)
		if event.Type == midi.DeviceConnected {
			m.controller = event.Controller
			m.Manager.SetController(event.Controller)

			// Listen for pad events from the controller
			go func() {
				for pad := range event.Controller.PadEvents() {
					m.Manager.HandlePad(pad.Row, pad.Col)
				}
			}()
		} else if event.Type == midi.DeviceDisconnected {
			if m.controller != nil && m.controller.ID() == event.ID {
				m.controller = nil
				m.Manager.SetController(nil)
			}
		}
		return m, ListenForDevices(m.DeviceMgr)
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

	deviceStatus := ""
	if m.controller != nil {
		deviceStatus = " LP:X"
	}

	header := headerStyle.Render(fmt.Sprintf("go-sequence  %s  %3dbpm  step:%02d%s", playState, tempo, step, deviceStatus))

	// Device view
	deviceView := m.Manager.View()

	// Launchpad help
	lpView := m.lpHelp.View()

	// Help line
	help := dimStyle.Render("0:session 1-8:device  hjkl:nav  space:toggle  p:play  +/-:tempo  q:quit")

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
