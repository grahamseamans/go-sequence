package settings

import (
	"go-sequence/view/surface"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Package settings is the project-level track/MIDI configuration editor. It
// mutates per-track fields: Type, PortName, Channel, Kit, Muted, Solo.
//
// Settings owns no state of its own — all edits land on project.Tracks[i].
//
// Signature diverges from the canonical project-level device shape: every
// handler takes an explicit `focusedTrack int`. Settings needs to know
// which track is being edited, and per DESIGN §4.8 focus lives on
// model.UIState (not persisted Model data), so the input router passes
// project.UI.LastFocusedTrack down to Settings' handlers.

// HandlePad handles pad input on the settings view.
//
// The old sequencer/settings.go used the leftmost pad column as a track
// selector; with an explicit focusedTrack argument that becomes the input
// router's job. Other pads are no-ops for now.
func HandlePad(project *model.Project, out midi.ToExternal, focusedTrack, row, col int, down bool) {
	// Intentional no-op. Track focus is driven by keyboard shortcuts in
	// controller.HandleKey, not by pad presses inside Settings.
}

// HandleKey handles keyboard input on the settings view.
//
// Edits are applied inline (no popup state — the old PopupState was UI
// chrome that doesn't survive the move to devices/). The keybindings below
// cycle through the discrete options directly:
//
//	t / T        cycle device type (Drum → Piano → Metropolix → None → ...)
//	[ / ]        decrement / increment MIDI channel (0..15)
//	k            cycle drum kit (only when track.Type == DeviceDrum)
//	m            toggle mute
//	s            toggle solo
//
// Port name selection is intentionally dropped for now: the old UI cached
// the list of available MIDI output ports on the device, which doesn't
// belong in the pure-compiler shape. Port-name selection will come back
// through the input router (which holds the live midi.Ports reference and
// can enumerate outputs).
func HandleKey(project *model.Project, out midi.ToExternal, focusedTrack int, key string) {
	if project == nil {
		return
	}
	if focusedTrack < 0 || focusedTrack >= 8 {
		return
	}
	track := project.Tracks[focusedTrack]
	if track == nil {
		return
	}

	switch key {
	case "t":
		setDeviceType(track, nextDeviceKind(track.Type))
	case "T":
		setDeviceType(track, prevDeviceKind(track.Type))
	case "[":
		if track.Channel > 0 {
			track.Channel--
		}
	case "]":
		if track.Channel < 15 {
			track.Channel++
		}
	case "k":
		if track.Type == model.DeviceDrum {
			track.Kit = nextKit(track.Kit)
		}
	case "m":
		track.Muted = !track.Muted
	case "s":
		track.Solo = !track.Solo
	}
}

// Render is a stub; step 5 will port the TUI view from sequencer/settings.go.
// TODO(step5): port rendering from sequencer/settings.go when view/ rewires.
func Render(project *model.Project) string { return "" }

// RenderLEDs is a stub; step 5 will port the LED layout from sequencer/settings.go.
// TODO(step5): port rendering from sequencer/settings.go when view/ rewires.
func RenderLEDs(project *model.Project) []surface.LED { return nil }

// --- helpers (package-private; caller holds track.mu) ---

// setDeviceType changes a track's device Type and reshapes its device-
// pointer fields to match. Old device data is discarded — we don't try to
// preserve it across a type change. Validate() would do the same thing if
// it ran, so replicate the logic inline rather than invoking Validate on
// the whole project.
func setDeviceType(track *model.Track, kind model.DeviceKind) {
	if track.Type == kind {
		// Already there; still nil-check and allocate in case the pointer
		// got out of sync somehow.
	}
	track.Type = kind
	switch kind {
	case model.DeviceNone:
		track.Drum = nil
		track.Piano = nil
		track.Metropolix = nil
	case model.DeviceDrum:
		track.Piano = nil
		track.Metropolix = nil
		if track.Drum == nil {
			track.Drum = &devices.Drum{}
		}
		track.Drum.Validate()
	case model.DevicePiano:
		track.Drum = nil
		track.Metropolix = nil
		if track.Piano == nil {
			track.Piano = &devices.Piano{}
		}
		track.Piano.Validate()
	case model.DeviceMetropolix:
		track.Drum = nil
		track.Piano = nil
		if track.Metropolix == nil {
			track.Metropolix = &devices.Metropolix{}
		}
		track.Metropolix.Validate()
	}
}

// deviceKindCycle is the canonical forward/back order when cycling device
// types via keyboard.
var deviceKindCycle = []model.DeviceKind{
	model.DeviceNone,
	model.DeviceDrum,
	model.DevicePiano,
	model.DeviceMetropolix,
}

func deviceKindIndex(kind model.DeviceKind) int {
	for i, k := range deviceKindCycle {
		if k == kind {
			return i
		}
	}
	return 0
}

func nextDeviceKind(kind model.DeviceKind) model.DeviceKind {
	i := deviceKindIndex(kind)
	return deviceKindCycle[(i+1)%len(deviceKindCycle)]
}

func prevDeviceKind(kind model.DeviceKind) model.DeviceKind {
	i := deviceKindIndex(kind)
	return deviceKindCycle[(i-1+len(deviceKindCycle))%len(deviceKindCycle)]
}

// nextKit returns the next kit name in devices.KitNames(), wrapping around.
// An empty or unknown current kit maps to the first kit in the list.
func nextKit(current string) string {
	names := devices.KitNames()
	if len(names) == 0 {
		return current
	}
	for i, n := range names {
		if n == current {
			return names[(i+1)%len(names)]
		}
	}
	return names[0]
}
