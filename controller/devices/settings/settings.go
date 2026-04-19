// Package settings is the project-level track/MIDI configuration domain.
// It has no Compile or mutators of its own — the edits are applied
// directly to project.Tracks[i] fields by the UI handlers under
// view/{launchpad,tui}/settings.go.
//
// This file is a domain marker so the tree stays parallel to the other
// domains.
package settings
