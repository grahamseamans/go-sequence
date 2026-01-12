package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ports := ListMIDIPorts()
	if len(ports) == 0 {
		fmt.Println("No MIDI output ports found")
		os.Exit(1)
	}

	fmt.Println("Available MIDI output ports:")
	for i, port := range ports {
		fmt.Printf("  %d: %s\n", i, port)
	}

	// Auto-detect Launchpad for LED feedback - need "MIDI In" port, not "DAW In"
	lpPortIndex := -1
	for i, port := range ports {
		lower := strings.ToLower(port)
		if strings.Contains(lower, "launchpad") && strings.Contains(lower, "midi") {
			lpPortIndex = i
			break
		}
	}

	// Get synth output port selection
	var synthPortIndex int = -1
	fmt.Println("\n(Enter to skip synth output, or pick a port for MIDI notes)")
	fmt.Print("Select synth output port: ")
	var input string
	fmt.Scanln(&input)
	if input != "" {
		var err error
		synthPortIndex, err = strconv.Atoi(input)
		if err != nil || synthPortIndex < 0 || synthPortIndex >= len(ports) {
			fmt.Println("Invalid port selection, skipping synth output")
			synthPortIndex = -1
		}
	}

	// Create sequencer
	seq := NewSequencer()
	if synthPortIndex >= 0 {
		if err := seq.OpenPort(synthPortIndex); err != nil {
			fmt.Printf("Failed to open MIDI port: %v\n", err)
		} else {
			fmt.Printf("Synth output: %s\n", ports[synthPortIndex])
		}
	} else {
		fmt.Println("No synth output (Launchpad LED only mode)")
	}
	defer seq.Close()

	// Create Launchpad for LED feedback
	lp := NewLaunchpad()
	if lpPortIndex >= 0 {
		if err := lp.Open(lpPortIndex); err != nil {
			fmt.Printf("Failed to open Launchpad: %v\n", err)
		} else {
			fmt.Printf("Launchpad LED: %s\n", ports[lpPortIndex])
		}
	}
	defer lp.Close()

	// Create Launchpad input (for pad presses)
	lpInput := NewLaunchpadInput()
	lpInputPortIndex := FindLaunchpadInputPort()

	fmt.Println("\nStarting sequencer...")
	fmt.Println("Put Launchpad in Programmer Mode: hold Session + bottom Scene button")
	fmt.Println("")

	// Create and run TUI
	m := model{
		seq:    seq,
		lp:     lp,
		cursor: 0,
	}

	// Initial Launchpad update
	steps, playhead, playing, _ := seq.GetState()
	lp.UpdateSequence(steps, playhead, playing)

	p := tea.NewProgram(m, tea.WithAltScreen())

	// Set up Launchpad input - pad presses toggle steps
	if lpInputPortIndex >= 0 {
		err := lpInput.Open(lpInputPortIndex, func(step int) {
			// Send pad press to bubbletea as a message
			p.Send(padPressMsg(step))
		})
		if err != nil {
			fmt.Printf("Failed to open Launchpad input: %v\n", err)
		}
	}
	defer lpInput.Close()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
