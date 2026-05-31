// Package controller: session.go — clip-launcher queue helpers.
//
// Both session surfaces (view/launchpad/session.go and view/tui/session.go)
// route through these so the two can't drift apart — queue/un-queue and scene
// launching live here, once. Each helper is self-contained: it locks the
// track for the cursor mutation and marks the affected pattern dirty so its
// Machine slots are compiled before playback wraps into them.
package controller

import (
	"go-sequence/model"
	"go-sequence/model/devices"
)

// ToggleQueue queues patternIdx on the track, or cancels the queue if that
// pattern is already queued — a second tap "un-queues" (QueuedPattern = -1,
// the "none" sentinel). The playback engine promotes QueuedPattern to
// CurrentPattern at the next loop boundary.
func ToggleQueue(project *model.Project, trackIdx, patternIdx int) {
	if project == nil || trackIdx < 0 || trackIdx >= 8 {
		return
	}
	if patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	track := project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return
	}

	track.Lock()
	cursor := &project.Playback.Cursors[trackIdx]
	if cursor.QueuedPattern == patternIdx {
		cursor.QueuedPattern = -1
	} else {
		cursor.QueuedPattern = patternIdx
	}
	track.Unlock()

	MarkPatternDirty(project, trackIdx, patternIdx)
}

// QueueScene queues pattern-column patternIdx on every device-bearing track at
// once — the "scene" gesture from the top row of round buttons. Unlike
// ToggleQueue it is NOT a toggle: it always queues the column, since launching
// an empty slot is the intended way to drop a track on a scene change.
func QueueScene(project *model.Project, patternIdx int) {
	if project == nil || patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone {
			continue
		}
		track.Lock()
		project.Playback.Cursors[i].QueuedPattern = patternIdx
		track.Unlock()
		MarkPatternDirty(project, i, patternIdx)
	}
}
