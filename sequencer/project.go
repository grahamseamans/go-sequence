package sequencer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SaveInfo represents a saved project file (for listing)
type SaveInfo struct {
	Filename  string
	Name      string // parsed from filename (empty if unnamed)
	Timestamp time.Time
}

// ProjectsDir returns the projects directory path
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "go-sequence", "projects"), nil
}

// ProjectDir returns the path to a specific project
func ProjectDir(projectName string) (string, error) {
	base, err := ProjectsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, projectName), nil
}

// ListProjects returns all project folder names
func ListProjects() ([]string, error) {
	dir, err := ProjectsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var projects []string
	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, entry.Name())
		}
	}

	sort.Strings(projects)
	return projects, nil
}

// ListSaves returns timestamped saves for a project, newest first
func ListSaves(projectName string) ([]SaveInfo, error) {
	dir, err := ProjectDir(projectName)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SaveInfo{}, nil
		}
		return nil, err
	}

	var saves []SaveInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		// Parse filename: 2024-01-15_14-30-00.json or 2024-01-15_14-30-00_name.json
		baseName := strings.TrimSuffix(name, ".json")

		// Timestamp is first 19 chars: 2006-01-02_15-04-05
		if len(baseName) < 19 {
			continue
		}

		tsStr := baseName[:19]
		ts, err := time.Parse("2006-01-02_15-04-05", tsStr)
		if err != nil {
			// Not a timestamped file, skip
			continue
		}

		// Check for name after timestamp
		saveName := ""
		if len(baseName) > 20 && baseName[19] == '_' {
			saveName = baseName[20:] // everything after the underscore
		}

		saves = append(saves, SaveInfo{
			Filename:  name,
			Name:      saveName,
			Timestamp: ts,
		})
	}

	// Sort by timestamp, newest first
	sort.Slice(saves, func(i, j int) bool {
		return saves[i].Timestamp.After(saves[j].Timestamp)
	})

	return saves, nil
}

// SaveProject saves current state to project with timestamp
func SaveProject(projectName string) error {
	if projectName == "" {
		projectName = "untitled"
	}

	dir, err := ProjectDir(projectName)
	if err != nil {
		return err
	}

	// Create project directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Serialize state
	data, err := json.MarshalIndent(S, "", "  ")
	if err != nil {
		return err
	}

	// Save with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	timestampPath := filepath.Join(dir, timestamp+".json")
	if err := os.WriteFile(timestampPath, data, 0644); err != nil {
		return err
	}

	// Update project name in runtime state
	S.ProjectName = projectName

	return nil
}

// LoadProject loads a specific save (or most recent if filename empty)
func LoadProject(projectName, filename string) error {
	dir, err := ProjectDir(projectName)
	if err != nil {
		return err
	}

	// If no filename specified, load most recent
	if filename == "" {
		saves, err := ListSaves(projectName)
		if err != nil || len(saves) == 0 {
			return fmt.Errorf("no saves found in project %s", projectName)
		}
		filename = saves[0].Filename // saves are sorted newest first
	}

	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Create a new state to load into
	newState := NewState()
	if err := json.Unmarshal(data, newState); err != nil {
		return err
	}

	// Copy loaded state to global singleton
	*S = *newState
	S.ProjectName = projectName

	// Reset runtime-only fields
	S.Playing = false
	S.Step = 0
	for _, track := range S.Tracks {
		if track.Drum != nil {
			track.Drum.Step = 0
			track.Drum.Recording = false
			track.Drum.Preview = false
		}
		if track.Piano != nil {
			track.Piano.Step = 0
			track.Piano.LastBeat = 0
			track.Piano.Recording = false
			track.Piano.Preview = false
		}
	}

	return nil
}

// CreateProject creates a new empty project folder
func CreateProject(name string) error {
	dir, err := ProjectDir(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// DeleteSave deletes a specific save file
func DeleteSave(projectName, filename string) error {
	dir, err := ProjectDir(projectName)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, filename)
	return os.Remove(path)
}

// RenameSave renames a save file (changes the name part, keeps timestamp)
func RenameSave(projectName, oldFilename, newName string) error {
	dir, err := ProjectDir(projectName)
	if err != nil {
		return err
	}

	// Parse the timestamp from old filename
	baseName := strings.TrimSuffix(oldFilename, ".json")
	if len(baseName) < 19 {
		return fmt.Errorf("invalid save filename")
	}
	tsStr := baseName[:19]

	// Build new filename
	var newFilename string
	if newName == "" {
		newFilename = tsStr + ".json"
	} else {
		// Sanitize name for filesystem
		safeName := sanitizeFilename(newName)
		newFilename = tsStr + "_" + safeName + ".json"
	}

	oldPath := filepath.Join(dir, oldFilename)
	newPath := filepath.Join(dir, newFilename)
	return os.Rename(oldPath, newPath)
}

// sanitizeFilename removes/replaces characters that are problematic in filenames
func sanitizeFilename(name string) string {
	// Replace spaces with hyphens, remove slashes and other problematic chars
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "")
	name = strings.ReplaceAll(name, "?", "")
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "<", "")
	name = strings.ReplaceAll(name, ">", "")
	name = strings.ReplaceAll(name, "|", "")
	return name
}

// DeleteProject deletes entire project folder
func DeleteProject(name string) error {
	dir, err := ProjectDir(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// RenameProject renames a project folder
func RenameProject(oldName, newName string) error {
	oldDir, err := ProjectDir(oldName)
	if err != nil {
		return err
	}

	newDir, err := ProjectDir(newName)
	if err != nil {
		return err
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}

	// Update project name if this is the current project
	if S.ProjectName == oldName {
		S.ProjectName = newName
	}

	return nil
}
