// Package controller: mark_dirty.go — pattern-invalidation helpers.
//
// After a view mutates spec state, it asks the compile goroutine to refill
// the affected pattern's Machine slots. These helpers trash both slots and
// send two CompileRequests (one per slot) so the "fresh rolls every loop"
// invariant (DESIGN §3.10) falls out of every request getting its own
// seed.
//
// Exported so view/{launchpad,tui,keyboard} can call them directly after
// their per-domain handlers mutate state. Moved out of the old
// controller/input.go when the UI input loops were pulled into view/.
package controller

import (
	"go-sequence/debug"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// MarkTrackDirty trashes both Machine slots of the currently-editing pattern
// on the named track and signals the compiler to refill them. The editing
// pattern is resolved from the device's EditingPatternIdx.
func MarkTrackDirty(project *model.Project, trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return
	}
	var patternIdx int
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			return
		}
		patternIdx = track.Drum.EditingPatternIdx
	case model.DevicePiano:
		if track.Piano == nil {
			return
		}
		patternIdx = track.Piano.EditingPatternIdx
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			return
		}
		patternIdx = track.Metropolix.EditingPatternIdx
	default:
		return
	}
	MarkPatternDirty(project, trackIdx, patternIdx)
}

// MarkPlayingDirty is like MarkTrackDirty but targets the cursor's
// CurrentPattern rather than the editing one. Used after project load to
// prime the Machine slots playback will read from.
func MarkPlayingDirty(project *model.Project, trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return
	}
	MarkPatternDirty(project, trackIdx, project.Playback.Cursors[trackIdx].CurrentPattern)
}

// MarkPatternDirty trashes both Machine slots of the named pattern on the
// named track, then non-blocking-sends ONE CompileRequest per slot so the
// compiler refills them with fresh rolls. Used by session (queue-change)
// and device handlers (spec edits).
//
// Trashing before sending is load-bearing: the nil-store MUST be visible
// before playback reads the slot — atomic.Store provides the needed publish
// semantics. (Compile unconditionally overwrites the slot, so the nil isn't
// strictly needed for compile's correctness, but playback observing a stale
// cached CompiledPattern between the edit and the compile completing would
// briefly emit wrong events. The nil makes playback silent for that window
// instead.)
func MarkPatternDirty(project *model.Project, trackIdx, patternIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	if patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	track := project.Tracks[trackIdx]
	if track == nil {
		return
	}
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil && track.Drum.Patterns[patternIdx] != nil {
			track.Drum.Patterns[patternIdx].Machine[0].Store(nil)
			track.Drum.Patterns[patternIdx].Machine[1].Store(nil)
		}
	case model.DevicePiano:
		if track.Piano != nil && track.Piano.Patterns[patternIdx] != nil {
			track.Piano.Patterns[patternIdx].Machine[0].Store(nil)
			track.Piano.Patterns[patternIdx].Machine[1].Store(nil)
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil && track.Metropolix.Patterns[patternIdx] != nil {
			track.Metropolix.Patterns[patternIdx].Machine[0].Store(nil)
			track.Metropolix.Patterns[patternIdx].Machine[1].Store(nil)
		}
	default:
		return
	}
	for slot := 0; slot < 2; slot++ {
		select {
		case project.Compile.Channel <- model.CompileRequest{Track: trackIdx, Pattern: patternIdx, Slot: slot}:
		default:
			debug.Log("mark_dirty", "compileCh full, dropping compile request for track=%d pattern=%d slot=%d", trackIdx, patternIdx, slot)
		}
	}
}
