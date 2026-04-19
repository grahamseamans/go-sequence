package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
)

// MaxDeltaTicks clamps a tick delta when the wall clock jumps (laptop sleep,
// host hiccup, etc.). Past this many ticks we treat it as a 1-tick advance
// instead of trying to replay thousands of events at once.
const MaxDeltaTicks = int64(model.PPQ) // 1 quarter note

// tickSleep is the granularity at which the tick goroutine wakes up to check
// whether a tick boundary has elapsed. It is small enough to keep jitter under
// a millisecond at 120 BPM (tick interval ~520µs at PPQ=960) while still
// letting the goroutine yield cleanly.
const tickSleep = 250 * time.Microsecond

// PlaybackEngine walks pre-compiled events with a cursor. Transport state
// (playing, t0, tick) lives here, NOT in model.Project.
type PlaybackEngine struct {
	project   *model.Project
	out       midi.ToExternal
	compileCh chan<- int // signals compiler that a track needs recompile

	// Double-buffered per-track events. Playback reads current; compile
	// writes next. At wrap boundary, current <- next (atomic swap).
	currentEvents [8]atomic.Pointer[model.CompiledPattern]
	nextEvents    [8]atomic.Pointer[model.CompiledPattern]

	// Per-track edit version. InputManager bumps on mutation. Compiler
	// stamps into each CompiledPattern it produces. Used by the wrap
	// handler to detect stale current events.
	editCounters [8]atomic.Uint64

	// Per-track cursor. Owned by the tick goroutine.
	cursors [8]cursor

	// Transport.
	playing        atomic.Bool
	tick           atomic.Int64 // global tick count since last Play()
	lastGlobalTick atomic.Int64 // previous globalTick; computes delta per tick
	t0Mu           sync.RWMutex
	t0             time.Time    // wall-clock anchor; reads/writes guarded by t0Mu
	tempo          atomic.Int64 // BPM as int, read by tick goroutine each iteration
}

// cursor is the per-track playback position within its currently-playing
// pattern, in ticks. Owned by the tick goroutine; no locking needed.
type cursor struct {
	Tick int64
}

// NewPlaybackEngine constructs a PlaybackEngine bound to the given project,
// MIDI output, and compile-request channel. The engine does not start
// walking until Run is invoked.
func NewPlaybackEngine(project *model.Project, out midi.ToExternal, compileCh chan<- int) *PlaybackEngine {
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
	return time.Duration(60e9 / (bpm * int64(model.PPQ)))
}

// processTick walks each track's current events and dispatches anything in
// (cursor, newCursor], handling pattern wrap atomically per DESIGN 4.1.
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

		buf := pe.currentEvents[i].Load()
		if buf == nil {
			// Not yet compiled; silently skip this tick.
			continue
		}

		plen := buf.Length
		if plen < 1 {
			plen = 1 // defense
		}

		c := &pe.cursors[i]
		c.Tick %= plen // handle prior length changes

		if delta == 0 {
			// No advance — nothing to dispatch and no wrap.
			continue
		}

		newTick := (c.Tick + delta) % plen
		wrapped := newTick < c.Tick || delta >= plen

		// Fire events in (c.Tick, newTick], splitting across the wrap boundary.
		// Compiler sorts events ascending by tick.
		for _, ev := range buf.Events {
			fire := false
			if wrapped {
				if ev.Tick > c.Tick || ev.Tick <= newTick {
					fire = true
				}
			} else {
				if ev.Tick > c.Tick && ev.Tick <= newTick {
					fire = true
				}
			}
			if fire {
				pe.dispatch(i, ev)
			}
		}

		c.Tick = newTick

		if wrapped {
			pe.handleWrap(i)
		}
	}

	pe.lastGlobalTick.Store(globalTick)
}

// dispatch sends a single timed event out through the configured MIDI output,
// respecting the track's mute flag. Errors from out.Send are ignored for now
// (mock outputs and OS send errors are both non-fatal in the tick path).
func (pe *PlaybackEngine) dispatch(trackIdx int, ev model.TimedEvent) {
	track := pe.project.Tracks[trackIdx]
	if track == nil {
		return
	}
	if track.Muted {
		return
	}
	// TODO(step5): honor Track.Solo across all tracks — if any track has
	// Solo==true, only solo'd tracks should sound. Requires a project-wide
	// solo check; deferring to a later UX pass.
	_ = pe.out.Send(track.PortName, track.Channel, ev.Event)
}

// handleWrap runs the per-track bookkeeping that happens when the cursor
// crosses the pattern boundary: advance any queued schedule entry, promote
// next_events into current_events, and request a fresh next iteration.
func (pe *PlaybackEngine) handleWrap(trackIdx int) {
	track := pe.project.Tracks[trackIdx]
	if track == nil {
		return
	}

	// 1) Schedule advance: promote Queued -> Playing if something is queued.
	track.Lock()
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum != nil && track.Drum.Schedule.Queued != -1 {
			track.Drum.Schedule.Playing = track.Drum.Schedule.Queued
			track.Drum.Schedule.Queued = -1
		}
	case model.DevicePiano:
		if track.Piano != nil && track.Piano.Schedule.Queued != -1 {
			track.Piano.Schedule.Playing = track.Piano.Schedule.Queued
			track.Piano.Schedule.Queued = -1
		}
	case model.DeviceMetropolix:
		if track.Metropolix != nil && track.Metropolix.Schedule.Queued != -1 {
			track.Metropolix.Schedule.Playing = track.Metropolix.Schedule.Queued
			track.Metropolix.Schedule.Queued = -1
		}
	}
	track.Unlock()

	// 2) Atomic swap: promote next_events to current_events.
	currentBuf := pe.currentEvents[trackIdx].Load()
	nextBuf := pe.nextEvents[trackIdx].Load()
	staleCurrent := currentBuf != nil && currentBuf.EditCounter < pe.editCounters[trackIdx].Load()

	switch {
	case nextBuf != nil:
		pe.currentEvents[trackIdx].Store(nextBuf)
		pe.nextEvents[trackIdx].Store(nil)
	case staleCurrent:
		// No next ready and current is stale. Log and continue with stale;
		// Step 4b will introduce a sync-compile path we can call here.
		// TODO(step4b): add syncCompile(trackIdx) fallback once compile.go exists.
		debug.Log("playback", "wrap: next not ready, current stale, track=%d", trackIdx)
	default:
		// next not ready but current is still valid. Reuse for another loop.
		debug.Log("playback", "wrap: next not ready, reusing current, track=%d", trackIdx)
	}

	// 3) Request a fresh next iteration. Non-blocking; compileCh is buffered.
	select {
	case pe.compileCh <- trackIdx:
	default:
		// channel full (should never happen with buffer >= NumTracks + headroom).
		debug.Log("playback", "compileCh full, dropped request for track=%d", trackIdx)
	}
}

// Play starts the transport. Resets the tick counter and anchors t0 to now.
func (pe *PlaybackEngine) Play() {
	pe.t0Mu.Lock()
	pe.t0 = time.Now()
	pe.t0Mu.Unlock()
	pe.tick.Store(0)
	pe.lastGlobalTick.Store(0)
	pe.playing.Store(true)
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

// ResetCursors zeroes all cursors. Called after project load.
func (pe *PlaybackEngine) ResetCursors() {
	for i := range pe.cursors {
		pe.cursors[i].Tick = 0
	}
}

// NextEvents exposes the next_events array to the compiler goroutine, which
// is the sole writer. Playback reads it (and clears it on promotion) in the
// wrap handler.
func (pe *PlaybackEngine) NextEvents() *[8]atomic.Pointer[model.CompiledPattern] {
	return &pe.nextEvents
}

// EditCounters exposes the per-track edit counters. InputManager bumps on
// mutation; Compiler reads to stamp CompiledPattern.EditCounter.
func (pe *PlaybackEngine) EditCounters() *[8]atomic.Uint64 {
	return &pe.editCounters
}

// Playing reports whether the transport is currently advancing.
func (pe *PlaybackEngine) Playing() bool { return pe.playing.Load() }

// Tick returns the current global tick count since the last Play().
func (pe *PlaybackEngine) Tick() int64 { return pe.tick.Load() }
