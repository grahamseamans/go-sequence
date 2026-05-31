package midi

import (
	"fmt"
	"sync"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"

	"go-sequence/debug"
)

// subscribeKey uniquely identifies a MIDI input subscription by port name
// and channel.
type subscribeKey struct {
	portName string
	channel  uint8
}

// subscribeState holds the stop function and event channel for one
// subscription.
type subscribeState struct {
	stop func()
	ch   chan Event
}

// outputState holds an open MIDI output port. send is the gomidi closure
// used to write messages; port is retained so the port can actually be
// closed — gomidi's SendTo opens the port but never closes it, so dropping
// the send closure alone would leak the OS port until process exit.
type outputState struct {
	port drivers.Out
	send func(gomidi.Message) error
}

// Ports manages OS MIDI port I/O. Implements ToExternal, FromExternal, and
// OutputLifecycle. Outputs are opened lazily via EnsureOutput and closed
// with CloseAllOutputs. Inputs are opened via Subscribe and closed via
// Close.
//
// rtmidi ports are not goroutine-safe, so Send holds mu across the actual
// write. This serializes it against CloseAllOutputs/Close (which call
// port.Close) — without it, a Pause on the TUI goroutine could close a port
// mid-Send on the playback goroutine. Sends are infrequent and cheap, so the
// lock is not a throughput concern.
type Ports struct {
	mu      sync.Mutex
	outputs map[string]outputState
	inputs  map[subscribeKey]subscribeState
}

func NewPorts() (*Ports, error) {
	return &Ports{
		outputs: make(map[string]outputState),
		inputs:  make(map[subscribeKey]subscribeState),
	}, nil
}

// Send sends ev to the named output port on the given channel. Returns an
// error when the port hasn't been opened via EnsureOutput.
func (p *Ports) Send(portName string, channel uint8, ev Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.outputs[portName]
	if !ok {
		return fmt.Errorf("output port not open: %s", portName)
	}

	var msg gomidi.Message
	switch ev.Type {
	case NoteOn:
		msg = gomidi.NoteOn(channel, ev.Note, ev.Velocity)
	case NoteOff:
		msg = gomidi.NoteOff(channel, ev.Note)
	case CC:
		msg = gomidi.ControlChange(channel, ev.Note, ev.Velocity)
	case PitchBend:
		msg = gomidi.Pitchbend(channel, ev.BendValue)
	case Trigger:
		msg = gomidi.NoteOn(channel, ev.Note, ev.Velocity)
	default:
		return fmt.Errorf("unknown event type: %d", ev.Type)
	}

	return state.send(msg)
}

// Subscribe opens the named MIDI input port and returns a channel carrying
// only events on the given channel. If an identical subscription already
// exists, the existing channel is returned. Subscribe is idempotent.
func (p *Ports) Subscribe(portName string, channel uint8) (<-chan Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := subscribeKey{portName: portName, channel: channel}
	if existing, ok := p.inputs[key]; ok {
		return existing.ch, nil
	}

	inPorts := gomidi.GetInPorts()
	var inPort drivers.In
	for _, port := range inPorts {
		if port.String() == portName {
			inPort = port
			break
		}
	}
	if inPort == nil {
		return nil, fmt.Errorf("input port not found: %s", portName)
	}

	ch := make(chan Event, 64)
	stop, err := gomidi.ListenTo(inPort, func(msg gomidi.Message, timestampMs int32) {
		var c, note, vel uint8
		switch {
		case msg.GetNoteOn(&c, &note, &vel):
			if c == channel {
				select {
				case ch <- Event{Type: NoteOn, Channel: c, Note: note, Velocity: vel}:
				default:
				}
			}
		case msg.GetNoteOff(&c, &note, &vel):
			if c == channel {
				select {
				case ch <- Event{Type: NoteOff, Channel: c, Note: note, Velocity: vel}:
				default:
				}
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", portName, err)
	}

	p.inputs[key] = subscribeState{stop: stop, ch: ch}
	return ch, nil
}

// EnsureOutput opens the named MIDI output port and caches the sender. If
// the port is already open this is a no-op. Returns nil on success or if the
// port is already open; returns an error when the port isn't found or can't
// be opened.
func (p *Ports) EnsureOutput(portName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.outputs[portName]; ok {
		return nil
	}

	outPorts := gomidi.GetOutPorts()
	var outPort drivers.Out
	for _, port := range outPorts {
		if port.String() == portName {
			outPort = port
			break
		}
	}
	if outPort == nil {
		return fmt.Errorf("output port not found: %s", portName)
	}

	send, err := gomidi.SendTo(outPort)
	if err != nil {
		return fmt.Errorf("open output %s: %w", portName, err)
	}

	p.outputs[portName] = outputState{port: outPort, send: send}
	return nil
}

// CloseAllOutputs closes every open MIDI output port. Safe to call
// repeatedly. Ports that need reopening must call EnsureOutput again.
func (p *Ports) CloseAllOutputs() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeOutputsLocked()
}

// closeOutputsLocked closes every open output port and clears the map. The
// caller must hold p.mu. A failing Close is logged rather than swallowed —
// the port reference is dropped regardless so the next EnsureOutput reopens
// cleanly.
func (p *Ports) closeOutputsLocked() {
	for name, state := range p.outputs {
		if err := state.port.Close(); err != nil {
			debug.Log("midi", "close output %q: %v", name, err)
		}
	}
	p.outputs = make(map[string]outputState)
}

// Close shuts down all outputs and subscriptions. Implements io.Closer.
func (p *Ports) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closeOutputsLocked()

	for key, state := range p.inputs {
		state.stop()
		close(state.ch)
		delete(p.inputs, key)
	}

	return nil
}

// OutputPortNames returns the OS-level MIDI output port names visible to
// gomidi right now. Used by the settings picker to populate the per-track
// output list. Re-queries on every call so newly-attached USB devices show
// up without a restart.
func OutputPortNames() []string {
	ports := gomidi.GetOutPorts()
	names := make([]string, 0, len(ports))
	for _, p := range ports {
		names = append(names, p.String())
	}
	return names
}

// InputPortNames returns the OS-level MIDI input port names visible to
// gomidi right now. Used by the settings page MIDI device list.
func InputPortNames() []string {
	ports := gomidi.GetInPorts()
	names := make([]string, 0, len(ports))
	for _, p := range ports {
		names = append(names, p.String())
	}
	return names
}
