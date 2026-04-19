package model

import (
	"fmt"
	"sync"

	"go-sequence/model/devices"
)

// DeviceKind tags which device (if any) a track uses. Empty string means no
// device is assigned.
type DeviceKind string

const (
	DeviceNone       DeviceKind = ""
	DeviceDrum       DeviceKind = "Drum"
	DevicePiano      DeviceKind = "Piano"
	DeviceMetropolix DeviceKind = "Metropolix"
)

// Project is the entire state of a go-sequence project. Persisted fields
// carry JSON tags; UI / Playback / Compile are runtime-only (json:"-") and
// live here per DESIGN §3 — "model owns ALL state".
type Project struct {
	Tempo    int           `json:"tempo"`
	Tracks   [8]*Track     `json:"tracks"`
	UI       UIState       `json:"-"`
	Playback PlaybackState `json:"-"`
	Compile  CompileState  `json:"-"`
}

// Track is one of the project's eight tracks. Exactly one of Drum/Piano/
// Metropolix is non-nil when Type names a device; all three are nil when
// Type == DeviceNone. mu guards the track's spec fields against concurrent
// reads (Compiler, View) and writes (input router).
type Track struct {
	Name       string              `json:"name"`
	Channel    uint8               `json:"channel"`
	PortName   string              `json:"portName,omitempty"`
	Type       DeviceKind          `json:"type"`
	Kit        string              `json:"kit,omitempty"`
	Muted      bool                `json:"muted"`
	Solo       bool                `json:"solo"`
	mu         sync.RWMutex        `json:"-"`
	Drum       *devices.Drum       `json:"drum,omitempty"`
	Piano      *devices.Piano      `json:"piano,omitempty"`
	Metropolix *devices.Metropolix `json:"metropolix,omitempty"`
}

// Lock acquires the track's write lock. Used by the input router on mutations.
func (t *Track) Lock() { t.mu.Lock() }

// Unlock releases the track's write lock.
func (t *Track) Unlock() { t.mu.Unlock() }

// RLock acquires the track's read lock. Used by Compiler and View.
func (t *Track) RLock() { t.mu.RLock() }

// RUnlock releases the track's read lock.
func (t *Track) RUnlock() { t.mu.RUnlock() }

// New constructs a fresh Project with default tempo, eight initialized
// tracks (all DeviceNone), and a ready CompileState channel.
func New() *Project {
	p := &Project{
		Tempo:   120,
		Compile: NewCompileState(),
	}
	for i := 0; i < 8; i++ {
		p.Tracks[i] = &Track{
			Name:    fmt.Sprintf("Track %d", i+1),
			Channel: uint8(i),
			Type:    DeviceNone,
			Kit:     "",
		}
	}
	return p
}

// ReplacePersisted copies the persisted fields (Tempo, Tracks) from src into
// p in place, leaving the runtime PlaybackState untouched. Used on project
// load so the rest of the system (which holds *p) keeps seeing the same
// pointer — and so vet doesn't flag a value-copy over atomic.Bool in
// PlaybackState.
func (p *Project) ReplacePersisted(src *Project) {
	if src == nil {
		return
	}
	p.Tempo = src.Tempo
	p.Tracks = src.Tracks
}

// Validate clamps every field of the project into a known-good range and
// enforces the "exactly one device pointer non-nil" invariant implied by
// Track.Type. Call after Load.
func (p *Project) Validate() {
	if p.Tempo <= 0 {
		p.Tempo = 120
	} else if p.Tempo < 20 {
		p.Tempo = 20
	} else if p.Tempo > 300 {
		p.Tempo = 300
	}

	for i, t := range p.Tracks {
		if t == nil {
			t = &Track{
				Name:    fmt.Sprintf("Track %d", i+1),
				Channel: uint8(i),
				Type:    DeviceNone,
			}
			p.Tracks[i] = t
		}
		if t.Channel > 15 {
			t.Channel = 15
		}

		switch t.Type {
		case DeviceNone:
			t.Drum = nil
			t.Piano = nil
			t.Metropolix = nil
		case DeviceDrum:
			if t.Drum == nil {
				t.Drum = &devices.Drum{}
			}
			t.Piano = nil
			t.Metropolix = nil
			t.Drum.Validate()
		case DevicePiano:
			if t.Piano == nil {
				t.Piano = &devices.Piano{}
			}
			t.Drum = nil
			t.Metropolix = nil
			t.Piano.Validate()
		case DeviceMetropolix:
			if t.Metropolix == nil {
				t.Metropolix = &devices.Metropolix{}
			}
			t.Drum = nil
			t.Piano = nil
			t.Metropolix.Validate()
		default:
			// Unknown device kind → downgrade to DeviceNone and drop data.
			t.Type = DeviceNone
			t.Drum = nil
			t.Piano = nil
			t.Metropolix = nil
		}
	}
}
