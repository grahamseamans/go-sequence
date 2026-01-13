package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PadConfig describes how a single pad should appear
type PadConfig struct {
	Color   [3]uint8
	Tooltip string // used for legend grouping
}

// LaunchpadLayout is provided by devices to configure the display
type LaunchpadLayout struct {
	TopRow   [8]PadConfig
	Grid     [8][8]PadConfig
	RightCol [8]PadConfig
}

// Zone describes a region of the Launchpad for the legend
type Zone struct {
	Name  string
	Color [3]uint8
	Desc  string
}

// RenderLaunchpad returns a colored ASCII representation of the Launchpad
func RenderLaunchpad(layout LaunchpadLayout) string {
	var lines []string

	// Top row
	var top strings.Builder
	for col := 0; col < 8; col++ {
		top.WriteString(renderPad(layout.TopRow[col].Color))
		top.WriteString(" ")
	}
	lines = append(lines, top.String())

	// 8x8 grid + right column (row 7 at top, row 0 at bottom)
	for row := 7; row >= 0; row-- {
		var line strings.Builder
		for col := 0; col < 8; col++ {
			line.WriteString(renderPad(layout.Grid[row][col].Color))
			line.WriteString(" ")
		}
		// Right column
		line.WriteString(renderPad(layout.RightCol[row].Color))
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

// RenderLegend returns a color-coordinated legend for the zones
func RenderLegend(zones []Zone) string {
	var lines []string
	for _, z := range zones {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(rgbToHex(z.Color)))
		pad := style.Render("■")
		lines = append(lines, fmt.Sprintf("  %s %s - %s", pad, z.Name, z.Desc))
	}
	return strings.Join(lines, "\n")
}

// RenderKeyHelp formats key bindings in a friendly way
func RenderKeyHelp(sections []KeySection) string {
	var lines []string
	for _, sec := range sections {
		if sec.Title != "" {
			lines = append(lines, sec.Title)
		}
		for _, k := range sec.Keys {
			lines = append(lines, fmt.Sprintf("  %-12s %s", k.Key, k.Desc))
		}
	}
	return strings.Join(lines, "\n")
}

// KeySection groups related key bindings
type KeySection struct {
	Title string
	Keys  []KeyBinding
}

// KeyBinding is a single key and its description
type KeyBinding struct {
	Key  string
	Desc string
}

func renderPad(color [3]uint8) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(rgbToHex(color)))
	return style.Render("■")
}

func rgbToHex(c [3]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
}
