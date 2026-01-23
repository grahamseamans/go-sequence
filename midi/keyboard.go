package midi

import (
	"fmt"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// KeyboardController handles a standard MIDI keyboard
type KeyboardController struct {
	id       string
	inPort   drivers.In
	stopFunc func()

	padChan  chan PadEvent
	noteChan chan NoteEvent
}

// NewKeyboardController creates a keyboard controller (input only)
func NewKeyboardController(id string, inPort drivers.In) (*KeyboardController, error) {
	kb := &KeyboardController{
		id:       id,
		inPort:   inPort,
		padChan:  make(chan PadEvent, 32),
		noteChan: make(chan NoteEvent, 32),
	}

	// Open input
	if inPort != nil {
		stop, err := gomidi.ListenTo(inPort, func(msg gomidi.Message, timestampms int32) {
			var channel, note, velocity uint8
			if msg.GetNoteOn(&channel, &note, &velocity) && velocity > 0 {
				select {
				case kb.noteChan <- NoteEvent{Note: note, Velocity: velocity, Channel: channel}:
				default:
				}
			}
		})
		if err != nil {
			return nil, fmt.Errorf("open input: %w", err)
		}
		kb.stopFunc = stop
	}

	return kb, nil
}

func (kb *KeyboardController) ID() string {
	return kb.id
}

func (kb *KeyboardController) Type() ControllerType {
	return ControllerKeyboard
}

func (kb *KeyboardController) PadEvents() <-chan PadEvent {
	return kb.padChan // Keyboards don't have pads
}

func (kb *KeyboardController) NoteEvents() <-chan NoteEvent {
	return kb.noteChan
}

// SetLEDRGB is a no-op for keyboards (no visual feedback)
func (kb *KeyboardController) SetLEDRGB(row, col int, rgb [3]uint8, channel uint8) error {
	return nil
}

// SetLEDBatch is a no-op for keyboards
func (kb *KeyboardController) SetLEDBatch(updates []LEDUpdate) error {
	return nil
}

func (kb *KeyboardController) Close() error {
	if kb.stopFunc != nil {
		kb.stopFunc()
	}
	close(kb.padChan)
	close(kb.noteChan)
	return nil
}
