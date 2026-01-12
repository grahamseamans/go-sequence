package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ControllerType identifies the kind of controller
type ControllerType string

const (
	ControllerLaunchpadX    ControllerType = "launchpad-x"
	ControllerLaunchpadMini ControllerType = "launchpad-mini"
	ControllerLaunchpadPro  ControllerType = "launchpad-pro"
	ControllerKeyboard      ControllerType = "keyboard"
	ControllerGenericGrid   ControllerType = "generic-grid"
)

// ControllerConfig defines a saved controller configuration
type ControllerConfig struct {
	PortName     string         `json:"portName"`
	Type         ControllerType `json:"type"`
	AutoConnect  bool           `json:"autoConnect"`
	InputChannel int            `json:"inputChannel,omitempty"` // for keyboards
}

// SynthOutputConfig defines the synth MIDI output
type SynthOutputConfig struct {
	PortName string `json:"portName,omitempty"`
	Channels []int  `json:"channels,omitempty"`
}

// UIConfig stores UI preferences
type UIConfig struct {
	LastTempo         int `json:"lastTempo,omitempty"`
	LastFocusedDevice int `json:"lastFocusedDevice,omitempty"`
}

// Config is the main configuration structure
type Config struct {
	Controllers []ControllerConfig `json:"controllers,omitempty"`
	SynthOutput SynthOutputConfig  `json:"synthOutput,omitempty"`
	UI          UIConfig           `json:"ui,omitempty"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Controllers: []ControllerConfig{
			{
				PortName:    "Launchpad X LPX MIDI",
				Type:        ControllerLaunchpadX,
				AutoConnect: true,
			},
		},
		UI: UIConfig{
			LastTempo: 120,
		},
	}
}

// ConfigDir returns the config directory path
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "go-sequence"), nil
}

// ConfigPath returns the full path to config.json
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk, or returns defaults if not found
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes the config to disk
func (c *Config) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// FindController finds a controller config by port name
func (c *Config) FindController(portName string) *ControllerConfig {
	for i := range c.Controllers {
		if c.Controllers[i].PortName == portName {
			return &c.Controllers[i]
		}
	}
	return nil
}

// AddController adds or updates a controller config
func (c *Config) AddController(ctrl ControllerConfig) {
	for i := range c.Controllers {
		if c.Controllers[i].PortName == ctrl.PortName {
			c.Controllers[i] = ctrl
			return
		}
	}
	c.Controllers = append(c.Controllers, ctrl)
}

// AutoConnectControllers returns controllers with autoConnect enabled
func (c *Config) AutoConnectControllers() []ControllerConfig {
	var result []ControllerConfig
	for _, ctrl := range c.Controllers {
		if ctrl.AutoConnect {
			result = append(result, ctrl)
		}
	}
	return result
}
