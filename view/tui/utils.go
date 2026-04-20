package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"go-sequence/theme"
)

// This file holds cross-domain TUI helpers. Per-domain helpers stay inside
// their own files (drum.go, piano.go, metropolix.go, session.go, settings.go,
// project.go, empty.go); the Bubbletea Model + top-level dispatcher is in
// tui.go.
//
// Theme plumbing:
//
// Every per-domain render function accepts a *theme.Theme (nil-safe via
// `themeSymbols(th)` / `themeColor*(th)` below). Styling uses the theme's
// existing role methods (th.Accent, th.Muted, ...) rather than introducing a
// parallel index-based layer — the theme package already exposes roles as
// normalized positions along the palette (see theme/theme.go).

// lipColor converts a palette RGB triple into a lipgloss.Color hex string.
// Low-level primitive; prefer the role-based helpers below for styling.
func lipColor(rgb theme.RGB) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2]))
}

// Fallback symbol set used when th or th.Palette is nil. Mirrors the
// defaults in theme.New so domains render sensibly without a theme.
var fallbackSymbols = theme.Symbols{
	Solid: '■', Empty: '□',
	StepEmpty: '·', StepActive: '●', StepPlayhead: '▶', StepBeyond: '-',
	CursorEmpty: '○', CursorActive: '◉', CursorPlayhead: '▷', CursorBeyond: '□',
}

// themeSymbols returns the theme's symbol set, or a safe fallback when the
// theme is unavailable. Never returns a zero-valued Symbols.
func themeSymbols(th *theme.Theme) theme.Symbols {
	if th == nil {
		return fallbackSymbols
	}
	return th.Symbols
}

// themeStyle returns a lipgloss style using the given role method, or an
// unstyled (no-op) style when the theme or its palette is unavailable. This
// keeps render code branch-free: `style := themeStyle(th, (*theme.Theme).FG)`.
type themeRoleFn func(*theme.Theme) lipgloss.Color

func themeStyle(th *theme.Theme, role themeRoleFn) lipgloss.Style {
	if th == nil || th.Palette == nil || len(th.Palette.Colors) == 0 {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(role(th))
}

// Role accessors wrapped as themeRoleFn values so call sites read as
// `themeStyle(th, roleAccent)` rather than method expressions.
var (
	roleBG      themeRoleFn = (*theme.Theme).BG
	roleFG      themeRoleFn = (*theme.Theme).FG
	roleMuted   themeRoleFn = (*theme.Theme).Muted
	roleAccent  themeRoleFn = (*theme.Theme).Accent
	roleCursor  themeRoleFn = (*theme.Theme).Cursor
	roleActive  themeRoleFn = (*theme.Theme).Active
	roleWarning themeRoleFn = (*theme.Theme).Warning
	roleSuccess themeRoleFn = (*theme.Theme).Success
)
