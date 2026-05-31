package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go-sequence/controller"
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/theme"
	"go-sequence/view/keyboard"
	"go-sequence/view/launchpad"
	"go-sequence/view/tui"
)

func main() {
	fmt.Println("go-sequence")
	debug.Enable()
	defer debug.Disable()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Model — fresh project with defaults. Persisted save-load lands when
	// the project browser UI is wired; for now we always start empty.
	project := model.New()
	project.Validate()

	// MIDI ports — output for playback, input for the keyboard view.
	ports, err := midi.NewPorts()
	if err != nil {
		fmt.Printf("midi: %v\n", err)
		os.Exit(1)
	}
	defer ports.Close()

	// Control surface — probe for a real Launchpad; fall back to Mock so the
	// rest of the app (TUI, playback, keyboard) still runs headless when no
	// hardware is connected.
	var surf launchpad.Surface
	if lp, err := launchpad.OpenLaunchpad(); err == nil {
		surf = lp
	} else {
		fmt.Printf("launchpad: %v — falling back to mock\n", err)
		surf = launchpad.NewMock()
	}
	defer surf.Close()

	// SaveOps wires the project-browser view to controller/project.go file I/O.
	saveOps := controller.NewControllerSaveOps()

	// Theme — load the plasma palette from disk and build the default Theme.
	// A missing file shouldn't crash the app; fall back to an empty palette
	// so the TUI still renders (uncolored) and the log tells the user why.
	palette, err := theme.LoadGPL("palettes/plasma.gpl")
	if err != nil {
		fmt.Printf("theme: %v — continuing with empty palette\n", err)
		palette = &theme.Palette{}
	}
	th := theme.New(palette)

	// Background routines — each runs in its own goroutine.
	go controller.RunPlayback(ctx, project, ports)
	go controller.RunPatternCompiler(ctx, project)
	go launchpad.RunRoutine(ctx, project, ports, saveOps, surf)
	go keyboard.RunRoutine(ctx, project, ports, ports)

	// TUI blocks until the user quits (q / ctrl+c) or ctx is cancelled.
	if err := tui.Run(ctx, project, ports, ports, saveOps, th); err != nil {
		fmt.Printf("tui: %v\n", err)
		os.Exit(1)
	}
}
