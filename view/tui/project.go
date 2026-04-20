package tui

import (
	"fmt"
	"strings"

	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/system"
	"go-sequence/theme"
)

// projectKey handles keyboard input on the project (save/load) browser view.
//
// Keybindings (non-edit mode):
//
//	j / down     cursor down
//	k / up       cursor up
//	enter        load save at cursor (swap persisted fields in place)
//	s            save current project (enters EditNewSave mode to name it)
//	d / delete   delete save at cursor
//	r / f2       rename save at cursor (enters EditRename mode, prefilled)
//
// Keybindings (edit mode):
//
//	enter        commit the buffered name
//	esc          cancel edit
//	backspace    drop last byte of EditBuf
//	<printable>  append to EditBuf (path separators rejected)
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
	case "enter", "return":
		if sp.Cursor < 0 || sp.Cursor >= len(sp.Cached) {
			return
		}
		name := sp.Cached[sp.Cursor]
		loaded, err := ops.LoadByName(name)
		if err != nil || loaded == nil {
			// Load failure: refresh the cache so the user sees the current
			// disk state rather than a stale list. We don't mutate project.
			save.Refresh(sp, ops)
			return
		}
		// Swap the project's persisted fields in place so the rest of the
		// system keeps seeing the same pointer. Validate clamps the loaded
		// data; playback pause + Machine-slot recompile are handled by the
		// outer HandleKey's wasLoad branch (tui.go).
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

// projectRender returns the TUI view for the project browser.
//
// Two modes:
//
//	edit-mode:  "Edit <kind>: <buf>_"
//	browsing:   scrollable list of Cached saves, cursor highlighted, with a
//	            key-help footer.
func projectRender(sp *system.Project, project *model.Project, ops save.SaveOps, th *theme.Theme) string {
	if sp == nil {
		return ""
	}

	titleStyle := themeStyle(th, roleFG).Bold(true)
	cursorStyle := themeStyle(th, roleCursor).Bold(true)
	editPromptStyle := themeStyle(th, roleAccent).Bold(true)
	mutedStyle := themeStyle(th, roleMuted)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Project Browser"))
	b.WriteString("\n\n")

	if sp.EditMode {
		kindLabel := "?"
		switch sp.EditKind {
		case system.EditNewSave:
			kindLabel = "new save"
		case system.EditRename:
			kindLabel = "rename"
		}
		b.WriteString(editPromptStyle.Render(fmt.Sprintf("Edit %s: %s_", kindLabel, sp.EditBuf)))
		b.WriteString("\n\n  enter=commit   esc=cancel   backspace=delete\n")
		return b.String()
	}

	if len(sp.Cached) == 0 {
		b.WriteString(mutedStyle.Render("  (no saves yet — press 's' to save the current project)"))
		b.WriteString("\n\n")
	} else {
		for i, name := range sp.Cached {
			if i == sp.Cursor {
				b.WriteString(cursorStyle.Render(fmt.Sprintf("> %s", name)))
			} else {
				b.WriteString(fmt.Sprintf("  %s", name))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("  enter=load   s=save   r=rename   d=delete   j/k=move\n")
	return b.String()
}

// projectEditKey handles keyboard input while the name editor is active.
//
// The committed name flows through one of two ops depending on EditKind:
// EditNewSave calls ops.Save to write the current project under the typed
// name; EditRename calls ops.RenameSave on the cursor save. A commit always
// exits edit mode and refreshes the cache so the new file is visible.
func projectEditKey(sp *system.Project, project *model.Project, ops save.SaveOps, key string) {
	switch key {
	case "enter", "return":
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
		// explicitly — the EditBuf becomes a filename downstream.
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
