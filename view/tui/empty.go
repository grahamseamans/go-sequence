package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/theme"
	"go-sequence/widgets"
)

// emptyKey handles keystrokes for a no-device track. Always a no-op —
// there's nothing to edit until the user assigns a device from the Settings
// page.
func emptyKey(project *model.Project, out midi.ToExternal, key string) {}

// emptyRender is shown when the focused track has no device assigned. Hints
// at the Settings keybind so the user can configure the track, then renders
// an all-dim Launchpad reference at the bottom for visual symmetry with the
// other devices.
func emptyRender(project *model.Project, th *theme.Theme) string {
	headerStyle := themeStyle(th, roleFG).Bold(true)
	mutedStyle := themeStyle(th, roleMuted)

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("TRACK %d  (empty)", project.UI.Focus.Track+1)))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("No device assigned to this track."))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Press , to open settings and assign a device."))
	b.WriteString("\n\n")
	b.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: ",", Desc: "open settings to assign device"},
			{Key: "tab", Desc: "back to session"},
			{Key: "1-8", Desc: "switch to another track"},
		}},
	}))
	b.WriteString("\n\n")
	b.WriteString(emptyRenderLaunchpadHelp())
	b.WriteString("\n")
	return b.String()
}

// emptyRenderLaunchpadHelp renders the Launchpad reference for an empty
// track — a fully dim 9x9 grid with a single legend entry. Mirrors the
// old sequencer/empty.go renderLaunchpadHelp().
func emptyRenderLaunchpadHelp() string {
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
	out += widgets.RenderLegendItem(dimColor, "Empty", "no device assigned")
	return out
}
