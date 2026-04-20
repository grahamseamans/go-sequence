package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/theme"
)

// sessionKey handles keyboard input on the session view.
//
// Intentional no-op: the session view is pad-driven (see
// view/launchpad/session.go). The text view is a read-only status display.
// Dispatched for symmetry.
func sessionKey(project *model.Project, out midi.ToExternal, key string) {}

// sessionRender returns the TUI view for the session (clip launcher) view.
//
// For each of the 8 tracks, a one-liner reports the device kind, the
// currently-playing pattern (1-indexed), and the queued pattern (1-indexed,
// "-" when nothing is queued).
func sessionRender(project *model.Project, th *theme.Theme) string {
	if project == nil {
		return ""
	}

	titleStyle := themeStyle(th, roleFG).Bold(true)
	queuedStyle := themeStyle(th, roleAccent)
	mutedStyle := themeStyle(th, roleMuted)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Session (clip launcher)"))
	b.WriteString("\n\n")

	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  Track %d  -", i+1)))
			b.WriteString("\n")
			continue
		}
		playing, queued, ok := sessionSchedule(track)
		if !ok {
			fmt.Fprintf(&b, "  Track %d  [%s]\n", i+1, sessionDeviceLabel(track.Type))
			continue
		}
		queuedStr := "-"
		if queued >= 0 {
			queuedStr = fmt.Sprintf("%d", queued+1)
		}
		fmt.Fprintf(&b, "  Track %d  [%-10s]  Playing=%d  Queued=",
			i+1, sessionDeviceLabel(track.Type), playing+1)
		if queued >= 0 {
			b.WriteString(queuedStyle.Render(queuedStr))
		} else {
			b.WriteString(mutedStyle.Render(queuedStr))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString("  (tap a pad in row=track, col=pattern to queue)\n")
	return b.String()
}

// sessionSchedule extracts (playing, queued) for the track's device, or
// reports ok=false if the track has no device.
func sessionSchedule(track *model.Track) (playing, queued int, ok bool) {
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			return 0, -1, false
		}
		return track.Drum.Schedule.Playing, track.Drum.Schedule.Queued, true
	case model.DevicePiano:
		if track.Piano == nil {
			return 0, -1, false
		}
		return track.Piano.Schedule.Playing, track.Piano.Schedule.Queued, true
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			return 0, -1, false
		}
		return track.Metropolix.Schedule.Playing, track.Metropolix.Schedule.Queued, true
	}
	return 0, -1, false
}

func sessionDeviceLabel(kind model.DeviceKind) string {
	switch kind {
	case model.DeviceDrum:
		return "Drum"
	case model.DevicePiano:
		return "Piano"
	case model.DeviceMetropolix:
		return "Metropolix"
	}
	return "empty"
}
