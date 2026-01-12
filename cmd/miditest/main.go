package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "list":
		listPorts()
	case "detect":
		detectLaunchpad()
	case "sysex":
		testSysEx()
	case "leds":
		testLEDs()
	case "poll":
		pollDevices()
	default:
		usage()
	}
}

func usage() {
	fmt.Println("MIDI Test Scripts")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  list    - List all MIDI ports")
	fmt.Println("  detect  - Find Launchpad X")
	fmt.Println("  sysex   - Send programmer mode SysEx")
	fmt.Println("  leds    - Test LED control")
	fmt.Println("  poll    - Poll for device changes")
}

func listPorts() {
	fmt.Println("=== MIDI Input Ports ===")
	fmt.Println("(waiting up to 3 seconds...)")

	type result struct {
		ins  []drivers.In
		outs []drivers.Out
	}
	ch := make(chan result, 1)
	go func() {
		ins := midi.GetInPorts()
		outs := midi.GetOutPorts()
		ch <- result{ins: ins, outs: outs}
	}()

	select {
	case r := <-ch:
		for i, p := range r.ins {
			fmt.Printf("  %d: %s\n", i, p.String())
		}
		fmt.Println("\n=== MIDI Output Ports ===")
		for i, p := range r.outs {
			fmt.Printf("  %d: %s\n", i, p.String())
		}
	case <-time.After(3 * time.Second):
		fmt.Println("\nTIMEOUT! CoreMIDI is hung.")
		fmt.Println("Fix: sudo killall coreaudiod midiserver")
	}
}

func detectLaunchpad() {
	fmt.Println("Looking for Launchpad X...")

	ins := midi.GetInPorts()
	outs := midi.GetOutPorts()

	var inIdx, outIdx = -1, -1

	for i, p := range ins {
		name := strings.ToLower(p.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			fmt.Printf("Found input: %d: %s\n", i, p.String())
			inIdx = i
		}
	}

	for i, p := range outs {
		name := strings.ToLower(p.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			fmt.Printf("Found output: %d: %s\n", i, p.String())
			outIdx = i
		}
	}

	if inIdx >= 0 && outIdx >= 0 {
		fmt.Println("\nLaunchpad X detected!")
	} else {
		fmt.Println("\nLaunchpad X not found")
	}
}

func testSysEx() {
	fmt.Println("Sending SysEx to switch to Programmer mode...")

	outs := midi.GetOutPorts()
	var outPort drivers.Out
	for _, p := range outs {
		name := strings.ToLower(p.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			outPort = p
			break
		}
	}

	if outPort == nil {
		fmt.Println("No Launchpad found")
		return
	}

	fmt.Printf("Using output: %s\n", outPort.String())

	send, err := midi.SendTo(outPort)
	if err != nil {
		fmt.Printf("Error opening port: %v\n", err)
		return
	}

	// Switch to Programmer mode: F0 00 20 29 02 0C 00 7F F7
	fmt.Println("Sending: Programmer mode (layout 0x7F)")
	err = send(midi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x00, 0x7F}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	time.Sleep(100 * time.Millisecond)

	// Enable LED feedback: F0 00 20 29 02 0C 0A 01 01 F7
	fmt.Println("Sending: Enable LED feedback")
	err = send(midi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x0A, 0x01, 0x01}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Done! Launchpad should now be in Programmer mode")
}

func testLEDs() {
	fmt.Println("Testing LED control...")

	outs := midi.GetOutPorts()
	var outPort drivers.Out
	for _, p := range outs {
		name := strings.ToLower(p.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			outPort = p
			break
		}
	}

	if outPort == nil {
		fmt.Println("No Launchpad found")
		return
	}

	send, err := midi.SendTo(outPort)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// First ensure programmer mode
	send(midi.SysEx([]byte{0x00, 0x20, 0x29, 0x02, 0x0C, 0x00, 0x7F}))
	time.Sleep(100 * time.Millisecond)

	fmt.Println("Lighting up diagonal (green)...")

	// Light diagonal - note numbers: row*10 + col + 11
	// Row 0 = 11-18, Row 7 = 81-88
	for i := 0; i < 8; i++ {
		note := uint8((i+1)*10 + i + 1) // diagonal
		send(midi.NoteOn(0, note, 13))  // 13 = green
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("Press Enter to clear...")
	fmt.Scanln()

	// Clear all
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			note := uint8((row+1)*10 + col + 1)
			send(midi.NoteOn(0, note, 0))
		}
	}

	fmt.Println("Done!")
}

func pollDevices() {
	fmt.Println("Polling for device changes every 2 seconds...")
	fmt.Println("Connect/disconnect Launchpad to test. Ctrl+C to exit.")

	lastIn := ""
	lastOut := ""

	for {
		ins := midi.GetInPorts()
		outs := midi.GetOutPorts()

		// Build current state
		var inNames, outNames []string
		for _, p := range ins {
			inNames = append(inNames, p.String())
		}
		for _, p := range outs {
			outNames = append(outNames, p.String())
		}

		currentIn := strings.Join(inNames, ",")
		currentOut := strings.Join(outNames, ",")

		if currentIn != lastIn || currentOut != lastOut {
			fmt.Printf("\n[%s] Device change detected!\n", time.Now().Format("15:04:05"))
			fmt.Printf("  Inputs: %v\n", inNames)
			fmt.Printf("  Outputs: %v\n", outNames)

			// Check for Launchpad
			for _, name := range inNames {
				if strings.Contains(strings.ToLower(name), "launchpad") {
					fmt.Println("  -> Launchpad detected!")
				}
			}

			lastIn = currentIn
			lastOut = currentOut
		}

		time.Sleep(2 * time.Second)
	}
}
