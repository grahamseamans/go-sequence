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
// whether a tick boundary has elapsed. It is small enough to keep jitter under
// a millisecond at 120 BPM (tick interval ~520µs at PPQ=960) while still
// letting the goroutine yield cleanly.
const tickSleep = 250 * time.Microsecond

// PlaybackEngine walks pre-compiled events with a cursor. Transport state
// (playing, t0, tick) lives here; per-track cursors live on project.Playback.
//
// Event storage no longer sits on PlaybackEngine: each pattern owns its own
// Machine[2]atomic.Pointer dual-buffer. The cursor's CurrentSlot names which
// one playback is reading; on wrap we store nil into the consumed slot, flip
// the cursor, and signal compile to refill that slot.
type PlaybackEngine struct {
	project   *model.Project
	out       midi.ToExternal
	compileCh chan<- model.CompileRequest

	// Transport.
	playing        atomic.Bool
	tick           atomic.Int64 // global tick count since last Play()
	lastGlobalTick atomic.Int64 // previous globalTick; computes delta per tick
	t0Mu           sync.RWMutex
	t0             time.Time    // wall-clock anchor; reads/writes guarded by t0Mu
	tempo          atomic.Int64 // BPM as int, read by tick goroutine each iteration
}

// NewPlaybackEngine constructs a PlaybackEngine bound to the given project,
// MIDI output, and compile-request channel. The engine does not start
// walking until Run is invoked.
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

// Run starts the tick goroutine. Blocks until ctx is done. Should be called
// from its own goroutine in main.
func (pe *PlaybackEngine) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Sleep a short fixed slice. We do NOT use time.Ticker because the
		// tick interval is derived from tempo, which can change mid-play.
		// A fixed short sleep keeps the loop responsive to tempo changes
		// while still tracking tick boundaries precisely via the wall-clock
		// anchor t0.
		time.Sleep(tickSleep)

		if !pe.playing.Load() {
			continue
		}

		interval := tickInterval(pe.tempo.Load())
		if interval <= 0 {
			// Defensive: bad tempo means no ticks advance (equivalent to pause).
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
// tempo. Guards against non-positive BPM (returns 0, which Run treats as
// "don't advance").
func tickInterval(bpm int64) time.Duration {
	if bpm <= 0 {
		return 0
	}
	// 60 seconds per minute * 1e9 ns/s / (bpm * PPQ) ticks per minute
	return time.Duration(60e9 / (bpm * int64(devices.PPQ)))
}

// processTick walks each track, reads its currently-playing pattern's
// Machine[CurrentSlot] buffer, and dispatches any events that fall in
// (LastPosition, newPos]. On wrap, splits the window at the pattern
// boundary, rotates slots / queued patterns, and dispatches the remainder
// against the NEW pattern's new slot.
func (pe *PlaybackEngine) processTick(globalTick int64) {
	delta := globalTick - pe.lastGlobalTick.Load()
	if delta > MaxDeltaTicks {
		delta = 1
	}
	if delta < 0 {
		// Defensive: shouldn't happen, but if the clock went backwards we
		// treat it as a no-op advance rather than a huge negative walk.
		delta = 0
	}

	for i := 0; i < 8; i++ {
		track := pe.project.Tracks[i]
		if track == nil || track.Type == model.DeviceNone {
			continue
		}

		cursor := &pe.project.Playback.Cursors[i]

		// Resolve the currently-playing pattern index + its Machine slot.
		curPatternIdx, machine := pe.resolveCurrent(track, i)
		if machine == nil {
			// Not yet compiled. Request this slot and stay silent this tick.
			// Playback resumes as soon as compile publishes the slot.
			pe.requestCompile(i, curPatternIdx, cursor.CurrentSlot)
			continue
		}

		plen := machine.Length
		if plen < 1 {
			plen = 1
		}

		// Clamp LastPosition in case the pattern length shrank under us
		// since the cursor was last written (user shortened pat.Length).
		if cursor.LastPosition < 0 || cursor.LastPosition >= plen {
			cursor.LastPosition = ((cursor.LastPosition % plen) + plen) % plen
		}

		if delta == 0 {
			continue
		}

		newPos := cursor.LastPosition + delta

		if newPos < plen {
			// No wrap — emit events in (LastPosition, newPos].
			for _, ev := range machine.Events {
				if ev.Tick > cursor.LastPosition && ev.Tick <= newPos {
					pe.dispatch(track, ev)
				}
			}
			cursor.LastPosition = newPos
			continue
		}

		// Wrap. First finish out the old slot: (LastPosition, plen).
		// Events sit at ticks in [0, plen) by compile's clamp, so the high
		// half of the split uses an open upper bound at plen.
		for _, ev := range machine.Events {
			if ev.Tick > cursor.LastPosition && ev.Tick < plen {
				pe.dispatch(track, ev)
			}
		}

		// Rotate slots / promote queued pattern. handleWrap advances the
		// cursor (T0, CurrentSlot) and signals compile to refill the
		// consumed slot (and, on queue promotion, both slots of the new
		// pattern as a safety net).
		pe.handleWrap(i, curPatternIdx, plen)

		// Dispatch the "leaked" events of the new iteration. If the delta
		// was so large we'd skip whole pattern iterations, we only fire ONE
		// loop's worth — MaxDeltaTicks already caps the common case.
		remainder := (newPos - plen) % plen
		if remainder < 0 {
			remainder = 0
		}

		// Re-resolve: the slot may have flipped, and (if queue promoted)
		// the pattern may have changed. The NEW slot may be nil (if
		// compile hasn't caught up yet) — that's fine, we just stay silent
		// for the remainder and pick up on the next tick.
		_, newMachine := pe.resolveCurrent(track, i)
		if newMachine != nil {
			for _, ev := range newMachine.Events {
				if ev.Tick >= 0 && ev.Tick <= remainder {
					pe.dispatch(track, ev)
				}
			}
		}
		cursor.LastPosition = remainder
	}

	pe.lastGlobalTick.Store(globalTick)
}

// resolveCurrent returns the track's currently-playing pattern index and the
// *CompiledPattern loaded from that pattern's Machine[cursor.CurrentSlot].
// Returns (0, nil) if the track has no device or no compiled pattern yet.
//
// Holds the track's RLock briefly; InputManager's writes briefly block us,
// which is fine (writes are sub-millisecond).
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

// dispatch sends a single timed event out through the configured MIDI output,
// respecting the track's mute flag.
func (pe *PlaybackEngine) dispatch(track *model.Track, ev devices.TimedEvent) {
	if track == nil {
		return
	}
	if track.Muted {
		return
	}
	// TODO(solo): honor Track.Solo across all tracks — if any track has
	// Solo==true, only solo'd tracks should sound.
	_ = pe.out.Send(track.PortName, track.Channel, ev.Event)
}

// handleWrap runs the per-track bookkeeping when the cursor crosses a pattern
// boundary:
//
//  1. Trash the just-consumed slot (Store nil).
//  2. If Schedule.Queued is set, promote it to Playing (under track.Lock()).
//  3. Advance cursor.T0 by the old pattern's length, flip CurrentSlot.
//  4. Signal compile to refill the trashed slot. If the promotion changed
//     the pattern, ALSO signal both slots of the new pattern as a safety
//     net (session.HandlePad ordinarily primes a queued pattern on click,
//     but a race where we wrap before that compile finishes is acceptable
//     if we re-signal here).
//
// oldPatternIdx is the pattern that was playing when the wrap fired.
// oldLength is its length in ticks — the caller passes it in because after
// we trash the slot we can't read it back.
func (pe *PlaybackEngine) handleWrap(trackIdx int, oldPatternIdx int, oldLength int64) {
	track := pe.project.Tracks[trackIdx]
	if track == nil {
		return
	}
	cursor := &pe.project.Playback.Cursors[trackIdx]
	trashedSlot := cursor.CurrentSlot

	// 1) Trash the slot we just consumed. atomic.Store — no lock needed.
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

	// 3) Advance cursor: flip slot, roll T0 forward by old pattern length.
	// T0 is bookkeeping for "when did the current pattern start"; the tick
	// loop doesn't read it today (it uses the global t0 + lastGlobalTick
	// delta instead), but the DESIGN.md §192 "t0 of the current pattern"
	// contract is kept so future features (pattern-relative position
	// display, tempo-sync handoff) have it on hand.
	interval := tickInterval(pe.tempo.Load())
	if interval > 0 && oldLength > 0 {
		cursor.T0 = cursor.T0.Add(time.Duration(oldLength) * interval)
	}
	cursor.CurrentSlot = 1 - trashedSlot

	// 4) Signal compile. Always refill the trashed slot of the old pattern
	// so if the user schedules back to it later the buffer's ready. On
	// queue promotion, also request both slots of the new pattern — the
	// session-queue edit path usually primed them already, but re-requesting
	// is cheap insurance against a compile-channel drop.
	pe.requestCompile(trackIdx, oldPatternIdx, trashedSlot)
	if newPatternIdx != oldPatternIdx {
		pe.requestCompile(trackIdx, newPatternIdx, 0)
		pe.requestCompile(trackIdx, newPatternIdx, 1)
	}
}

// requestCompile is a non-blocking send on compileCh, logging (and dropping)
// when the buffer is full. The compiler is about to drain it anyway, and the
// next wrap re-signals the same slot.
func (pe *PlaybackEngine) requestCompile(trackIdx, patternIdx, slot int) {
	select {
	case pe.compileCh <- model.CompileRequest{Track: trackIdx, Pattern: patternIdx, Slot: slot}:
	default:
		debug.Log("playback", "compileCh full, dropped refill for track=%d pattern=%d slot=%d",
			trackIdx, patternIdx, slot)
	}
}

// trashSlot stores nil into pattern[patternIdx].Machine[slot] for the given
// track's device type. No-op if the track or pattern is missing.
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

// Play starts the transport. Resets the tick counter, anchors t0 to now,
// zeros every cursor (T0=now, CurrentSlot=0, LastPosition=0), and signals
// compile to prime BOTH slots of each track's currently-playing pattern.
// Priming on Play means a fresh NewProject or a post-Load project can hit
// Play and have audible output on the first wrap without waiting for an
// edit to trigger compile.
func (pe *PlaybackEngine) Play() {
	now := time.Now()
	pe.t0Mu.Lock()
	pe.t0 = now
	pe.t0Mu.Unlock()
	pe.tick.Store(0)
	pe.lastGlobalTick.Store(0)
	for i := range pe.project.Playback.Cursors {
		pe.project.Playback.Cursors[i] = model.Cursor{T0: now, CurrentSlot: 0, LastPosition: 0}
	}
	pe.playing.Store(true)

	// Prime each track's currently-playing pattern's Machine slots. Done
	// AFTER setting playing=true so the tick goroutine that sees the first
	// machine-nil slot has already had the compile request queued up.
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

// Pause halts the transport. Cursors and tick counter are preserved.
func (pe *PlaybackEngine) Pause() {
	pe.playing.Store(false)
}

// SetTempo updates the playback tempo in BPM, clamped to [20, 300]. When
// called during play, t0 is re-anchored so the current tick count stays
// consistent under the new interval.
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

// ResetCursors zeroes every track's cursor. Called after project load so
// the new project's cursors start from a known state. Does not touch
// Machine slots — the caller (InputManager) marks patterns dirty separately.
func (pe *PlaybackEngine) ResetCursors() {
	for i := range pe.project.Playback.Cursors {
		pe.project.Playback.Cursors[i] = model.Cursor{}
	}
}

// Playing reports whether the transport is currently advancing.
func (pe *PlaybackEngine) Playing() bool { return pe.playing.Load() }

// Tick returns the current global tick count since the last Play().
func (pe *PlaybackEngine) Tick() int64 { return pe.tick.Load() }
