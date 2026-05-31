package controller

import (
	"context"
	"time"

	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// MaxDeltaTicks clamps a tick delta when the wall clock jumps (laptop sleep,
// host hiccup, etc.). Past this many ticks we treat it as a 1-tick advance
// instead of replaying thousands of events at once.
const MaxDeltaTicks = int64(devices.PPQ) // 1 quarter note

// tickSleep is the granularity at which the tick goroutine wakes up to check
// whether a tick boundary has elapsed. Small enough to keep jitter sub-ms at
// 120 BPM (tick interval ~520µs at PPQ=960) while letting the goroutine yield.
const tickSleep = 250 * time.Microsecond

// RunPlayback is the playback goroutine entry point. Reads the project's
// runtime transport state (project.Playback) and per-pattern Machine slots,
// dispatches MIDI out through `out`, and signals project.Compile.Channel to
// refill any consumed slots.
//
// Per DESIGN §8, the playhead is NOT stored. Pattern-relative position is
// derived each tick as (globalTick - cursor.T0Tick). Cursor only carries
// T0Tick + CurrentSlot.
//
// Start this as its own goroutine. Blocks until ctx is done.
func RunPlayback(ctx context.Context, project *model.Project, out midi.ToExternal) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		time.Sleep(tickSleep)

		if !project.Playback.Playing.Load() {
			continue
		}

		interval := tickInterval(project.Playback.Tempo.Load())
		if interval <= 0 {
			continue
		}

		project.Playback.T0Mu.RLock()
		elapsed := time.Since(project.Playback.T0)
		project.Playback.T0Mu.RUnlock()

		globalTick := int64(elapsed / interval)
		if globalTick > project.Playback.Tick.Load() {
			processTick(project, out, globalTick)
			project.Playback.Tick.Store(globalTick)
		}
	}
}

// tickInterval returns the wall-clock duration of a single tick at the given
// tempo. Returns 0 if bpm is non-positive (treated as "don't advance").
func tickInterval(bpm int64) time.Duration {
	if bpm <= 0 {
		return 0
	}
	return time.Duration(60e9 / (bpm * int64(devices.PPQ)))
}

// processTick walks each track, reads its currently-playing pattern's
// Machine[CurrentSlot], and dispatches events whose pattern-relative tick
// falls in (lastPos, newPos].
//
// On wrap, the window is split: tail of the old slot, handleWrap rotation,
// and head of the new slot.
func processTick(project *model.Project, out midi.ToExternal, globalTick int64) {
	lastGlobal := project.Playback.LastGlobalTick.Load()
	delta := globalTick - lastGlobal
	if delta > MaxDeltaTicks {
		delta = 1
		lastGlobal = globalTick - 1
	}
	if delta < 0 {
		delta = 0
	}

	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone {
			continue
		}

		cursor := &project.Playback.Cursors[i]

		curPatternIdx, machine := resolveCurrent(project, track, i)
		if machine == nil {
			requestCompile(project, i, curPatternIdx, cursor.CurrentSlot)
			continue
		}
		plen := machine.Length
		if plen < 1 {
			plen = 1
		}

		lastPos := lastGlobal - cursor.T0Tick
		newPos := globalTick - cursor.T0Tick

		if delta == 0 {
			continue
		}

		if newPos < plen {
			for _, ev := range machine.Events {
				if ev.Tick > lastPos && ev.Tick <= newPos {
					dispatch(out, track, ev)
				}
			}
			continue
		}

		// Wrap: tail of old slot, rotate, head of new slot.
		for _, ev := range machine.Events {
			if ev.Tick > lastPos && ev.Tick < plen {
				dispatch(out, track, ev)
			}
		}

		handleWrap(project, i, curPatternIdx, plen)

		remainder := newPos - plen
		if remainder < 0 {
			remainder = 0
		}
		_, newMachine := resolveCurrent(project, track, i)
		if newMachine != nil {
			for _, ev := range newMachine.Events {
				if ev.Tick >= 0 && ev.Tick <= remainder {
					dispatch(out, track, ev)
				}
			}
		}
	}

	project.Playback.LastGlobalTick.Store(globalTick)
}

// resolveCurrent reads the track's currently-playing pattern index and the
// *CompiledPattern loaded from Machine[cursor.CurrentSlot]. Returns (0, nil)
// if the track has no device or no compiled pattern yet.
//
// Holds the track's RLock briefly.
func resolveCurrent(project *model.Project, track *model.Track, trackIdx int) (int, *devices.CompiledPattern) {
	track.RLock()
	defer track.RUnlock()

	cursor := &project.Playback.Cursors[trackIdx]
	slot := cursor.CurrentSlot
	if slot < 0 || slot > 1 {
		slot = 0
		cursor.CurrentSlot = 0
	}
	idx := cursor.CurrentPattern

	switch d := track.Device.(type) {
	case *devices.Drum:
		if d == nil {
			return 0, nil
		}
		if idx < 0 || idx >= len(d.Patterns) {
			return 0, nil
		}
		pat := d.Patterns[idx]
		if pat == nil {
			return idx, nil
		}
		return idx, pat.Machine[slot].Load()
	case *devices.Looper:
		if d == nil {
			return 0, nil
		}
		if idx < 0 || idx >= len(d.Patterns) {
			return 0, nil
		}
		pat := d.Patterns[idx]
		if pat == nil {
			return idx, nil
		}
		return idx, pat.Machine[slot].Load()
	}
	return 0, nil
}

// dispatch sends a single event through MIDI out for the track's
// configured port + channel. Track-level muting was intentionally removed
// — silencing a track is the user's mixer's job; from the sequencer side
// you stop the pattern instead.
func dispatch(out midi.ToExternal, track *model.Track, ev devices.TimedEvent) {
	if track == nil {
		return
	}
	if err := out.Send(track.PortName, track.Channel, ev.Event); err != nil {
		// Throttled: a misconfigured/closed port fails on every dispatched
		// event, and unthrottled logging here reintroduces MIDI lag.
		debug.LogEvery(100, "playback", "send to %q ch%d: %v", track.PortName, track.Channel, err)
	}
}

// handleWrap runs per-track bookkeeping when the cursor crosses a pattern
// boundary: trash consumed slot, promote QueuedPattern → CurrentPattern,
// advance T0Tick, flip CurrentSlot, signal compile to refill.
func handleWrap(project *model.Project, trackIdx int, oldPatternIdx int, oldLength int64) {
	track := project.Tracks[trackIdx]
	if track == nil {
		return
	}
	cursor := &project.Playback.Cursors[trackIdx]
	trashedSlot := cursor.CurrentSlot

	trashSlot(track, oldPatternIdx, trashedSlot)

	newPatternIdx := oldPatternIdx
	if cursor.QueuedPattern != -1 {
		newPatternIdx = cursor.QueuedPattern
		cursor.CurrentPattern = newPatternIdx
		cursor.QueuedPattern = -1
	}

	cursor.T0Tick += oldLength
	cursor.CurrentSlot = 1 - trashedSlot

	requestCompile(project, trackIdx, oldPatternIdx, trashedSlot)
	if newPatternIdx != oldPatternIdx {
		requestCompile(project, trackIdx, newPatternIdx, 0)
		requestCompile(project, trackIdx, newPatternIdx, 1)
	}
}

// requestCompile is a non-blocking send on project.Compile.Channel.
func requestCompile(project *model.Project, trackIdx, patternIdx, slot int) {
	select {
	case project.Compile.Channel <- model.CompileRequest{Track: trackIdx, Pattern: patternIdx, Slot: slot}:
	default:
		debug.Log("playback", "compileCh full, dropped refill for track=%d pattern=%d slot=%d",
			trackIdx, patternIdx, slot)
	}
}

// trashSlot stores nil into pattern[patternIdx].Machine[slot].
func trashSlot(track *model.Track, patternIdx, slot int) {
	if track == nil {
		return
	}
	if slot < 0 || slot > 1 || patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	switch d := track.Device.(type) {
	case *devices.Drum:
		if d != nil && d.Patterns[patternIdx] != nil {
			d.Patterns[patternIdx].Machine[slot].Store(nil)
		}
	case *devices.Looper:
		if d != nil && d.Patterns[patternIdx] != nil {
			d.Patterns[patternIdx].Machine[slot].Store(nil)
		}
	}
}

// Play starts the transport. Anchors wall-clock T0, resets the tick counter
// and the transport-related cursor fields (T0Tick, CurrentSlot, QueuedPattern),
// preserves CurrentPattern so playback resumes on whatever pattern the user
// last had selected, and primes both Machine slots of that pattern on every
// track.
//
// Tick/LastGlobalTick start at -1 so the FIRST processTick call enters at
// globalTick=0 with a (-1, 0] walk window that includes tick 0 (the downbeat).
func Play(project *model.Project, lc midi.OutputLifecycle) {
	now := time.Now()
	project.Playback.T0Mu.Lock()
	project.Playback.T0 = now
	project.Playback.T0Mu.Unlock()
	project.Playback.Tick.Store(-1)
	project.Playback.LastGlobalTick.Store(-1)
	for i := range project.Playback.Cursors {
		c := &project.Playback.Cursors[i]
		c.T0Tick = 0
		c.CurrentSlot = 0
		c.QueuedPattern = -1
	}
	if project.Playback.Tempo.Load() <= 0 {
		project.Playback.Tempo.Store(int64(project.Tempo))
	}

	// Open every configured output port. A failing port doesn't block play
	// — the track will simply produce no output, giving the user a chance
	// to fix the port assignment in settings.
	seen := map[string]struct{}{}
	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone || track.PortName == "" {
			continue
		}
		if _, ok := seen[track.PortName]; ok {
			continue
		}
		seen[track.PortName] = struct{}{}
		if err := lc.EnsureOutput(track.PortName); err != nil {
			debug.Log("playback", "open output %q: %v", track.PortName, err)
		}
	}

	project.Playback.Playing.Store(true)

	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone {
			continue
		}
		patternIdx := project.Playback.Cursors[i].CurrentPattern
		requestCompile(project, i, patternIdx, 0)
		requestCompile(project, i, patternIdx, 1)
	}
}

func Pause(project *model.Project, lc midi.OutputLifecycle) {
	project.Playback.Playing.Store(false)
	lc.CloseAllOutputs()
}

// SetTempo updates the playback tempo in BPM, clamped to [20, 300]. Re-
// anchors T0 during play so the current tick count stays consistent.
func SetTempo(project *model.Project, bpm int) {
	if bpm < 20 {
		bpm = 20
	}
	if bpm > 300 {
		bpm = 300
	}
	newTempo := int64(bpm)
	oldTempo := project.Playback.Tempo.Swap(newTempo)
	project.Tempo = bpm
	if project.Playback.Playing.Load() && oldTempo != newTempo {
		newInterval := tickInterval(newTempo)
		project.Playback.T0Mu.Lock()
		project.Playback.T0 = time.Now().Add(-time.Duration(project.Playback.Tick.Load()) * newInterval)
		project.Playback.T0Mu.Unlock()
	}
}

// ResetCursors returns every track's cursor to the initial state:
// pattern 0 playing, nothing queued, slot 0, T0 at tick 0. Called after
// project load and from main.go at startup.
func ResetCursors(project *model.Project) {
	for i := range project.Playback.Cursors {
		project.Playback.Cursors[i] = model.Cursor{QueuedPattern: -1}
	}
}

// Playing reports whether the transport is advancing.
func Playing(project *model.Project) bool { return project.Playback.Playing.Load() }

// Tick returns the current global tick count.
func Tick(project *model.Project) int64 { return project.Playback.Tick.Load() }
