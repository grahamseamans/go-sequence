package system

// Project holds UI state for the project browser (load/save/rename/delete).
// Not a musical-domain entity — purely the widget's internal state.
type Project struct {
	// Cursor is the index into Cached of the currently-highlighted save.
	Cursor int

	// EditMode is true when the user is typing into EditBuf — either to
	// name a new save or to rename an existing one.
	EditMode bool

	// EditKind distinguishes "save current project as <EditBuf>" from
	// "rename Saves[Cursor] to <EditBuf>".
	EditKind EditKind

	// EditBuf is the in-progress filename while EditMode is true.
	EditBuf string

	// Cached is the last-known list of save names (no directory path).
	// Refreshed on entry and after any mutation.
	Cached []string
}

// EditKind identifies what a rename/save edit is for.
type EditKind uint8

const (
	EditNone EditKind = iota
	EditNewSave
	EditRename
)
