package tui

import (
	"strings"

	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/system"
)

// projectKey handles keyboard input on the project (save/load) browser view.
//
// Ported from controller/devices/save.HandleKey.
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
func projectKey(sp *system.Project, project *model.Project, ops save.SaveOps, out midi.ToExternal, key string) {
	if sp == nil || project == nil || ops == nil {
		return
	}

	if sp.EditMode {
		projectEditKey(sp, project, ops, key)
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
			save.Refresh(sp, ops)
			return
		}
		// Swap the project's persisted fields in place so the rest of the
		// system keeps seeing the same pointer.
		project.ReplacePersisted(loaded)
		project.Validate()
		save.Refresh(sp, ops)
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
			save.Refresh(sp, ops)
			return
		}
		save.Refresh(sp, ops)
	case "r", "f2":
		if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
			return
		}
		sp.EditMode = true
		sp.EditKind = system.EditRename
		sp.EditBuf = sp.Cached[sp.Cursor]
	}
}

// projectRender returns the TUI view for the project browser. Stub.
func projectRender(sp *system.Project, project *model.Project, ops save.SaveOps) string {
	return ""
}

// projectEditKey handles keyboard input while the name editor is active.
func projectEditKey(sp *system.Project, project *model.Project, ops save.SaveOps, key string) {
	switch key {
	case "enter":
		name := strings.TrimSpace(sp.EditBuf)
		if name == "" {
			projectExitEdit(sp)
			return
		}
		switch sp.EditKind {
		case system.EditNewSave:
			if err := ops.Save(project, name); err != nil {
				projectExitEdit(sp)
				save.Refresh(sp, ops)
				return
			}
		case system.EditRename:
			if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
				projectExitEdit(sp)
				return
			}
			oldName := sp.Cached[sp.Cursor]
			if err := ops.RenameSave(oldName, name); err != nil {
				projectExitEdit(sp)
				save.Refresh(sp, ops)
				return
			}
		}
		projectExitEdit(sp)
		save.Refresh(sp, ops)
	case "esc":
		projectExitEdit(sp)
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

// projectExitEdit clears editor state.
func projectExitEdit(sp *system.Project) {
	sp.EditMode = false
	sp.EditKind = system.EditNone
	sp.EditBuf = ""
}
