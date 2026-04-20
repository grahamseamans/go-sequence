package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// settingsKey handles keyboard input on the settings view.
//
// Edits apply to Tracks[focusedTrack]. Caller (HandleKey in tui.go) holds
// track.Lock and calls MarkTrackDirty afterwards.
//
// Keybindings:
//
//	t / T        cycle device type forward / backward
//	             (None → Drum → Piano → Metropolix → None)
//	[ / ]        decrement / increment MIDI channel (0..15)
//	k            cycle drum kit (only when Type == DeviceDrum)
//	m            toggle mute
//	o            toggle solo  (NOT 's' — that's reserved for the global
//	                           project-browser save command)
//
// Port-name selection isn't ported: it requires a picker UI that isn't
// trivial text (see TODO in SHORTCUTS.md).
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
	case "o":
		track.Solo = !track.Solo
	}
}

// settingsRender returns the TUI view for the settings page.
//
// Layout: one line per track. The currently-focused track (from
// project.UI.LastFocusedTrack) is marked with '>'. Each line shows index,
// mute/solo state, name, device type, port (or "-" when unset), MIDI
// channel (1..16 to the user, 0..15 internally), and for drum tracks, the
// kit name.
func settingsRender(project *model.Project) string {
	if project == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("Settings\n\n")

	focused := project.UI.LastFocusedTrack
	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		mark := "  "
		if i == focused {
			mark = "> "
		}
		if track == nil {
			fmt.Fprintf(&b, "%s%d  -\n", mark, i+1)
			continue
		}
		flags := settingsFlagsLabel(track)
		port := track.PortName
		if port == "" {
			port = "-"
		}
		kit := ""
		if track.Type == model.DeviceDrum {
			kit = fmt.Sprintf("  Kit=%s", settingsKitLabel(track.Kit))
		}
		fmt.Fprintf(&b, "%s%d  %s  Name=%-10s  Type=%-10s  Port=%s  Ch=%d%s\n",
			mark, i+1, flags, track.Name, sessionDeviceLabel(track.Type), port, track.Channel+1, kit)
	}

	b.WriteString("\n")
	b.WriteString("  t/T cycle type   k cycle kit   [ ] channel -/+\n")
	b.WriteString("  m mute           o solo        1-8 focus track\n")
	return b.String()
}

// settingsFlagsLabel renders mute/solo state as a short 5-char indicator,
// e.g. "[MS]", "[M-]", "[-S]", "[--]".
func settingsFlagsLabel(track *model.Track) string {
	m := "-"
	if track.Muted {
		m = "M"
	}
	s := "-"
	if track.Solo {
		s = "S"
	}
	return fmt.Sprintf("[%s%s]", m, s)
}

// settingsKitLabel returns the user-visible kit name for a track's Kit
// string, falling back to the raw string if the kit is unknown.
func settingsKitLabel(kit string) string {
	if kit == "" {
		return "-"
	}
	return kit
}

// --- mutation helpers (package-private; caller holds track.mu) ---

// settingsSetDeviceType changes a track's device Type and reshapes its device
// pointers so exactly one is non-nil (matching the new Type). Validate() is
// called on the new device so patterns get default lengths.
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
