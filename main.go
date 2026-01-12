package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"go-sequence/midi"
	"go-sequence/sequencer"
	"go-sequence/theme"
	"go-sequence/tui"
)

func main() {
	// Load theme
	palette := theme.MustLoadGPL("palettes/plasma.gpl")
	th := theme.New(palette)

	// Create sequencer manager
	manager := sequencer.NewManager()

	// Create devices
	drum1 := sequencer.NewDrumDevice()
	drum2 := sequencer.NewDrumDevice()

	manager.AddDevice(drum1, 1) // external channel 1
	manager.AddDevice(drum2, 2) // external channel 2

	// Create session (clip launcher)
	session := sequencer.NewSessionDevice(manager.Devices())
	manager.SetSession(session)

	// Create MIDI device manager (handles hot-plug)
	deviceMgr := midi.NewDeviceManager()

	// Start device manager in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go deviceMgr.Run(ctx)

	fmt.Println("go-sequence")
	fmt.Println("Connect MIDI devices any time - they'll be detected automatically")
	fmt.Println("")

	// Create and run TUI
	m := tui.NewModel(manager, deviceMgr, th)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
