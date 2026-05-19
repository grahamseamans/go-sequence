// Package controller: project.go — on-disk persistence + browser helpers.
//
// Save layout:
//
//	~/.go-sequence/saves/<name>.json
//
// A "save" is a single JSON file holding a whole *model.Project. The project's
// identity is its file name on disk; there is no in-project Name field (by
// design — model/ stays pure data and doesn't track its own filename).
//
// Writes use a tmp-file + rename for atomicity, so a crash mid-write never
// leaves a half-written save at the target path. ListSaves deliberately
// skips leftover *.tmp.json files.
//
// LoadProject does NOT stop playback or trigger a recompile. Per DESIGN §4.5
// that is the view layer's job: after calling SaveOps.LoadByName, the project-
// browser view pauses playback and calls controller.MarkPlayingDirty for
// each track so the compile goroutine re-renders against the fresh specs.
//
// controllerSaveOps wraps the package-level helpers and implements
// save.SaveOps so that controller/devices/save can stay ignorant of
// controller/ (no import cycle). NewControllerSaveOps is the factory the
// wiring code (main.go) calls.
package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-sequence/controller/devices/save"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// savesDir returns the absolute path to the fixed saves directory. The
// directory itself is created lazily on first SaveProject call.
func savesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".go-sequence", "saves"), nil
}

// SaveProject serializes p to ~/.go-sequence/saves/<name>.json. The write is
// atomic: content goes to <name>.json.tmp first, then os.Rename swaps it into
// place. A crash mid-write leaves the old file (or nothing) untouched — never
// a half-written target.
//
// name is the bare save name: no extension, no path separators. Empty names
// and names containing '/' or '\' are rejected rather than silently
// rewritten, so the caller always sees the same file it asked for.
func SaveProject(p *model.Project, name string) error {
	if p == nil {
		return errors.New("SaveProject: nil project")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("SaveProject: empty name")
	}
	if strings.ContainsAny(name, "/\\") {
		return errors.New("SaveProject: name must not contain path separators")
	}

	dir, err := savesDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	path := filepath.Join(dir, name+".json")
	bytes, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Clean up the tmp on rename failure so we don't accrete stale files.
		// The outer error is the interesting one; we swallow the Remove error
		// deliberately since the user will already see the rename failure and
		// ListSaves filters out *.tmp.json anyway.
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// LoadProject reads path, unmarshals it into a *model.Project, runs
// Validate() to clamp out-of-range fields, and returns the project.
//
// This function is deliberately I/O-only: it does NOT stop playback, does
// NOT swap the live project pointer, and does NOT trigger recompiles. Per
// DESIGN §4.5 those responsibilities sit with the input router (which is
// the sole writer to the live project state) and the compile goroutine
// (which is the only producer of CompiledPatterns).
func LoadProject(path string) (*model.Project, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var p model.Project
	if err := json.Unmarshal(bytes, &p); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	p.Validate()
	return &p, nil
}

// NewProject constructs a fresh project with sensible defaults so the user
// can hit play immediately without first configuring anything.
//
// Track 0 is pre-assigned to Drum with the GM kit and an empty Drum spec.
// Validate() fills in the pattern lengths (one bar of 16th-notes in ticks)
// and clamps the rest. Tracks 1-7 stay DeviceNone until the user assigns
// a device.
//
// There is no name parameter: the project's identity is its file name on
// disk, not a field in the struct. If we want on-screen project names
// later, add them at the view layer, not here.
func NewProject() *model.Project {
	p := model.New()
	p.Tracks[0].Type = model.DeviceDrum
	p.Tracks[0].Kit = devices.DefaultKit
	p.Tracks[0].Device = &devices.Drum{}
	p.Validate()
	return p
}

// ListSaves returns all save names (without the .json extension) in
// alphabetical order. A missing saves directory is not an error — it just
// means no saves yet — so the result is (nil, nil) in that case.
//
// *.tmp.json files (leftovers from SaveProject's atomic rename) are filtered
// out; they're not real saves.
func ListSaves() ([]string, error) {
	dir, err := savesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("readdir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		if strings.HasSuffix(n, ".tmp.json") {
			continue
		}
		names = append(names, strings.TrimSuffix(n, ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// DeleteSave removes ~/.go-sequence/saves/<name>.json. Empty name is an
// error; any underlying os.Remove error (including "file not found") is
// returned verbatim so callers can distinguish.
func DeleteSave(name string) error {
	if name == "" {
		return errors.New("DeleteSave: empty name")
	}
	dir, err := savesDir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, name+".json"))
}

// RenameSave renames a save from oldName.json to newName.json within the
// saves directory. Like SaveProject, newName is rejected if it contains a
// path separator, so the rename can't escape the saves directory.
func RenameSave(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return errors.New("RenameSave: empty name")
	}
	if strings.ContainsAny(newName, "/\\") {
		return errors.New("RenameSave: newName must not contain path separators")
	}
	dir, err := savesDir()
	if err != nil {
		return err
	}
	return os.Rename(
		filepath.Join(dir, oldName+".json"),
		filepath.Join(dir, newName+".json"),
	)
}

// controllerSaveOps is the save.SaveOps implementation that forwards to the
// package-level Save/Load/List/Delete/Rename functions. Injected into the
// view dispatchers (view/launchpad, view/tui) so the save package doesn't
// need to import controller/ (which would create a cycle).
type controllerSaveOps struct{}

// NewControllerSaveOps returns the concrete SaveOps impl for injection.
func NewControllerSaveOps() save.SaveOps {
	return controllerSaveOps{}
}

func (controllerSaveOps) Save(p *model.Project, name string) error { return SaveProject(p, name) }

// LoadByName resolves the bare save name to its on-disk path
// (~/.go-sequence/saves/<name>.json) and delegates to LoadProject. Rejects
// empty names and path separators so a caller can't escape the saves dir.
func (controllerSaveOps) LoadByName(name string) (*model.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("LoadByName: empty name")
	}
	if strings.ContainsAny(name, "/\\") {
		return nil, errors.New("LoadByName: name must not contain path separators")
	}
	dir, err := savesDir()
	if err != nil {
		return nil, err
	}
	return LoadProject(filepath.Join(dir, name+".json"))
}

func (controllerSaveOps) ListSaves() ([]string, error) { return ListSaves() }
func (controllerSaveOps) DeleteSave(name string) error { return DeleteSave(name) }
func (controllerSaveOps) RenameSave(oldName, newName string) error {
	return RenameSave(oldName, newName)
}
