package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PadConfig describes how a single pad should appear and behave
type PadConfig struct {
	Color   [3]uint8 // RGB color
	Tooltip string   // what to show on hover
}

// LaunchpadLayout is provided by devices to configure the help widget
type LaunchpadLayout struct {
	TopRow   [8]PadConfig    // top function buttons
	Grid     [8][8]PadConfig // 8x8 main grid
	RightCol [8]PadConfig    // scene buttons (right column)
}

// LaunchpadHelp shows a static help diagram of the Launchpad layout
type LaunchpadHelp struct {
	layout LaunchpadLayout

	// Current hover position (-1 = none)
	HoverRow, HoverCol int
}

func NewLaunchpadHelp() *LaunchpadHelp {
	return &LaunchpadHelp{
		HoverRow: -1,
		HoverCol: -1,
	}
}

// SetLayout applies a device-provided layout configuration
func (l *LaunchpadHelp) SetLayout(layout LaunchpadLayout) {
	l.layout = layout
}

// HitTest checks if x,y (relative to widget top-left) hits a pad
// Returns: hit bool, tooltip string
// Also updates HoverRow/HoverCol for visual feedback
func (l *LaunchpadHelp) HitTest(x, y int) (bool, string) {
	// Layout: each pad is "■ " = 2 chars wide, 1 char tall
	// Top row is y=0, grid rows are y=1 to y=8 (row 7 at y=1, row 0 at y=8)
	// Right column is at x=16 (after 8 pads * 2 chars)

	col := x / 2 // 2 chars per pad

	if y == 0 {
		// Top row
		if col >= 0 && col < 8 {
			l.HoverRow = -1 // special: top row
			l.HoverCol = col
			return true, l.layout.TopRow[col].Tooltip
		}
		l.HoverRow, l.HoverCol = -2, -2 // nothing
		return false, ""
	}

	if y >= 1 && y <= 8 {
		gridRow := 7 - (y - 1) // y=1 is row 7, y=8 is row 0

		if col == 8 {
			// Right column (scene buttons)
			l.HoverRow = gridRow
			l.HoverCol = 8 // special: right column
			return true, l.layout.RightCol[gridRow].Tooltip
		}

		if col >= 0 && col < 8 {
			// Main grid
			l.HoverRow = gridRow
			l.HoverCol = col
			return true, l.layout.Grid[gridRow][col].Tooltip
		}
	}

	l.HoverRow, l.HoverCol = -2, -2 // nothing
	return false, ""
}

// ClearHover resets hover state
func (l *LaunchpadHelp) ClearHover() {
	l.HoverRow, l.HoverCol = -2, -2
}

// Height returns the height of the widget in lines
func (l *LaunchpadHelp) Height() int {
	return 9 // 1 top row + 8 grid rows
}

// Width returns the width of the widget in chars
func (l *LaunchpadHelp) Width() int {
	return 18 // 8 pads * 2 + 1 right col * 2 = 18
}

// View renders the help diagram
func (l *LaunchpadHelp) View() string {
	var lines []string

	// Top row
	var top strings.Builder
	for col := 0; col < 8; col++ {
		isHover := l.HoverRow == -1 && l.HoverCol == col
		color := l.layout.TopRow[col].Color
		top.WriteString(l.renderPad(color, isHover))
		top.WriteString(" ")
	}
	lines = append(lines, top.String())

	// 8x8 grid + right column (row 7 at top, row 0 at bottom)
	for row := 7; row >= 0; row-- {
		var line strings.Builder
		for col := 0; col < 8; col++ {
			isHover := l.HoverRow == row && l.HoverCol == col
			color := l.layout.Grid[row][col].Color
			line.WriteString(l.renderPad(color, isHover))
			line.WriteString(" ")
		}
		// Right column
		isHover := l.HoverRow == row && l.HoverCol == 8
		line.WriteString(l.renderPad(l.layout.RightCol[row].Color, isHover))
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

func (l *LaunchpadHelp) renderPad(color [3]uint8, isHover bool) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(rgbToHex(color)))
	if isHover {
		// Highlight: white background, inverted
		style = style.Background(lipgloss.Color("#ffffff")).Foreground(lipgloss.Color("#000000"))
		return style.Render("●")
	}
	return style.Render("■")
}

func rgbToHex(c [3]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
}
