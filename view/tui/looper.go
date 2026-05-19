package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
	"go-sequence/theme"
	"go-sequence/widgets"
)

// looperKey handles a keyboard event on the looper view.
func looperKey(track *model.Track, project *model.Project, out midi.ToExternal, key string) {
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return
	}

	switch key {
	case "r", "R":
		d.Recording = !d.Recording
		if d.Recording {
			pat.Events = nil
		}
	case "c", "C":
		pat.Events = nil
		d.Recording = false
	case "<", ",":
		if d.EditingPatternIdx > 0 {
			d.EditingPatternIdx--
		}
	case ">", ".":
		if d.EditingPatternIdx < devices.NumPatterns-1 {
			d.EditingPatternIdx++
		}
	case "[":
		stepTicks := int64(devices.PPQ / 4)
		if pat.Length > stepTicks {
			pat.Length -= stepTicks
		}
	case "]":
		stepTicks := int64(devices.PPQ / 4)
		if pat.Length < 32*stepTicks {
			pat.Length += stepTicks
		}
	case " ", "space":
		// Space is consumed by global play/pause in tui.go, but handle
		// it here for completeness if routing changes.
	}
}

// looperRender returns the TUI view for the looper device.
func looperRender(track *model.Track, project *model.Project, th *theme.Theme) string {
	d, ok := track.Device.(*devices.Looper)
	if !ok || d == nil {
		return ""
	}
	pat := d.Patterns[d.EditingPatternIdx]
	if pat == nil {
		return ""
	}

	headerStyle := themeStyle(th, roleFG).Bold(true)
	recStyle := themeStyle(th, roleWarning).Bold(true)
	mutedStyle := themeStyle(th, roleMuted)

	stepsPerPattern := pat.Length / (devices.PPQ / 4)
	if stepsPerPattern < 1 {
		stepsPerPattern = 1
	}

	trackIdx := project.TrackIndex(track)
	playInfo := ""
	playingIdx := 0
	if trackIdx >= 0 {
		playingIdx = project.Playback.Cursors[trackIdx].CurrentPattern
	}
	if d.EditingPatternIdx != playingIdx {
		playInfo = fmt.Sprintf(" (playing:%d)", playingIdx+1)
	}

	var b strings.Builder
	header := fmt.Sprintf("LOOPER  Pattern %d%s  %d events  Len %d steps",
		d.EditingPatternIdx+1, playInfo, len(pat.Events), stepsPerPattern)
	b.WriteString(headerStyle.Render(header))
	if d.Recording {
		b.WriteString("  ")
		b.WriteString(recStyle.Render("REC"))
	}
	b.WriteString("\n\n")

	if d.Recording {
		b.WriteString(recStyle.Render(" ● RECORDING — playing MIDI in will be captured"))
		b.WriteString("\n\n")
	} else if len(pat.Events) == 0 {
		b.WriteString(mutedStyle.Render("   No recorded events. Press r to arm recording."))
		b.WriteString("\n\n")
	} else {
		b.WriteString(fmt.Sprintf("   %d MIDI events recorded, looping every %d steps\n", len(pat.Events), stepsPerPattern))
		b.WriteString("\n")
	}

	b.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: "r", Desc: "toggle record (wipes previous recording)"},
			{Key: "c", Desc: "clear recorded events"},
			{Key: "< / >", Desc: "previous/next pattern"},
			{Key: "[ / ]", Desc: "shorter/longer loop"},
		}},
	}))

	b.WriteString("\n\n")
	b.WriteString(looperRenderLaunchpadHelp())
	b.WriteString("\n")

	return b.String()
}

func looperRenderLaunchpadHelp() string {
	commandsColor := [3]uint8{253, 157, 110}
	dimColor := [3]uint8{40, 40, 40}

	var grid [8][8][3]uint8
	var rightCol [8][3]uint8
	topRow := make([][3]uint8, 8)

	for i := 0; i < 8; i++ {
		topRow[i] = dimColor
		rightCol[i] = dimColor
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			grid[row][col] = dimColor
		}
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, &rightCol) + "\n\n"
	out += widgets.RenderLegendItem(commandsColor, "Commands", "record/clear from keyboard for now")
	return out
}
