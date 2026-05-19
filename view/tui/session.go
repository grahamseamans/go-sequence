package tui

import (
	"fmt"
	"strings"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
	"go-sequence/model/system"
	"go-sequence/theme"
	"go-sequence/widgets"
)

// sessionViewRows is how many pattern rows the TUI session grid shows at
// once. NumPatterns is 16 in the model but only the first 8 are reachable
// from the Launchpad pads — TUI shows a wider window so the user can see
// where the cursor sits in the full pattern list.
const sessionViewRows = 16

// sessionKey handles keyboard input on the session view.
//
// hjkl moves the cursor through the (track, pattern) grid; space/enter
// queues the cell under the cursor (matching the launchpad pad behavior in
// view/launchpad/session.go::sessionPad). State lives on
// project.UI.SystemSession; the focused track is tracked there so device
// edits stay aware of where the user navigated to.
//
// Keybindings (ported from old sequencer/session.go):
//
//	h / left              cursor left  (previous track)
//	l / right             cursor right (next track)
//	j / down              cursor down  (next pattern row)
//	k / up                cursor up    (previous pattern row)
//	space / enter         queue cell under cursor
//	1-8                   handled globally — focuses that track
func sessionKey(project *model.Project, out midi.ToExternal, key string) {
	if project == nil {
		return
	}
	ses := &project.UI.SystemSession

	switch key {
	case "h", "left":
		if ses.CursorTrack > 0 {
			ses.CursorTrack--
		}
	case "l", "right":
		if ses.CursorTrack < 7 {
			ses.CursorTrack++
		}
	case "j", "down":
		if ses.CursorPattern < devices.NumPatterns-1 {
			ses.CursorPattern++
		}
	case "k", "up":
		if ses.CursorPattern > 0 {
			ses.CursorPattern--
		}
	case " ", "space", "enter", "return":
		sessionLaunchCursor(project)
	}
}

// sessionLaunchCursor queues the pattern at the session cursor on the
// cursor's track. Mirrors view/launchpad/session.go::sessionPad: writes
// QueuedPattern on the track's playback cursor; the engine swaps it in
// at the next pattern boundary.
func sessionLaunchCursor(project *model.Project) {
	ses := &project.UI.SystemSession
	track := project.Tracks[ses.CursorTrack]
	if track == nil || track.Type == model.DeviceNone {
		return
	}
	project.Playback.Cursors[ses.CursorTrack].QueuedPattern = ses.CursorPattern
}

// sessionRender returns the TUI view for the session (clip launcher) view.
//
// Layout (ported from old sequencer/session.go):
//
//	header:  "SESSION  Clip Launcher"
//	col-hdr: track names across the top
//	grid:    one row per pattern (sessionViewRows visible), one col per track.
//	         '▶' = playing, '◆' = queued, '·' = has content, ' ' = empty.
//	         Cursor cell wrapped in '[ ]'; non-cursor cells padded with ' '.
//	legend:  glyph reference line.
//	help:    key bindings.
//	pads:    Launchpad reference at the bottom.
func sessionRender(project *model.Project, th *theme.Theme) string {
	if project == nil {
		return ""
	}
	ses := &project.UI.SystemSession
	clampSession(ses)

	titleStyle := themeStyle(th, roleFG).Bold(true)
	colHdrStyle := themeStyle(th, roleMuted)
	playingStyle := themeStyle(th, roleSuccess).Bold(true)
	queuedStyle := themeStyle(th, roleWarning).Bold(true)
	hasContentStyle := themeStyle(th, roleAccent)
	emptyStyle := themeStyle(th, roleMuted)
	cursorStyle := themeStyle(th, roleCursor)

	masks := make([][]bool, 8)
	for i := 0; i < 8; i++ {
		masks[i] = sessionContentMask(project.Tracks[i])
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("SESSION  Clip Launcher"))
	b.WriteString("\n\n")

	// Column header: track names (truncated to 2 chars, padded to width 4).
	b.WriteString("        ")
	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		var lbl string
		if track != nil && track.Name != "" {
			n := track.Name
			if len(n) > 2 {
				n = n[:2]
			}
			lbl = fmt.Sprintf(" %2s ", n)
		} else {
			lbl = fmt.Sprintf(" T%d ", i+1)
		}
		b.WriteString(colHdrStyle.Render(lbl))
	}
	b.WriteString("\n")

	// Grid rows.
	rowsToShow := sessionViewRows
	if rowsToShow > devices.NumPatterns {
		rowsToShow = devices.NumPatterns
	}
	for row := ses.ViewOffset; row < ses.ViewOffset+rowsToShow && row < devices.NumPatterns; row++ {
		fmt.Fprintf(&b, "Pat %2d: ", row+1)
		for col := 0; col < 8; col++ {
			playing, queued := sessionTrackSchedule(project, col)
			hasContent := col < len(masks) && row < len(masks[col]) && masks[col][row]

			ch := " "
			cellStyle := emptyStyle
			switch {
			case playing == row:
				ch, cellStyle = "▶", playingStyle
			case queued == row && queued != playing:
				ch, cellStyle = "◆", queuedStyle
			case hasContent:
				ch, cellStyle = "·", hasContentStyle
			}

			isCursor := row == ses.CursorPattern && col == ses.CursorTrack
			if isCursor {
				b.WriteString(cursorStyle.Render("["))
				b.WriteString(cellStyle.Render(ch))
				b.WriteString(cursorStyle.Render("] "))
			} else {
				b.WriteString(" ")
				b.WriteString(cellStyle.Render(ch))
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}

	// Legend line.
	b.WriteString("\n")
	b.WriteString(playingStyle.Render("▶"))
	b.WriteString(" playing  ")
	b.WriteString(queuedStyle.Render("◆"))
	b.WriteString(" queued  ")
	b.WriteString(hasContentStyle.Render("·"))
	b.WriteString(" has content   enter launches\n")

	// Key help.
	b.WriteString("\n")
	b.WriteString(widgets.RenderKeyHelp([]widgets.KeySection{
		{Keys: []widgets.KeyBinding{
			{Key: "h / l", Desc: "move cursor left/right (tracks)"},
			{Key: "j / k", Desc: "move cursor up/down (patterns)"},
			{Key: "enter", Desc: "queue clip under cursor (space is global play/pause)"},
			{Key: "1-8", Desc: "focus device on that track"},
		}},
	}))

	// Launchpad reference.
	b.WriteString("\n\n")
	b.WriteString(sessionRenderLaunchpadHelp(project))
	b.WriteString("\n")
	return b.String()
}

// clampSession bounds the session cursor + scroll into valid ranges. We
// clamp every render rather than trusting writers, since the TUI can be
// re-entered from many different code paths (key handlers, project load,
// pad input).
func clampSession(s *system.Session) {
	if s.CursorTrack < 0 {
		s.CursorTrack = 0
	}
	if s.CursorTrack > 7 {
		s.CursorTrack = 7
	}
	if s.CursorPattern < 0 {
		s.CursorPattern = 0
	}
	if s.CursorPattern > devices.NumPatterns-1 {
		s.CursorPattern = devices.NumPatterns - 1
	}
	if s.ViewOffset < 0 {
		s.ViewOffset = 0
	}
	maxOffset := devices.NumPatterns - sessionViewRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.ViewOffset > maxOffset {
		s.ViewOffset = maxOffset
	}
	// Slide the window so the cursor stays visible.
	if s.CursorPattern < s.ViewOffset {
		s.ViewOffset = s.CursorPattern
	}
	if s.CursorPattern >= s.ViewOffset+sessionViewRows {
		s.ViewOffset = s.CursorPattern - sessionViewRows + 1
	}
}

// sessionTrackSchedule returns (playing, queued) pattern indices for the
// track via its playback cursor, or (-1, -1) when the track has no device.
func sessionTrackSchedule(project *model.Project, trackIdx int) (playing, queued int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return -1, -1
	}
	track := project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return -1, -1
	}
	c := project.Playback.Cursors[trackIdx]
	return c.CurrentPattern, c.QueuedPattern
}

func sessionDeviceLabel(kind model.DeviceKind) string {
	return model.DeviceLabel(kind)
}

// sessionRenderLaunchpadHelp renders the clip-launcher Launchpad reference.
// Each cell at (row=track, col=pattern) is colored by the device's content
// and schedule: playing > queued > has-content > empty. The right column
// shows scene-launch buttons; the top row is unused. Mirrors the old
// sequencer/session.go renderLaunchpadHelp(), but reads from the new model.
func sessionRenderLaunchpadHelp(project *model.Project) string {
	playingColor := [3]uint8{255, 255, 255}
	queuedColor := [3]uint8{255, 200, 0}
	clipColor := [3]uint8{71, 13, 121}
	emptyColor := [3]uint8{20, 4, 30}
	sceneColor := [3]uint8{148, 18, 126}
	offColor := [3]uint8{30, 30, 30}

	var grid [8][8][3]uint8
	var rightCol [8][3]uint8

	for row := 0; row < 8; row++ {
		track := project.Tracks[row]
		playing, queued := sessionTrackSchedule(project, row)
		mask := sessionContentMask(track)
		for col := 0; col < 8; col++ {
			switch {
			case col == playing:
				grid[row][col] = playingColor
			case col == queued && queued != playing:
				grid[row][col] = queuedColor
			case col < len(mask) && mask[col]:
				grid[row][col] = clipColor
			default:
				grid[row][col] = emptyColor
			}
		}
		rightCol[row] = sceneColor
	}

	topRow := make([][3]uint8, 8)
	for i := range topRow {
		topRow[i] = offColor
	}

	out := widgets.RenderPadRow(topRow) + "\n"
	out += widgets.RenderPadGrid(grid, &rightCol) + "\n\n"
	out += widgets.RenderLegendItem(playingColor, "Playing", "currently playing pattern") + "\n"
	out += widgets.RenderLegendItem(queuedColor, "Queued", "queued for next pattern boundary") + "\n"
	out += widgets.RenderLegendItem(clipColor, "Clip", "pattern has content") + "\n"
	out += widgets.RenderLegendItem(emptyColor, "Empty", "pattern has no content") + "\n"
	out += widgets.RenderLegendItem(sceneColor, "Scene", "launch scenes (right column)")
	return out
}

// sessionContentMask returns a per-pattern bool slice indicating which
// patterns on the track have any user-entered content. Length == NumPatterns,
// or empty when the track has no device.
func sessionContentMask(track *model.Track) []bool {
	if track == nil {
		return nil
	}
	mask := make([]bool, devices.NumPatterns)
	for i := 0; i < devices.NumPatterns; i++ {
		mask[i] = model.PatternHasContent(track, i)
	}
	return mask
}
