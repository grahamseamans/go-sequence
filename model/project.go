package model

import (
	"encoding/json"
	"fmt"
	"sync"

	"go-sequence/model/devices"
)

// DeviceKind tags which device (if any) a track uses. Empty string means no
// device is assigned.
type DeviceKind string

const (
	DeviceNone   DeviceKind = ""
	DeviceDrum   DeviceKind = "Drum"
	DeviceLooper DeviceKind = "Looper"
)

// DeviceLabels maps DeviceKind to a human-readable label.
var DeviceLabels = map[DeviceKind]string{
	DeviceNone:   "empty",
	DeviceDrum:   "Drum",
	DeviceLooper: "Looper",
}

// DeviceCycle is the ordered list of device types for cycling through in
// settings views (t / T shortcuts).
var DeviceCycle = []DeviceKind{
	DeviceNone,
	DeviceDrum,
	DeviceLooper,
}

// DeviceLabel returns a human-readable label for a DeviceKind.
func DeviceLabel(kind DeviceKind) string {
	if l, ok := DeviceLabels[kind]; ok {
		return l
	}
	return "empty"
}

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

// Track is one of the project's eight tracks. Device carries the per-device
// state as a concrete type (*devices.Drum, *devices.Looper) and is nil when
// Type == DeviceNone. mu guards the track's spec fields against concurrent
// reads (Compiler, View) and writes (input router).
//
// Mute / solo are deliberately NOT on the track — those belong in the
// downstream mixer the user is sending MIDI to, not in the sequencer.
// To silence a track from the sequencer, stop the pattern instead.
type Track struct {
	Name     string       `json:"name"`
	Channel  uint8        `json:"channel"`
	PortName string       `json:"portName,omitempty"`
	Type     DeviceKind   `json:"type"`
	Kit      string       `json:"kit,omitempty"`
	mu       sync.RWMutex `json:"-"`
	Device   any          `json:"-"`
}

// trackJSON mirrors Track for JSON serialization. It carries a flat field
// per device type so the on-disk format is the same as the old per-field
// approach.
type trackJSON struct {
	Name     string           `json:"name"`
	Channel  uint8            `json:"channel"`
	PortName string           `json:"portName,omitempty"`
	Type     DeviceKind       `json:"type"`
	Kit      string           `json:"kit,omitempty"`
	Drum     *devices.Drum    `json:"drum,omitempty"`
	Looper   *devices.Looper  `json:"looper,omitempty"`
}

func (t *Track) MarshalJSON() ([]byte, error) {
	j := trackJSON{
		Name: t.Name, Channel: t.Channel,
		PortName: t.PortName, Type: t.Type, Kit: t.Kit,
	}
	switch d := t.Device.(type) {
	case *devices.Drum:
		j.Drum = d
	case *devices.Looper:
		j.Looper = d
	}
	return json.Marshal(j)
}

func (t *Track) UnmarshalJSON(data []byte) error {
	var j trackJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	t.Name = j.Name
	t.Channel = j.Channel
	t.PortName = j.PortName
	t.Type = j.Type
	t.Kit = j.Kit
	switch t.Type {
	case DeviceDrum:
		t.Device = j.Drum
	case DeviceLooper:
		t.Device = j.Looper
	}
	return nil
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
// tracks (all DeviceNone), a ready CompileState channel, and per-track
// playback cursors initialized to "pattern 0, nothing queued".
func New() *Project {
	p := &Project{
		Tempo:   120,
		Compile: NewCompileState(),
		UI: UIState{
			KeyboardRouteChanged: make(chan struct{}, 1),
		},
	}
	for i := 0; i < 8; i++ {
		p.Tracks[i] = &Track{
			Name:    fmt.Sprintf("Track %d", i+1),
			Channel: uint8(i),
			Type:    DeviceNone,
			Kit:     "",
		}
		p.Playback.Cursors[i] = Cursor{QueuedPattern: -1}
	}
	return p
}

// TrackIndex returns the index of t within p.Tracks, or -1 when t isn't
// found. Cheap (8 pointer compares); used by view + controller code
// that has a *Track and needs to reach into project.Playback.Cursors[i].
func (p *Project) TrackIndex(t *Track) int {
	for i, tt := range p.Tracks {
		if tt == t {
			return i
		}
	}
	return -1
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
// enforces the "device matches type" invariant implied by Track.Type.
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
			t.Device = nil
		case DeviceDrum:
			if _, ok := t.Device.(*devices.Drum); !ok {
				t.Device = &devices.Drum{}
			}
			t.Device.(*devices.Drum).Validate()
		case DeviceLooper:
			if _, ok := t.Device.(*devices.Looper); !ok {
				t.Device = &devices.Looper{}
			}
			t.Device.(*devices.Looper).Validate()
		default:
			t.Type = DeviceNone
			t.Device = nil
		}
	}
}

// PatternHasContent returns true if the named pattern on the track has any
// user-entered content. Returns false when the track has no device.
func PatternHasContent(track *Track, patternIdx int) bool {
	if track == nil || patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return false
	}
	switch d := track.Device.(type) {
	case *devices.Drum:
		if d == nil || d.Patterns[patternIdx] == nil {
			return false
		}
		for _, lane := range d.Patterns[patternIdx].Notes {
			for _, step := range lane.Steps {
				if step.Active {
					return true
				}
			}
		}
		return false
	case *devices.Looper:
		if d == nil || d.Patterns[patternIdx] == nil {
			return false
		}
		return len(d.Patterns[patternIdx].Events) > 0
	}
	return false
}
