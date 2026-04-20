// Package save is the project-level save/load domain.
//
// Save has no device struct and instantiates no singleton: its entire state
// (cursor, edit buffer, cached save list) lives on model.UIState as a
// system.Project value. UI handlers live under view/{launchpad,tui}/project.go.
//
// What remains here is the SaveOps interface (the on-disk I/O surface that
// views call) and the Refresh / IsEditing helpers that the view layer needs
// to synchronize the widget state with disk. Keeping these in controller/
// preserves the invariant that persistent I/O flows through the controller
// layer even when the UI driving it sits in view/.
package save

import (
	"go-sequence/model"
	"go-sequence/model/system"
)

// SaveOps is the file-I/O surface save views need. Implemented by the
// controller/ package (controller/project.go). Injected into view handlers
// so view/tui/project.go can stay ignorant of controller/ (which would
// create an import cycle).
//
// Errors are returned to the handler; for now we surface them minimally
// (edit mode cleared, cache refreshed). A future step can add a status-line
// channel so the view can display failures.
type SaveOps interface {
	Save(project *model.Project, name string) error
	// LoadByName reads the save with the given bare name (no extension, no
	// path) and returns a validated *model.Project. The implementation owns
	// the saves-directory path join so views stay ignorant of on-disk
	// layout.
	LoadByName(name string) (*model.Project, error)
	ListSaves() ([]string, error)
	DeleteSave(name string) error
	RenameSave(oldName, newName string) error
}

// Refresh pulls the latest save list from ops into sp.Cached. Called on
// entry (by the top-level UI dispatcher) and after any mutation. Clamps
// cursor into the new range so a delete doesn't leave it dangling.
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
// Callers (the top-level key dispatcher) use this to distinguish
// enter-commits-a-name from enter-loads-a-project.
func IsEditing(sp *system.Project) bool {
	if sp == nil {
		return false
	}
	return sp.EditMode
}
