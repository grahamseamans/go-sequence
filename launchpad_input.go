package main

import (
	"strings"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type LaunchpadInput struct {
	inPort drivers.In
	stop   func()
}

func NewLaunchpadInput() *LaunchpadInput {
	return &LaunchpadInput{}
}

// noteToStep converts Launchpad note number to step index (0-15)
// Returns -1 if not a step pad
func noteToStep(note uint8) int {
	// Bottom row: notes 11-18 = steps 0-7
	if note >= 11 && note <= 18 {
		return int(note - 11)
	}
	// Second row: notes 21-28 = steps 8-15
	if note >= 21 && note <= 28 {
		return int(note - 21 + 8)
	}
	return -1
}

func (li *LaunchpadInput) Open(portIndex int, onPadPress func(step int)) error {
	ins := midi.GetInPorts()
	if portIndex < 0 || portIndex >= len(ins) {
		return nil
	}
	li.inPort = ins[portIndex]

	stop, err := midi.ListenTo(li.inPort, func(msg midi.Message, timestampms int32) {
		var channel, note, velocity uint8
		if msg.GetNoteOn(&channel, &note, &velocity) {
			if velocity > 0 { // Note on (velocity 0 is note off)
				step := noteToStep(note)
				if step >= 0 {
					onPadPress(step)
				}
			}
		}
	})
	if err != nil {
		return err
	}
	li.stop = stop
	return nil
}

func (li *LaunchpadInput) Close() {
	if li.stop != nil {
		li.stop()
	}
}

// FindLaunchpadInputPort finds the Launchpad MIDI input port index
func FindLaunchpadInputPort() int {
	ins := midi.GetInPorts()
	for i, port := range ins {
		name := port.String()
		// Look for "MIDI Out" - that's what sends pad presses to us
		if containsIgnoreCase(name, "launchpad") && containsIgnoreCase(name, "midi") {
			return i
		}
	}
	return -1
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
