package tui

import (
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// settingsKey handles keyboard input on the settings view.
//
// Ported from controller/devices/settings.HandleKey.
//
//	t / T        cycle device type (Drum → Piano → Metropolix → None → ...)
//	[ / ]        decrement / increment MIDI channel (0..15)
//	k            cycle drum kit (only when track.Type == DeviceDrum)
//	m            toggle mute
//	s            toggle solo
func settingsKey(project *model.Project, out midi.ToExternal, focusedTrack int, key string) {
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
		settingsSetDeviceType(track, settingsNextDeviceKind(track.Type))
	case "T":
		settingsSetDeviceType(track, settingsPrevDeviceKind(track.Type))
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
			track.Kit = settingsNextKit(track.Kit)
		}
	case "m":
		track.Muted = !track.Muted
	case "s":
		track.Solo = !track.Solo
	}
}

// settingsRender returns the TUI view for the settings device. Stub.
func settingsRender(project *model.Project) string { return "" }

// --- helpers (package-private; caller holds track.mu) ---

// settingsSetDeviceType changes a track's device Type and reshapes its device
// pointers to match.
func settingsSetDeviceType(track *model.Track, kind model.DeviceKind) {
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

var settingsDeviceKindCycle = []model.DeviceKind{
	model.DeviceNone,
	model.DeviceDrum,
	model.DevicePiano,
	model.DeviceMetropolix,
}

func settingsDeviceKindIndex(kind model.DeviceKind) int {
	for i, k := range settingsDeviceKindCycle {
		if k == kind {
			return i
		}
	}
	return 0
}

func settingsNextDeviceKind(kind model.DeviceKind) model.DeviceKind {
	i := settingsDeviceKindIndex(kind)
	return settingsDeviceKindCycle[(i+1)%len(settingsDeviceKindCycle)]
}

func settingsPrevDeviceKind(kind model.DeviceKind) model.DeviceKind {
	i := settingsDeviceKindIndex(kind)
	return settingsDeviceKindCycle[(i-1+len(settingsDeviceKindCycle))%len(settingsDeviceKindCycle)]
}

// settingsNextKit returns the next kit name in devices.KitNames(), wrapping.
// An unknown current kit maps to the first kit in the list.
func settingsNextKit(current string) string {
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
