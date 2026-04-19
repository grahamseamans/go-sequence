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

	// Control surface — Mock for now (no Launchpad auto-discovery yet).
	// Swap in launchpad.NewLaunchpad(...) once the port-discovery story
	// from the old midi.DeviceManager lands in the new stack.
	surf := launchpad.NewMock()
	defer surf.Close()

	// SaveOps wires the project-browser view to controller/project.go file I/O.
	saveOps := controller.NewControllerSaveOps()

	// Background routines — each runs in its own goroutine.
	go controller.RunPlayback(ctx, project, ports)
	go controller.RunPatternCompiler(ctx, project)
	go launchpad.RunRoutine(ctx, project, ports, saveOps, surf)
	go keyboard.RunRoutine(ctx, project, ports, ports)

	// TUI blocks until the user quits (q / ctrl+c) or ctx is cancelled.
	if err := tui.Run(ctx, project, ports, saveOps); err != nil {
		fmt.Printf("tui: %v\n", err)
		os.Exit(1)
	}
}
