package midi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-sequence/config"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // Register MIDI driver
)

// DeviceEvent is emitted when controllers connect/disconnect
type DeviceEvent struct {
	Type       DeviceEventType
	Controller Controller
	ID         string
	Error      error
}

type DeviceEventType int

const (
	DeviceConnected DeviceEventType = iota
	DeviceDisconnected
	DeviceError
)

// DeviceManager handles MIDI controller connections (no polling - user-initiated only)
type DeviceManager struct {
	controller Controller // Launchpad (special control surface)
	noteInput  Controller // MIDI keyboard for recording
	mu         sync.RWMutex
	timeout    time.Duration
}

// NewDeviceManager creates a new device manager
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{
		timeout: 5 * time.Second,
	}
}

// GetController returns the currently connected controller (or nil)
func (dm *DeviceManager) GetController() Controller {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.controller
}

// GetNoteInput returns the currently connected note input (or nil)
func (dm *DeviceManager) GetNoteInput() Controller {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.noteInput
}

// ConnectNoteInput connects to a MIDI keyboard for recording
func (dm *DeviceManager) ConnectNoteInput(portName string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Close existing if any
	if dm.noteInput != nil {
		dm.noteInput.Close()
		dm.noteInput = nil
	}

	if portName == "" {
		return nil // Just disconnect
	}

	// Find the port
	inPorts := gomidi.GetInPorts()
	var inPort drivers.In
	for _, p := range inPorts {
		if p.String() == portName {
			inPort = p
			break
		}
	}

	if inPort == nil {
		return fmt.Errorf("MIDI input port not found: %s", portName)
	}

	// Create keyboard controller for note input
	ctrl, err := NewKeyboardController(portName, inPort)
	if err != nil {
		return err
	}

	dm.noteInput = ctrl
	return nil
}

// DisconnectNoteInput closes the note input
func (dm *DeviceManager) DisconnectNoteInput() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.noteInput != nil {
		dm.noteInput.Close()
		dm.noteInput = nil
	}
}

// Connect attempts to connect to a controller (with timeout)
// This is called on startup and when user requests a rescan
func (dm *DeviceManager) Connect(cfg *config.Config) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Close existing controller if any
	if dm.controller != nil {
		dm.controller.Close()
		dm.controller = nil
	}

	// Timeout wrapper for all CoreMIDI operations
	ctx, cancel := context.WithTimeout(context.Background(), dm.timeout)
	defer cancel()

	resultCh := make(chan error, 1)
	var newController Controller

	go func() {
		ctrl, err := dm.tryConnect(cfg)
		if err == nil {
			newController = ctrl
		}
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		if err == nil {
			dm.controller = newController
		}
		return err
	case <-ctx.Done():
		return fmt.Errorf("MIDI timeout - system may be busy. Try: sudo killall coreaudiod midiserver")
	}
}

// tryConnect attempts to find and connect to a controller
func (dm *DeviceManager) tryConnect(cfg *config.Config) (Controller, error) {
	// Get ports (single enumeration, not a loop)
	inPorts := gomidi.GetInPorts()
	outPorts := gomidi.GetOutPorts()

	if len(inPorts) == 0 {
		return nil, fmt.Errorf("no MIDI input ports found")
	}

	// Try auto-connect controllers from config
	for _, ctrlCfg := range cfg.AutoConnectControllers() {
		inPort := findPortByName(inPorts, ctrlCfg.PortName)
		if inPort == nil {
			continue
		}

		// Find matching output port
		outName := strings.Replace(ctrlCfg.PortName, "In", "Out", 1)
		outPort := findPortByName(outPorts, outName)

		ctrl, err := dm.createController(ctrlCfg.Type, inPort, outPort)
		if err == nil {
			return ctrl, nil
		}
	}

	// Fallback: try to find any Launchpad
	for _, inPort := range inPorts {
		name := strings.ToLower(inPort.String())
		if strings.Contains(name, "launchpad") && strings.Contains(name, "midi") {
			// Find matching output
			outPort := findPortByName(outPorts, inPort.String())

			// Detect type from name
			ctrlType := detectControllerType(inPort.String())

			ctrl, err := dm.createController(ctrlType, inPort, outPort)
			if err == nil {
				return ctrl, nil
			}
		}
	}

	return nil, fmt.Errorf("no compatible controller found")
}

// createController creates the appropriate controller based on type
func (dm *DeviceManager) createController(ctrlType config.ControllerType, inPort drivers.In, outPort drivers.Out) (Controller, error) {
	switch ctrlType {
	case config.ControllerLaunchpadX:
		return NewLaunchpadController(inPort.String(), inPort, outPort)
	case config.ControllerLaunchpadMini:
		return NewLaunchpadController(inPort.String(), inPort, outPort) // Same for now
	case config.ControllerLaunchpadPro:
		return NewLaunchpadController(inPort.String(), inPort, outPort) // Same for now
	case config.ControllerKeyboard:
		return NewKeyboardController(inPort.String(), inPort)
	default:
		return nil, fmt.Errorf("unknown controller type: %s", ctrlType)
	}
}

// Disconnect closes the current controller
func (dm *DeviceManager) Disconnect() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.controller != nil {
		dm.controller.Close()
		dm.controller = nil
	}
}

// ScanPorts returns available MIDI ports (with timeout)
func (dm *DeviceManager) ScanPorts() ([]string, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dm.timeout)
	defer cancel()

	type result struct {
		inNames  []string
		outNames []string
		err      error
	}

	ch := make(chan result, 1)
	go func() {
		inPorts := gomidi.GetInPorts()
		outPorts := gomidi.GetOutPorts()

		var inNames, outNames []string
		for _, p := range inPorts {
			inNames = append(inNames, p.String())
		}
		for _, p := range outPorts {
			outNames = append(outNames, p.String())
		}
		ch <- result{inNames: inNames, outNames: outNames}
	}()

	select {
	case r := <-ch:
		return r.inNames, r.outNames, r.err
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("MIDI scan timeout")
	}
}

// Helper functions

func findPortByName[T interface{ String() string }](ports []T, name string) T {
	nameLower := strings.ToLower(name)
	for _, p := range ports {
		if strings.Contains(strings.ToLower(p.String()), nameLower) {
			return p
		}
	}
	var zero T
	return zero
}

func detectControllerType(portName string) config.ControllerType {
	name := strings.ToLower(portName)
	switch {
	case strings.Contains(name, "launchpad x"):
		return config.ControllerLaunchpadX
	case strings.Contains(name, "launchpad mini"):
		return config.ControllerLaunchpadMini
	case strings.Contains(name, "launchpad pro"):
		return config.ControllerLaunchpadPro
	default:
		return config.ControllerLaunchpadX // Default assumption
	}
}
