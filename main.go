package main

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"gitlab.com/gomidi/midi/v2"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

func main() {
	ports := midi.GetOutPorts()
	if len(ports) == 0 {
		fmt.Println("No MIDI output ports found")
		os.Exit(1)
	}

	fmt.Println("Available MIDI output ports:")
	for i, port := range ports {
		fmt.Printf("  %d: %s\n", i, port.String())
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

	// Create controller (Launchpad)
	controller := NewController()
	if err := controller.Open(); err != nil {
		fmt.Printf("Warning: Failed to open Launchpad: %v\n", err)
	}
	defer controller.Close()

	// Create manager
	manager := NewManager(controller)
	if synthPortIndex >= 0 {
		if err := manager.OpenMIDI(synthPortIndex); err != nil {
			fmt.Printf("Failed to open MIDI port: %v\n", err)
		} else {
			fmt.Printf("Synth output: %s\n", ports[synthPortIndex].String())
		}
	} else {
		fmt.Println("No synth output (LED only mode)")
	}

	// Create devices
	drum1 := NewDrumDevice(controller)
	drum2 := NewDrumDevice(controller)

	manager.AddDevice(drum1, 1) // external channel 1
	manager.AddDevice(drum2, 2) // external channel 2

	// Create session (clip launcher)
	session := NewSessionDevice(controller, manager.devices)
	manager.SetSession(session)

	// Wire up controller input to manager
	controller.OnPad = func(row, col int) {
		manager.HandlePad(row, col)
	}

	fmt.Println("\nStarting sequencer...")
	fmt.Println("Put Launchpad in Programmer Mode: hold Session + bottom Scene button")
	fmt.Println("")

	// Create and run TUI
	m := newModel(manager)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
