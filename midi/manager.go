package midi

import (
	"context"
	"strings"
	"sync"
	"time"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // Register MIDI driver
)

// DeviceEvent is emitted when controllers connect/disconnect
type DeviceEvent struct {
	Type       DeviceEventType
	Controller Controller
	ID         string
}

type DeviceEventType int

const (
	DeviceConnected DeviceEventType = iota
	DeviceDisconnected
)

// DeviceManager handles hot-plug detection of MIDI controllers
type DeviceManager struct {
	controllers map[string]Controller
	mu          sync.RWMutex
	events      chan DeviceEvent
	pollRate    time.Duration
}

// NewDeviceManager creates a new device manager
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{
		controllers: make(map[string]Controller),
		events:      make(chan DeviceEvent, 16),
		pollRate:    time.Second,
	}
}

// Events returns a channel of device connect/disconnect events
func (dm *DeviceManager) Events() <-chan DeviceEvent {
	return dm.events
}

// Controllers returns a snapshot of connected controllers
func (dm *DeviceManager) Controllers() map[string]Controller {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	copy := make(map[string]Controller, len(dm.controllers))
	for k, v := range dm.controllers {
		copy[k] = v
	}
	return copy
}

// GetLaunchpad returns the first connected Launchpad (or nil)
func (dm *DeviceManager) GetLaunchpad() Controller {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	for _, c := range dm.controllers {
		if c.Type() == ControllerLaunchpad {
			return c
		}
	}
	return nil
}

// Run starts the polling loop (blocking - run in goroutine)
func (dm *DeviceManager) Run(ctx context.Context) {
	ticker := time.NewTicker(dm.pollRate)
	defer ticker.Stop()

	// Initial scan
	dm.scan()

	for {
		select {
		case <-ctx.Done():
			dm.closeAll()
			close(dm.events)
			return
		case <-ticker.C:
			dm.scan()
		}
	}
}

func (dm *DeviceManager) scan() {
	// Get current MIDI ports with timeout (CoreMIDI can hang)
	type portsResult struct {
		inPorts  []drivers.In
		outPorts []drivers.Out
		err      error
	}

	ch := make(chan portsResult, 1)
	go func() {
		inPorts := gomidi.GetInPorts()
		outPorts := gomidi.GetOutPorts()
		ch <- portsResult{inPorts: inPorts, outPorts: outPorts}
	}()

	// Wait for result or timeout
	var inPorts []drivers.In
	var outPorts []drivers.Out

	select {
	case result := <-ch:
		inPorts = result.inPorts
		outPorts = result.outPorts
	case <-time.After(3 * time.Second):
		// CoreMIDI is hung - skip this scan
		// User needs to run: sudo killall coreaudiod midiserver
		return
	}

	// Build map of what we see now
	seenIDs := make(map[string]bool)

	// Look for Launchpads
	for i, inPort := range inPorts {
		name := strings.ToLower(inPort.String())
		if isLaunchpad(name) {
			id := inPort.String()
			seenIDs[id] = true

			dm.mu.RLock()
			_, exists := dm.controllers[id]
			dm.mu.RUnlock()

			if !exists {
				// Find matching output port
				var outPort drivers.Out
				for j, op := range outPorts {
					if strings.ToLower(op.String()) == name {
						outPort = outPorts[j]
						break
					}
				}

				// Try to create controller
				lp, err := NewLaunchpadController(id, inPorts[i], outPort)
				if err != nil {
					continue
				}

				dm.mu.Lock()
				dm.controllers[id] = lp
				dm.mu.Unlock()

				dm.events <- DeviceEvent{
					Type:       DeviceConnected,
					Controller: lp,
					ID:         id,
				}
			}
		}
	}

	// TODO: Detect keyboards (non-Launchpad MIDI inputs)

	// Check for disconnects
	dm.mu.Lock()
	var toRemove []string
	for id := range dm.controllers {
		if !seenIDs[id] {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		c := dm.controllers[id]
		c.Close()
		delete(dm.controllers, id)
		dm.events <- DeviceEvent{
			Type: DeviceDisconnected,
			ID:   id,
		}
	}
	dm.mu.Unlock()
}

func (dm *DeviceManager) closeAll() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for _, c := range dm.controllers {
		c.Close()
	}
	dm.controllers = make(map[string]Controller)
}

func isLaunchpad(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "launchpad") && strings.Contains(name, "midi")
}
