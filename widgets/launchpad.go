package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderPad renders a single colored pad
func RenderPad(color [3]uint8) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(rgbToHex(color)))
	return style.Render("■")
}

// RenderPadRow renders a row of colored pads with spacing
func RenderPadRow(colors [][3]uint8) string {
	var out strings.Builder
	for i, c := range colors {
		if i > 0 {
			out.WriteString(" ")
		}
		out.WriteString(RenderPad(c))
	}
	return out.String()
}

// RenderPadGrid renders an 8x8 grid of pads (row 0 at bottom, row 7 at top)
// Optional rightCol adds a 9th column (scene buttons)
func RenderPadGrid(grid [8][8][3]uint8, rightCol *[8][3]uint8) string {
	var lines []string
	for row := 7; row >= 0; row-- {
		var line strings.Builder
		for col := 0; col < 8; col++ {
			line.WriteString(RenderPad(grid[row][col]))
			line.WriteString(" ")
		}
		if rightCol != nil {
			line.WriteString(RenderPad(rightCol[row]))
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// RenderLegendItem renders a single legend item: "■ Name - description"
func RenderLegendItem(color [3]uint8, name, desc string) string {
	return fmt.Sprintf("  %s %s - %s", RenderPad(color), name, desc)
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

func rgbToHex(c [3]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
}
