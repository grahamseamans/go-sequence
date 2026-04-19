// Package session is the project-level "clip launcher" domain. Session
// has no Compile or mutators of its own — all state lives on
// project.Tracks[*].<Device>.Schedule, and the UI logic that drives it
// lives under view/{launchpad,tui}/session.go.
//
// This file is a domain marker so the tree stays parallel to the other
// domains.
package session
