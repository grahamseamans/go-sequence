package theme

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Palette *Palette
	Symbols Symbols
}

type Symbols struct {
	// Launchpad help widget
	Solid rune // ■ active/has function
	Empty rune // □ inactive/no function

	// Grid states (no cursor)
	StepEmpty    rune // · inactive step
	StepActive   rune // ● has hit
	StepPlayhead rune // ▶ current playing
	StepBeyond   rune // - past track length

	// Grid states (with cursor)
	CursorEmpty    rune // ○ cursor on empty
	CursorActive   rune // ◉ cursor on active
	CursorPlayhead rune // ▷ cursor on playhead
	CursorBeyond   rune // □ cursor beyond length
}

func New(palette *Palette) *Theme {
	return &Theme{
		Palette: palette,
		Symbols: Symbols{
			Solid: '■',
			Empty: '□',

			StepEmpty:    '·',
			StepActive:   '●',
			StepPlayhead: '▶',
			StepBeyond:   '-',

			CursorEmpty:    '○',
			CursorActive:   '◉',
			CursorPlayhead: '▷',
			CursorBeyond:   '□',
		},
	}
}

// Color roles mapped to palette positions (0-1)
const (
	RoleBG      = 0.0  // deep purple
	RoleSurface = 0.1  // dark purple
	RoleMuted   = 0.2  // purple-magenta
	RoleFG      = 0.4  // pink-purple (readable)
	RoleAccent  = 0.5  // vivid magenta
	RoleCursor  = 0.6  // rose pink
	RoleActive  = 0.7  // soft red
	RoleWarning = 0.8  // orange
	RoleSuccess = 1.0  // bright yellow
)

// Style helpers

func (t *Theme) BG() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleBG))
}

func (t *Theme) FG() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleFG))
}

func (t *Theme) Accent() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleAccent))
}

func (t *Theme) Muted() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleMuted))
}

func (t *Theme) Active() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleActive))
}

func (t *Theme) Cursor() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleCursor))
}

func (t *Theme) Warning() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleWarning))
}

func (t *Theme) Success() lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(RoleSuccess))
}

// Color returns lipgloss color for any normalized value 0-1
func (t *Theme) Color(norm float64) lipgloss.Color {
	return rgbToLipgloss(t.Palette.Lookup(norm))
}

// RGB returns raw RGB for any normalized value (for Launchpad)
func (t *Theme) RGB(norm float64) RGB {
	return t.Palette.Lookup(norm)
}

func rgbToLipgloss(c RGB) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]))
}
