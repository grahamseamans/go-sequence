package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Orca-inspired minimal palette
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#555"))
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#fff"))
	cursorStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#444"))
	playheadStyle = lipgloss.NewStyle().Reverse(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
)

type model struct {
	seq      *Sequencer
	lp       *Launchpad
	cursor   int
	quitting bool
}

type playheadMsg int
type padPressMsg int

func listenForPlayhead(seq *Sequencer) tea.Cmd {
	return func() tea.Msg {
		playhead := <-seq.PlayheadChan
		return playheadMsg(playhead)
	}
}

func (m model) Init() tea.Cmd {
	return listenForPlayhead(m.seq)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "h", "left":
			if m.cursor > 0 {
				m.cursor--
			}

		case "l", "right":
			if m.cursor < 15 {
				m.cursor++
			}

		case "j", "down":
			m.seq.AdjustNote(m.cursor, -1)

		case "k", "up":
			m.seq.AdjustNote(m.cursor, 1)

		case "J": // Shift+J = octave down
			m.seq.AdjustNote(m.cursor, -12)

		case "K": // Shift+K = octave up
			m.seq.AdjustNote(m.cursor, 12)

		case " ":
			m.seq.ToggleStep(m.cursor)
			// Update Launchpad
			steps, playhead, playing, _ := m.seq.GetState()
			m.lp.UpdateSequence(steps, playhead, playing)

		case "p":
			_, _, playing, _ := m.seq.GetState()
			if playing {
				m.seq.Stop()
			} else {
				m.seq.Play()
			}
			// Update Launchpad
			steps, playhead, playing, _ := m.seq.GetState()
			m.lp.UpdateSequence(steps, playhead, playing)

		case "+", "=":
			_, _, _, tempo := m.seq.GetState()
			m.seq.SetTempo(tempo + 5)

		case "-", "_":
			_, _, _, tempo := m.seq.GetState()
			m.seq.SetTempo(tempo - 5)
		}

	case playheadMsg:
		// Update Launchpad LEDs
		steps, playhead, playing, _ := m.seq.GetState()
		m.lp.UpdateSequence(steps, playhead, playing)
		// Continue listening for playhead updates
		return m, listenForPlayhead(m.seq)

	case padPressMsg:
		// Launchpad pad pressed - toggle that step
		step := int(msg)
		m.seq.ToggleStep(step)
		m.cursor = step // Move cursor to pressed pad
		// Update Launchpad LEDs
		steps, playhead, playing, _ := m.seq.GetState()
		m.lp.UpdateSequence(steps, playhead, playing)
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	steps, playhead, playing, tempo := m.seq.GetState()

	// Build grid - Orca style, raw characters
	var cells []string
	for i, step := range steps {
		char := "·"
		if step.Active {
			char = noteToChar(step.Note)
		}

		style := dimStyle
		if step.Active {
			style = activeStyle
		}
		if i == m.cursor {
			style = style.Inherit(cursorStyle)
		}
		if i == playhead && playing {
			style = playheadStyle
		}

		cells = append(cells, style.Render(char))
	}
	grid := strings.Join(cells, "")

	// Status line
	playState := "stop"
	if playing {
		playState = "play"
	}

	// Show note at cursor
	cursorNote := steps[m.cursor].Note
	noteStr := noteToName(cursorNote)

	status := statusStyle.Render(fmt.Sprintf("%s %3dbpm  %s", playState, tempo, noteStr))

	// Help line
	help := dimStyle.Render("h/l:move  j/k:note  J/K:octave  space:toggle  p:play  +/-:tempo  q:quit")

	return fmt.Sprintf("\n%s\n%s\n\n%s\n", grid, status, help)
}

// noteToChar converts MIDI note to a single character display
func noteToChar(note uint8) string {
	notes := []string{"C", "c", "D", "d", "E", "F", "f", "G", "g", "A", "a", "B"}
	return notes[note%12]
}

// noteToName converts MIDI note to readable name (e.g., "C4", "F#3")
func noteToName(note uint8) string {
	names := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	octave := int(note)/12 - 1
	return fmt.Sprintf("%2s%d", names[note%12], octave)
}
