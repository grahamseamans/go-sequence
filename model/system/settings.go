package system

// Settings holds UI state for the per-track settings editor.
//
// Cursor row is the global Focus.Track (kept in sync by the keyboard
// handler so the dispatcher's track-lock always matches the cursor).
// Col in [0, 3] selects a column (Type, Channel, Port, Kit). Popup
// carries the modal picker state when the user is editing a cell —
// closed by default.
type Settings struct {
	Col   int
	Popup PopupState
}

// SettingsCol identifies which cell of a settings row is selected.
type SettingsCol uint8

const (
	SettingsColType SettingsCol = iota
	SettingsColChannel
	SettingsColPort
	SettingsColKit
)

// SettingsColCount is the number of editable columns in the settings table.
const SettingsColCount = 4

// PopupKind identifies what kind of editor the popup is rendering.
type PopupKind uint8

const (
	PopupNone PopupKind = iota
	PopupDeviceType
	PopupChannel
	PopupOutput
	PopupKit
)

// PopupState carries the open/closed state and contents of the settings
// modal picker. When Open is false everything else is ignored.
//
// Options is the visible list of strings (rendered top-to-bottom); Selected
// is the index into Options. TrackIndex captures which track was focused
// when the popup opened, so a commit knows which track to write to even if
// the user has navigated since.
type PopupState struct {
	Open       bool
	Kind       PopupKind
	Options    []string
	Selected   int
	TrackIndex int
}
