package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#555"))
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#fff"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#0ff"))
)

type model struct {
	manager  *Manager
	quitting bool
}

type updateMsg struct{}
type padPressMsg struct {
	row, col int
}

func newModel(manager *Manager) model {
	return model{manager: manager}
}

func listenForUpdates(manager *Manager) tea.Cmd {
	return func() tea.Msg {
		<-manager.UpdateChan
		return updateMsg{}
	}
}

func (m model) Init() tea.Cmd {
	return listenForUpdates(m.manager)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.manager.Stop()
			return m, tea.Quit

		case "p":
			_, playing, _ := m.manager.GetState()
			if playing {
				m.manager.Stop()
			} else {
				m.manager.Play()
			}

		case "+", "=":
			_, _, tempo := m.manager.GetState()
			m.manager.SetTempo(tempo + 5)

		case "-", "_":
			_, _, tempo := m.manager.GetState()
			m.manager.SetTempo(tempo - 5)

		case "0":
			m.manager.FocusSession()

		case "1", "2", "3", "4", "5", "6", "7", "8":
			idx := int(msg.String()[0] - '1')
			m.manager.FocusDevice(idx)

		default:
			// Pass to focused device
			m.manager.HandleKey(msg.String())
		}

	case updateMsg:
		return m, listenForUpdates(m.manager)

	case padPressMsg:
		m.manager.HandlePad(msg.row, msg.col)
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	step, playing, tempo := m.manager.GetState()

	// Header
	playState := "STOP"
	if playing {
		playState = "PLAY"
	}
	header := headerStyle.Render(fmt.Sprintf("go-sequence  %s  %3dbpm  step:%02d", playState, tempo, step))

	// Device view
	deviceView := m.manager.View()

	// Help
	help := dimStyle.Render("0:session 1-8:device  h/l/j/k:nav  space:toggle  p:play  +/-:tempo  q:quit")

	return fmt.Sprintf("\n%s\n\n%s\n%s\n", header, deviceView, help)
}
