package system

// Session holds UI state for the session-view (clip launcher) grid.
//
// Cursor selects a (track, pattern) cell in the 8 × NumPatterns grid.
// CursorTrack is in [0, 7]; CursorPattern is in [0, NumPatterns-1].
// ViewOffset scrolls the visible window vertically when NumPatterns
// exceeds the available row count.
type Session struct {
	CursorTrack   int
	CursorPattern int
	ViewOffset    int
}
