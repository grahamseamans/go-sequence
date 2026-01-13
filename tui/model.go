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
)

type Model struct {
	Manager    *sequencer.Manager
	DeviceMgr  *midi.DeviceManager
	Config     *config.Config
	Theme      *theme.Theme
	quitting   bool
	controller midi.Controller
	statusMsg  string
}

type UpdateMsg struct{}

type RescanResultMsg struct {
	controller  midi.Controller
	err         error
	midiInputs  []string
	midiOutputs []string
}

func NewModel(manager *sequencer.Manager, deviceMgr *midi.DeviceManager, cfg *config.Config, th *theme.Theme) Model {
	controller := deviceMgr.GetController()
	m := Model{
		Manager:    manager,
		DeviceMgr:  deviceMgr,
		Config:     cfg,
		Theme:      th,
		controller: controller,
	}
	return m
}

func ListenForUpdates(manager *sequencer.Manager) tea.Cmd {
	return func() tea.Msg {
		<-manager.UpdateChan
		return UpdateMsg{}
	}
}

func RescanDevices(deviceMgr *midi.DeviceManager, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		// Get port lists first
		inputs, outputs, _ := deviceMgr.ScanPorts()

		err := deviceMgr.Connect(cfg)
		if err != nil {
			return RescanResultMsg{err: err, midiInputs: inputs, midiOutputs: outputs}
		}
		return RescanResultMsg{controller: deviceMgr.GetController(), midiInputs: inputs, midiOutputs: outputs}
	}
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, ListenForUpdates(m.Manager))

	if m.controller != nil {
		cmds = append(cmds, m.listenForPads())
	}

	return tea.Batch(cmds...)
}

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
		case "Q", "ctrl+c":
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
			// Only allow rescan from settings device
			if _, ok := m.Manager.GetFocused().(*sequencer.SettingsDevice); ok {
				m.statusMsg = "Scanning..."
				return m, RescanDevices(m.DeviceMgr, m.Config)
			}
			m.Manager.HandleKey(msg.String())

		case "0":
			m.Manager.FocusSession()

		case ",":
			m.Manager.FocusSettings()

		case "1", "2", "3", "4", "5", "6", "7", "8":
			idx := int(msg.String()[0] - '1')
			m.Manager.FocusDevice(idx)

		default:
			m.Manager.HandleKey(msg.String())
		}

	case UpdateMsg:
		return m, ListenForUpdates(m.Manager)

	case RescanResultMsg:
		// Update settings with port info
		if settings := m.Manager.GetSettings(); settings != nil {
			settings.SetMIDIPorts(msg.midiInputs, msg.midiOutputs)
		}

		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("No device: %v", msg.err)
			m.controller = nil
			m.Manager.SetController(nil)
		} else if msg.controller != nil {
			m.statusMsg = fmt.Sprintf("Connected: %s", msg.controller.ID())
			m.controller = msg.controller
			m.Manager.SetController(msg.controller)
			return m, m.listenForPads()
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	step, playing, tempo := m.Manager.GetState()

	// Styles
	titleStyle := lipgloss.NewStyle().Foreground(m.Theme.Accent()).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(m.Theme.Muted())
	borderStyle := lipgloss.NewStyle().Foreground(m.Theme.Muted())

	// Transport status
	playState := "STOP"
	if playing {
		playState = "PLAY"
	}

	ctrlStatus := "no controller"
	if m.controller != nil {
		ctrlStatus = "Launchpad X"
	}

	// Header block
	title := titleStyle.Render("go-sequence")
	status := fmt.Sprintf("  %s  %3d bpm  step %02d  [%s]", playState, tempo, step+1, ctrlStatus)
	controls := dimStyle.Render("p:play  +/-:tempo  0:session  1-8:device  ,:settings  r:rescan  q:quit")
	border := borderStyle.Render("════════════════════════════════════════════════════════════════")

	// Device view (includes grid, key help, and launchpad)
	deviceView := m.Manager.View()

	// Build output
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(title)
	out.WriteString(status)
	if m.statusMsg != "" {
		out.WriteString("  ")
		out.WriteString(dimStyle.Render(m.statusMsg))
	}
	out.WriteString("\n")
	out.WriteString(controls)
	out.WriteString("\n")
	out.WriteString(border)
	out.WriteString("\n\n")
	out.WriteString(deviceView)

	return out.String()
}
