package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// MaxDeltaTicks clamps a tick delta when the wall clock jumps (laptop sleep,
// host hiccup, etc.). Past this many ticks we treat it as a 1-tick advance
// instead of trying to replay thousands of events at once.
const MaxDeltaTicks = int64(devices.PPQ) // 1 quarter note

// tickSleep is the granularity at which the tick goroutine wakes up to check
// whether a tick boundary has elapsed. Small enough to keep jitter sub-ms at
// 120 BPM (tick interval ~520µs at PPQ=960) while letting the goroutine yield.
const tickSleep = 250 * time.Microsecond

// PlaybackEngine is the transport + per-track cursor walker. Event storage
// lives on each pattern's Machine[2]atomic.Pointer dual-buffer; PlaybackEngine
// holds nothing except the transport anchor.
//
// Per DESIGN, the playhead itself is NOT stored. On each tick it is
// recomputed as (globalTick - cursor.T0Tick), giving the pattern-relative
// position without needing a separately-advanced cursor.
type PlaybackEngine struct {
	project   *model.Project
	out       midi.ToExternal
	compileCh chan<- model.CompileRequest

	// Transport.
	playing        atomic.Bool
	tick           atomic.Int64 // global tick count (latest value passed to processTick)
	lastGlobalTick atomic.Int64 // previous globalTick; paired with per-track T0Tick to compute lastPos
	t0Mu           sync.RWMutex
	t0             time.Time    // wall-clock anchor; reads/writes guarded by t0Mu
	tempo          atomic.Int64 // BPM as int, read by tick goroutine each iteration
}

// NewPlaybackEngine constructs a PlaybackEngine bound to the given project,
// MIDI output, and compile-request channel.
func NewPlaybackEngine(project *model.Project, out midi.ToExternal, compileCh chan<- model.CompileRequest) *PlaybackEngine {
	pe := &PlaybackEngine{
		project:   project,
		out:       out,
		compileCh: compileCh,
	}
	pe.tempo.Store(int64(project.Tempo))
	if pe.tempo.Load() < 20 {
		pe.tempo.Store(120)
	}
	return pe
}

// Run starts the tick goroutine. Blocks until ctx is done.
func (pe *PlaybackEngine) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		time.Sleep(tickSleep)

		if !pe.playing.Load() {
			continue
		}

		interval := tickInterval(pe.tempo.Load())
		if interval <= 0 {
			continue
		}

		pe.t0Mu.RLock()
		elapsed := time.Since(pe.t0)
		pe.t0Mu.RUnlock()

		globalTick := int64(elapsed / interval)
		if globalTick > pe.tick.Load() {
			pe.processTick(globalTick)
			pe.tick.Store(globalTick)
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
// lastPos and newPos are both computed from globalTick / lastGlobalTick and
// the per-track T0Tick — the playhead itself is never stored.
//
// On wrap (newPos crosses pattern.Length), the window is split: the tail of
// the old pattern/slot is emitted, handleWrap rotates slots and promotes any
// queued pattern, and the remainder is emitted against the new slot.
func (pe *PlaybackEngine) processTick(globalTick int64) {
	lastGlobal := pe.lastGlobalTick.Load()
	delta := globalTick - lastGlobal
	if delta > MaxDeltaTicks {
		// Clock jumped; clamp to one tick. Per-track math still works
		// because we synthesize lastGlobal = globalTick - delta below.
		delta = 1
		lastGlobal = globalTick - 1
	}
	if delta < 0 {
		delta = 0
	}

	for i := 0; i < 8; i++ {
		track := pe.project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone {
			continue
		}

		cursor := &pe.project.Playback.Cursors[i]

		curPatternIdx, machine := pe.resolveCurrent(track, i)
		if machine == nil {
			// Slot isn't compiled yet. Request it; stay silent this tick.
			pe.requestCompile(i, curPatternIdx, cursor.CurrentSlot)
			continue
		}
		plen := machine.Length
		if plen < 1 {
			plen = 1
		}

		// Pattern-relative positions derived from globalTick + T0Tick.
		// lastPos can be negative on the first tick after Play (we init
		// lastGlobalTick = -1 so tick 0 falls in the (−1, 0] window).
		lastPos := lastGlobal - cursor.T0Tick
		newPos := globalTick - cursor.T0Tick

		if delta == 0 {
			continue
		}

		if newPos < plen {
			// No wrap — fire events in (lastPos, newPos].
			for _, ev := range machine.Events {
				if ev.Tick > lastPos && ev.Tick <= newPos {
					pe.dispatch(track, ev)
				}
			}
			continue
		}

		// Wrap. Fire tail of old pattern: (lastPos, plen).
		// CompiledPattern.Events are at ticks in [0, plen), so the strict
		// upper bound at plen is the natural end-of-iteration boundary.
		for _, ev := range machine.Events {
			if ev.Tick > lastPos && ev.Tick < plen {
				pe.dispatch(track, ev)
			}
		}

		pe.handleWrap(i, curPatternIdx, plen)

		// After handleWrap: cursor.T0Tick has advanced by plen, CurrentSlot
		// has flipped, and (possibly) Schedule.Playing was promoted. The new
		// pattern's Machine[newSlot] may be nil (compile hasn't caught up);
		// if so we just stay silent for this tick's remainder.
		remainder := newPos - plen
		if remainder < 0 {
			remainder = 0
		}
		_, newMachine := pe.resolveCurrent(track, i)
		if newMachine != nil {
			// First tick of new pattern: emit events at [0, remainder].
			// The lower bound is INCLUSIVE so tick 0 of the new pattern
			// fires at the very moment of the wrap.
			for _, ev := range newMachine.Events {
				if ev.Tick >= 0 && ev.Tick <= remainder {
					pe.dispatch(track, ev)
				}
			}
		}
	}

	pe.lastGlobalTick.Store(globalTick)
}

// resolveCurrent reads the track's currently-playing pattern index and the
// *CompiledPattern loaded from that pattern's Machine[cursor.CurrentSlot].
// Returns (0, nil) if the track has no device or no compiled pattern yet.
//
// Takes the track's RLock briefly.
func (pe *PlaybackEngine) resolveCurrent(track *model.Track, trackIdx int) (int, *devices.CompiledPattern) {
	track.RLock()
	defer track.RUnlock()

	cursor := &pe.project.Playback.Cursors[trackIdx]
	slot := cursor.CurrentSlot
	if slot < 0 || slot > 1 {
		slot = 0
		cursor.CurrentSlot = 0
	}

	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			return 0, nil
		}
		idx := track.Drum.Schedule.Playing
		if idx < 0 || idx >= len(track.Drum.Patterns) {
			return 0, nil
		}
		pat := track.Drum.Patterns[idx]
		if pat == nil {
			return idx, nil
		}
		return idx, pat.Machine[slot].Load()
	case model.DevicePiano:
		if track.Piano == nil {
			return 0, nil
		}
		idx := track.Piano.Schedule.Playing
		if idx < 0 || idx >= len(track.Piano.Patterns) {
			return 0, nil
		}
		pat := track.Piano.Patterns[idx]
		if pat == nil {
			return idx, nil
		}
		return idx, pat.Machine[slot].Load()
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			return 0, nil
		}
		idx := track.Metropolix.Schedule.Playing
		if idx < 0 || idx >= len(track.Metropolix.Patterns) {
			return 0, nil
		}
		pat := track.Metropolix.Patterns[idx]
		if pat == nil {
			return idx, nil
		}
		return idx, pat.Machine[slot].Load()
	}
	return 0, nil
}

// dispatch sends a single event through the MIDI output, honoring Muted.
func (pe *PlaybackEngine) dispatch(track *model.Track, ev devices.TimedEvent) {
	if track == nil || track.Muted {
		return
	}
	// TODO(solo): honor Track.Solo across all tracks.
	_ = pe.out.Send(track.PortName, track.Channel, ev.Event)
}

// handleWrap runs the per-track bookkeeping when the cursor crosses a
// pattern boundary:
//
//  1. Trash the just-consumed slot (Store nil).
//  2. If Schedule.Queued is set, promote it to Playing (under track.Lock()).
//  3. Advance cursor.T0Tick by oldLength, flip CurrentSlot.
//  4. Signal compile to refill the trashed slot (+ both slots of the new
//     pattern on queue promotion, as a safety net).
func (pe *PlaybackEngine) handleWrap(trackIdx int, oldPatternIdx int, oldLength int64) {
	track := pe.project.Tracks[trackIdx]
	if track == nil {
		return
	}
	cursor := &pe.project.Playback.Cursors[trackIdx]
	trashedSlot := cursor.CurrentSlot

	// 1) Trash the consumed slot.
	pe.trashSlot(track, oldPatternIdx, trashedSlot)

	// 2) Schedule advance under write lock.
	track.Lock()
	newPatternIdx := oldPatternIdx
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil && track.Drum.Schedule.Queued != -1 {
			track.Drum.Schedule.Playing = track.Drum.Schedule.Queued
			track.Drum.Schedule.Queued = -1
			newPatternIdx = track.Drum.Schedule.Playing
		}
	case model.DevicePiano:
		if track.Piano != nil && track.Piano.Schedule.Queued != -1 {
			track.Piano.Schedule.Playing = track.Piano.Schedule.Queued
			track.Piano.Schedule.Queued = -1
			newPatternIdx = track.Piano.Schedule.Playing
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil && track.Metropolix.Schedule.Queued != -1 {
			track.Metropolix.Schedule.Playing = track.Metropolix.Schedule.Queued
			track.Metropolix.Schedule.Queued = -1
			newPatternIdx = track.Metropolix.Schedule.Playing
		}
	}
	track.Unlock()

	// 3) Advance cursor: flip slot, roll T0Tick forward by old length.
	cursor.T0Tick += oldLength
	cursor.CurrentSlot = 1 - trashedSlot

	// 4) Request compile refills.
	pe.requestCompile(trackIdx, oldPatternIdx, trashedSlot)
	if newPatternIdx != oldPatternIdx {
		pe.requestCompile(trackIdx, newPatternIdx, 0)
		pe.requestCompile(trackIdx, newPatternIdx, 1)
	}
}

// requestCompile is a non-blocking send on compileCh.
func (pe *PlaybackEngine) requestCompile(trackIdx, patternIdx, slot int) {
	select {
	case pe.compileCh <- model.CompileRequest{Track: trackIdx, Pattern: patternIdx, Slot: slot}:
	default:
		debug.Log("playback", "compileCh full, dropped refill for track=%d pattern=%d slot=%d",
			trackIdx, patternIdx, slot)
	}
}

// trashSlot stores nil into pattern[patternIdx].Machine[slot].
func (pe *PlaybackEngine) trashSlot(track *model.Track, patternIdx, slot int) {
	if track == nil {
		return
	}
	if slot < 0 || slot > 1 || patternIdx < 0 || patternIdx >= devices.NumPatterns {
		return
	}
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil && track.Drum.Patterns[patternIdx] != nil {
			track.Drum.Patterns[patternIdx].Machine[slot].Store(nil)
		}
	case model.DevicePiano:
		if track.Piano != nil && track.Piano.Patterns[patternIdx] != nil {
			track.Piano.Patterns[patternIdx].Machine[slot].Store(nil)
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil && track.Metropolix.Patterns[patternIdx] != nil {
			track.Metropolix.Patterns[patternIdx].Machine[slot].Store(nil)
		}
	}
}

// Play starts the transport. Anchors wall-clock t0, resets the tick counter,
// zeros every cursor (T0Tick=0, CurrentSlot=0), and primes both Machine slots
// of each track's currently-playing pattern.
//
// pe.tick and pe.lastGlobalTick are both set to -1 so the FIRST call to
// processTick enters at globalTick=0, giving a (-1, 0] walk window that
// includes events at tick 0 (the downbeat).
func (pe *PlaybackEngine) Play() {
	now := time.Now()
	pe.t0Mu.Lock()
	pe.t0 = now
	pe.t0Mu.Unlock()
	pe.tick.Store(-1)
	pe.lastGlobalTick.Store(-1)
	for i := range pe.project.Playback.Cursors {
		pe.project.Playback.Cursors[i] = model.Cursor{T0Tick: 0, CurrentSlot: 0}
	}
	pe.playing.Store(true)

	// Prime each active track's current pattern — both slots.
	for i := 0; i < 8; i++ {
		track := pe.project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone {
			continue
		}
		var patternIdx int
		track.RLock()
		switch track.Type {
		case model.DeviceDrum:
			if track.Drum != nil {
				patternIdx = track.Drum.Schedule.Playing
			}
		case model.DevicePiano:
			if track.Piano != nil {
				patternIdx = track.Piano.Schedule.Playing
			}
		case model.DeviceMetropolix:
			if track.Metropolix != nil {
				patternIdx = track.Metropolix.Schedule.Playing
			}
		}
		track.RUnlock()
		pe.requestCompile(i, patternIdx, 0)
		pe.requestCompile(i, patternIdx, 1)
	}
}

// Pause halts the transport. Cursors are preserved.
func (pe *PlaybackEngine) Pause() {
	pe.playing.Store(false)
}

// SetTempo updates the playback tempo in BPM, clamped to [20, 300]. Re-
// anchors t0 during play so the current tick count stays consistent.
func (pe *PlaybackEngine) SetTempo(bpm int) {
	if bpm < 20 {
		bpm = 20
	}
	if bpm > 300 {
		bpm = 300
	}
	newTempo := int64(bpm)
	oldTempo := pe.tempo.Swap(newTempo)
	if pe.playing.Load() && oldTempo != newTempo {
		newInterval := tickInterval(newTempo)
		pe.t0Mu.Lock()
		pe.t0 = time.Now().Add(-time.Duration(pe.tick.Load()) * newInterval)
		pe.t0Mu.Unlock()
	}
}

// ResetCursors zeroes every track's cursor. Called after project load.
func (pe *PlaybackEngine) ResetCursors() {
	for i := range pe.project.Playback.Cursors {
		pe.project.Playback.Cursors[i] = model.Cursor{}
	}
}

// Playing reports whether the transport is currently advancing.
func (pe *PlaybackEngine) Playing() bool { return pe.playing.Load() }

// Tick returns the current global tick count.
func (pe *PlaybackEngine) Tick() int64 { return pe.tick.Load() }
