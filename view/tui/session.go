package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
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
func sessionRender(project *model.Project) string {
	if project == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("Session (clip launcher)\n\n")

	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil {
			fmt.Fprintf(&b, "  Track %d  -\n", i+1)
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
		fmt.Fprintf(&b, "  Track %d  [%-10s]  Playing=%d  Queued=%s\n",
			i+1, sessionDeviceLabel(track.Type), playing+1, queuedStr)
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
