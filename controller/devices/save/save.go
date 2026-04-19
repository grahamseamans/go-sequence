package save

import (
	"strings"

	"go-sequence/controller/surface"
	"go-sequence/midi"
	"go-sequence/model"
)

// SaveOps is the file-I/O surface Save needs. Implemented by the
// controller/ package (controller/project.go) in step 4. Passed into Save's
// handlers so devices/ can avoid importing controller/ (which would create
// an import cycle: controller → controller/devices → controller).
//
// Errors are returned to the handler; for now we surface them minimally
// (edit mode cleared, cache refreshed). A future step can add a status-
// line channel through Save so the view can display failures.
type SaveOps interface {
	Save(project *model.Project, name string) error
	Load(path string) (*model.Project, error)
	ListSaves() ([]string, error)
	DeleteSave(name string) error
	RenameSave(oldName, newName string) error
}

// Save is the project-level save/load browser.
//
// Divergences from the canonical empty-struct shape:
//
//  1. Save holds transient UI state (cursor, edit-mode buffer, cached
//     save list). Empty-struct singletons can't carry that without globals.
//     Per the step 3e brief we put it on the struct and flag this for
//     possible relocation to InputManager later (DESIGN §4.8-style: UI
//     cursor/edit state is not Model data).
//
//  2. Every handler takes an extra `ops SaveOps` parameter to reach the
//     filesystem without importing controller/.
//
// Save is a non-empty singleton in practice: one Save value lives on the
// InputManager (it's the only caller); no code instantiates Save twice.
type Save struct {
	// cursor is the index into cached of the currently-highlighted save.
	cursor int

	// editMode is true when the user is typing into editBuf — either to
	// name a new save or to rename an existing one.
	editMode bool

	// editKind distinguishes "save current project as <editBuf>" from
	// "rename saves[cursor] to <editBuf>".
	editKind editKind

	// editBuf is the in-progress filename while editMode is true.
	editBuf string

	// cached is the last-known list of save names (no directory path; just
	// filenames or project-level names — whatever SaveOps.ListSaves returns).
	// Refreshed on entry and after any mutation.
	cached []string
}

// editKind is a small enum distinguishing Save's two text-input modes.
type editKind uint8

const (
	editNone editKind = iota
	editNewSave
	editRename
)

// Refresh pulls the latest save list from ops into cached. Called on entry
// (by InputManager) and after any mutation. Clamps cursor into the new
// range so a delete doesn't leave it dangling.
func (s *Save) Refresh(ops SaveOps) {
	if ops == nil {
		s.cached = nil
		s.cursor = 0
		return
	}
	saves, err := ops.ListSaves()
	if err != nil {
		// ListSaves failed — surface as empty list rather than stale data.
		// No silent-catch: we still clear cached and reset the cursor, so a
		// later Render can make "no saves" visible.
		s.cached = nil
		s.cursor = 0
		return
	}
	s.cached = saves
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.cached) {
		if len(s.cached) == 0 {
			s.cursor = 0
		} else {
			s.cursor = len(s.cached) - 1
		}
	}
}

// HandlePad handles pad input on the save view. The old browser drove a
// pad grid for project/save selection; with the simplified flat "list of
// saves" model that's redundant with the keyboard cursor. Leave as a
// no-op and revisit in step 5 if a touch-first UX is needed.
// TODO(step5): decide whether to restore a pad-grid browser.
func (s *Save) HandlePad(project *model.Project, ops SaveOps, out midi.ToExternal, row, col int, down bool) {
	// Intentional no-op.
}

// HandleKey handles keyboard input on the save view.
//
// Keybindings (non-edit mode):
//
//	j / down     cursor down
//	k / up       cursor up
//	enter         load save at cursor (stops playback elsewhere, per DESIGN §4.5)
//	s            save current project (enters editNewSave mode to name it)
//	d / delete   delete save at cursor
//	r / F2       rename save at cursor (enters editRename mode)
//
// Keybindings (edit mode):
//
//	enter     commit the buffered name
//	esc       cancel edit
//	backspace drop last rune of editBuf
//	<printable> append to editBuf
//
// The step 3e brief also mentions arrow navigation; we honor left/right as
// alternate escape/commit for consistency with the drum handler's h/l
// navigation — but the simplest mapping is: arrows do nothing in edit
// mode, commit/cancel require enter/esc explicitly. That avoids a typo
// committing a half-finished name.
//
// Divergences from the old sequencer/save.go:
//   - Projects-vs-saves two-column layout is dropped. The old code had a
//     notion of "projects" (directories) each containing multiple dated
//     "saves". The new SaveOps surface is flat (ListSaves returns a single
//     list). If we want the project/save split back, extend SaveOps
//     rather than re-adding it here.
//   - Confirmation dialog on delete is dropped. ops.DeleteSave is called
//     immediately; a confirmation step can land later via a third edit
//     kind if the UX demands it.
//   - Note-input flag and recreateDevicesFromState are dropped — neither
//     belongs in the new model.
func (s *Save) HandleKey(project *model.Project, ops SaveOps, out midi.ToExternal, key string) {
	if project == nil || ops == nil {
		return
	}

	if s.editMode {
		s.handleEditKey(project, ops, key)
		return
	}

	switch key {
	case "j", "down":
		if s.cursor < len(s.cached)-1 {
			s.cursor++
		}
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
	case "enter":
		if s.cursor < 0 || s.cursor >= len(s.cached) {
			return
		}
		name := s.cached[s.cursor]
		loaded, err := ops.Load(name)
		if err != nil || loaded == nil {
			// Load failure. We don't mutate project; callers can inspect the
			// refreshed cache for evidence the file list changed under us.
			s.Refresh(ops)
			return
		}
		// Swap the project contents in place. InputManager holds the same
		// *model.Project pointer the rest of the system reads from, so we
		// must not replace the pointer — only its contents.
		*project = *loaded
		project.Validate()
		s.Refresh(ops)
	case "s":
		s.editMode = true
		s.editKind = editNewSave
		s.editBuf = ""
	case "d", "delete":
		if s.cursor < 0 || s.cursor >= len(s.cached) {
			return
		}
		name := s.cached[s.cursor]
		if err := ops.DeleteSave(name); err != nil {
			// Delete failure; refresh to show true state.
			s.Refresh(ops)
			return
		}
		s.Refresh(ops)
	case "r", "f2":
		if s.cursor < 0 || s.cursor >= len(s.cached) {
			return
		}
		s.editMode = true
		s.editKind = editRename
		s.editBuf = s.cached[s.cursor]
	}
}

// handleEditKey handles keyboard input while the name editor is active.
func (s *Save) handleEditKey(project *model.Project, ops SaveOps, key string) {
	switch key {
	case "enter":
		name := strings.TrimSpace(s.editBuf)
		if name == "" {
			// Empty name — cancel rather than commit to an empty filename.
			s.exitEdit()
			return
		}
		switch s.editKind {
		case editNewSave:
			if err := ops.Save(project, name); err != nil {
				s.exitEdit()
				s.Refresh(ops)
				return
			}
		case editRename:
			if s.cursor < 0 || s.cursor >= len(s.cached) {
				s.exitEdit()
				return
			}
			oldName := s.cached[s.cursor]
			if err := ops.RenameSave(oldName, name); err != nil {
				s.exitEdit()
				s.Refresh(ops)
				return
			}
		}
		s.exitEdit()
		s.Refresh(ops)
	case "esc":
		s.exitEdit()
	case "backspace":
		if len(s.editBuf) > 0 {
			s.editBuf = s.editBuf[:len(s.editBuf)-1]
		}
	default:
		// Accept single printable ASCII characters. Reject path separators
		// explicitly — the editBuf is used as a filename downstream.
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 && key != "/" && key != "\\" {
			s.editBuf += key
		}
	}
}

// exitEdit clears editor state. Called on commit, cancel, and any failure.
func (s *Save) exitEdit() {
	s.editMode = false
	s.editKind = editNone
	s.editBuf = ""
}

// Render is a stub; step 5 will port the TUI view from sequencer/save.go.
// TODO(step5): port rendering from sequencer/save.go when view/ rewires.
func (s *Save) Render(project *model.Project, ops SaveOps) string { return "" }

// RenderLEDs is a stub; step 5 will port the LED layout from sequencer/save.go.
// TODO(step5): port rendering from sequencer/save.go when view/ rewires.
func (s *Save) RenderLEDs(project *model.Project, ops SaveOps) []surface.LED { return nil }
