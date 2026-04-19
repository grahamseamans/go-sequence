// Package save is the project-level save/load browser.
//
// Unlike the per-track musical devices, save has no device struct and
// instantiates no singleton: its entire state (cursor, edit buffer, cached
// save list) lives on model.UIState as a system.Project value. All handlers
// are free functions that take that *system.Project explicitly, following
// DESIGN §3 ("controller is bags of functions, no types") and DESIGN §4.8
// ("focus / UI state lives on model.UIState").
package save

import (
	"strings"

	"go-sequence/controller/surface"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/system"
)

// SaveOps is the file-I/O surface save handlers need. Implemented by the
// controller/ package (controller/project.go). Passed into handlers so
// controller/devices/save can stay ignorant of controller/ (which would
// create an import cycle: controller → controller/devices → controller).
//
// Errors are returned to the handler; for now we surface them minimally
// (edit mode cleared, cache refreshed). A future step can add a status-
// line channel so the view can display failures.
type SaveOps interface {
	Save(project *model.Project, name string) error
	Load(path string) (*model.Project, error)
	ListSaves() ([]string, error)
	DeleteSave(name string) error
	RenameSave(oldName, newName string) error
}

// Refresh pulls the latest save list from ops into sp.Cached. Called on
// entry (by the input router) and after any mutation. Clamps cursor into
// the new range so a delete doesn't leave it dangling.
func Refresh(sp *system.Project, ops SaveOps) {
	if sp == nil {
		return
	}
	if ops == nil {
		sp.Cached = nil
		sp.Cursor = 0
		return
	}
	saves, err := ops.ListSaves()
	if err != nil {
		// ListSaves failed — surface as empty list rather than stale data.
		// No silent-catch: we still clear Cached and reset the cursor, so a
		// later Render can make "no saves" visible.
		sp.Cached = nil
		sp.Cursor = 0
		return
	}
	sp.Cached = saves
	if sp.Cursor < 0 {
		sp.Cursor = 0
	}
	if sp.Cursor >= len(sp.Cached) {
		if len(sp.Cached) == 0 {
			sp.Cursor = 0
		} else {
			sp.Cursor = len(sp.Cached) - 1
		}
	}
}

// IsEditing reports whether the save browser is currently in name-edit mode.
// Callers (e.g. the input router) use this to distinguish enter-commits-
// a-name from enter-loads-a-project.
func IsEditing(sp *system.Project) bool {
	if sp == nil {
		return false
	}
	return sp.EditMode
}

// HandlePad handles pad input on the save view. The old browser drove a
// pad grid for project/save selection; with the simplified flat "list of
// saves" model that's redundant with the keyboard cursor. Leave as a
// no-op and revisit if a touch-first UX is needed.
func HandlePad(sp *system.Project, project *model.Project, ops SaveOps, out midi.ToExternal, row, col int, down bool) {
	// Intentional no-op.
}

// HandleKey handles keyboard input on the save view.
//
// Keybindings (non-edit mode):
//
//	j / down     cursor down
//	k / up       cursor up
//	enter         load save at cursor (stops playback elsewhere, per DESIGN §4.5)
//	s            save current project (enters EditNewSave mode to name it)
//	d / delete   delete save at cursor
//	r / F2       rename save at cursor (enters EditRename mode)
//
// Keybindings (edit mode):
//
//	enter     commit the buffered name
//	esc       cancel edit
//	backspace drop last rune of EditBuf
//	<printable> append to EditBuf
func HandleKey(sp *system.Project, project *model.Project, ops SaveOps, out midi.ToExternal, key string) {
	if sp == nil || project == nil || ops == nil {
		return
	}

	if sp.EditMode {
		handleEditKey(sp, project, ops, key)
		return
	}

	switch key {
	case "j", "down":
		if sp.Cursor < len(sp.Cached)-1 {
			sp.Cursor++
		}
	case "k", "up":
		if sp.Cursor > 0 {
			sp.Cursor--
		}
	case "enter":
		if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
			return
		}
		name := sp.Cached[sp.Cursor]
		loaded, err := ops.Load(name)
		if err != nil || loaded == nil {
			// Load failure. We don't mutate project; callers can inspect the
			// refreshed cache for evidence the file list changed under us.
			Refresh(sp, ops)
			return
		}
		// Swap the project's persisted fields in place. The input router
		// holds the same *model.Project pointer the rest of the system
		// reads from, so we mustn't replace the pointer. Runtime state
		// (PlaybackState) stays put because it contains atomics that can't
		// be copied.
		project.ReplacePersisted(loaded)
		project.Validate()
		Refresh(sp, ops)
	case "s":
		sp.EditMode = true
		sp.EditKind = system.EditNewSave
		sp.EditBuf = ""
	case "d", "delete":
		if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
			return
		}
		name := sp.Cached[sp.Cursor]
		if err := ops.DeleteSave(name); err != nil {
			// Delete failure; refresh to show true state.
			Refresh(sp, ops)
			return
		}
		Refresh(sp, ops)
	case "r", "f2":
		if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
			return
		}
		sp.EditMode = true
		sp.EditKind = system.EditRename
		sp.EditBuf = sp.Cached[sp.Cursor]
	}
}

// handleEditKey handles keyboard input while the name editor is active.
func handleEditKey(sp *system.Project, project *model.Project, ops SaveOps, key string) {
	switch key {
	case "enter":
		name := strings.TrimSpace(sp.EditBuf)
		if name == "" {
			// Empty name — cancel rather than commit to an empty filename.
			exitEdit(sp)
			return
		}
		switch sp.EditKind {
		case system.EditNewSave:
			if err := ops.Save(project, name); err != nil {
				exitEdit(sp)
				Refresh(sp, ops)
				return
			}
		case system.EditRename:
			if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
				exitEdit(sp)
				return
			}
			oldName := sp.Cached[sp.Cursor]
			if err := ops.RenameSave(oldName, name); err != nil {
				exitEdit(sp)
				Refresh(sp, ops)
				return
			}
		}
		exitEdit(sp)
		Refresh(sp, ops)
	case "esc":
		exitEdit(sp)
	case "backspace":
		if len(sp.EditBuf) > 0 {
			sp.EditBuf = sp.EditBuf[:len(sp.EditBuf)-1]
		}
	default:
		// Accept single printable ASCII characters. Reject path separators
		// explicitly — the EditBuf is used as a filename downstream.
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 && key != "/" && key != "\\" {
			sp.EditBuf += key
		}
	}
}

// exitEdit clears editor state. Called on commit, cancel, and any failure.
func exitEdit(sp *system.Project) {
	sp.EditMode = false
	sp.EditKind = system.EditNone
	sp.EditBuf = ""
}

// Render is a stub; the view layer will port the TUI view.
func Render(sp *system.Project, project *model.Project, ops SaveOps) string { return "" }

// RenderLEDs is a stub; the view layer will port the LED layout.
func RenderLEDs(sp *system.Project, project *model.Project, ops SaveOps) []surface.LED {
	return nil
}
