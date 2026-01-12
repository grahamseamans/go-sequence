package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"go-sequence/config"
	"go-sequence/midi"
	"go-sequence/sequencer"
	"go-sequence/theme"
	"go-sequence/tui"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Warning: could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

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

	// Create MIDI device manager
	deviceMgr := midi.NewDeviceManager()

	// Try to connect to controller once on startup (with timeout, won't hang)
	fmt.Println("go-sequence")
	if err := deviceMgr.Connect(cfg); err != nil {
		fmt.Printf("No controller: %v\n", err)
		fmt.Println("Press 'r' in the app to scan for devices")
	} else {
		ctrl := deviceMgr.GetController()
		if ctrl != nil {
			fmt.Printf("Connected: %s\n", ctrl.ID())
			manager.SetController(ctrl)
		}
	}
	fmt.Println("")

	// Create and run TUI
	m := tui.NewModel(manager, deviceMgr, cfg, th)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Cleanup
	deviceMgr.Disconnect()
}
