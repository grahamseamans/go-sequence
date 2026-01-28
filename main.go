package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"go-sequence/config"
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/sequencer"
	"go-sequence/theme"
	"go-sequence/tui"
)

func main() {
	fmt.Println("starting...")

	// Enable debug logging
	debug.Enable()
	defer debug.Disable()

	// Load config
	fmt.Println("loading config...")
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Warning: could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Load theme
	fmt.Println("loading theme...")
	palette := theme.MustLoadGPL("palettes/plasma.gpl")
	th := theme.New(palette)

	// Create sequencer manager
	fmt.Println("creating sequencer...")
	manager := sequencer.NewManager()

	// Assign devices to slots
	manager.SetDevice(0, manager.CreateDrumDevice(0))
	manager.SetDevice(1, manager.CreateDrumDevice(1))
	manager.SetDevice(2, manager.CreatePianoDevice(2))
	// Remaining slots get EmptyDevice
	for i := 3; i < 8; i++ {
		manager.SetDevice(i, manager.CreateEmptyDevice(i))
	}

	// Create session (clip launcher)
	session := sequencer.NewSessionDevice(manager)
	manager.SetSession(session)

	// Create settings device
	settings := sequencer.NewSettingsDevice(manager)
	manager.SetSettings(settings)

	// Create save device
	saveDevice := sequencer.NewSaveDevice(manager)
	manager.SetSave(saveDevice)

	// Start all runtime goroutines
	manager.StartRuntime()

	// Create MIDI device manager
	fmt.Println("initializing MIDI...")
	deviceMgr := midi.NewDeviceManager()

	// Try to connect to controller once on startup (with timeout, won't hang)
	fmt.Println("connecting controller...")
	fmt.Println("")
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

	// Wire MIDI input if available
	if noteInput := deviceMgr.GetNoteInput(); noteInput != nil {
		manager.SetMIDIInput(noteInput)
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
