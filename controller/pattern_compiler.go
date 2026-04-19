package controller

import (
	"context"
	"sync/atomic"
	"time"

	"go-sequence/controller/devices/drum"
	"go-sequence/controller/devices/metropolix"
	"go-sequence/controller/devices/piano"
	"go-sequence/debug"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Compiler consumes trackIdx on compileCh, renders the track's pattern via
// the appropriate device's Compile method with a fresh seed, and stores the
// result into nextEvents for the playback engine to swap in at the next wrap.
//
// Runs as its own goroutine so playback never blocks on compile. InputManager
// and PlaybackEngine both send trackIdx on compileCh to request a recompile;
// Compiler serializes those requests and publishes results via atomic stores
// into the double-buffer's "next" slot.
type Compiler struct {
	project      *model.Project
	nextEvents   *[8]atomic.Pointer[devices.CompiledPattern]
	editCounters *[8]atomic.Uint64
	compileCh    <-chan int

	// loopCounter mixes into the seed so two compiles in the same nanosecond
	// for the same track still get distinct seeds. Per DESIGN §7.
	loopCounter atomic.Uint64
}

// NewCompiler wires the compiler up to PlaybackEngine's double-buffer
// pointers and the shared compileCh. The sender end of compileCh is held by
// InputManager (edits, queue) and PlaybackEngine (wrap promotion) — both
// signal "please compile track i".
func NewCompiler(
	project *model.Project,
	pe *PlaybackEngine,
	compileCh <-chan int,
) *Compiler {
	return &Compiler{
		project:      project,
		nextEvents:   pe.NextEvents(),
		editCounters: pe.EditCounters(),
		compileCh:    compileCh,
	}
}

// Run consumes compileCh until ctx is done or the channel is closed.
// Should be called in its own goroutine.
func (c *Compiler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case trackIdx, ok := <-c.compileCh:
			if !ok {
				return
			}
			if trackIdx < 0 || trackIdx >= 8 {
				// Defensive: out-of-range index. Ignore rather than panic.
				continue
			}
			c.compileTrack(trackIdx)
		}
	}
}

// compileTrack renders the given track's currently-queued pattern spec into
// a fresh CompiledPattern and publishes it into nextEvents[trackIdx]. The
// editCounter observed at the start of compile is stamped onto the result so
// the wrap handler can detect stale events.
func (c *Compiler) compileTrack(trackIdx int) {
	track := c.project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone {
		return
	}

	// Record the editCounter BEFORE compile. If it advances during compile,
	// the resulting pattern will already be stale on publish — the next
	// compile signal should already be queued (InputManager increments then
	// sends on compileCh). We stamp the observed counter so playback's wrap
	// handler can detect the stale-ness either way.
	edit := c.editCounters[trackIdx].Load()

	// Fresh seed per compile. Per DESIGN §7: nanosecond clock XOR'd with
	// trackIdx and a monotonic counter so two compiles at the same wall-clock
	// nanosecond still get distinct seeds.
	seed := uint64(time.Now().UnixNano()) ^ uint64(trackIdx) ^ c.loopCounter.Add(1)

	track.RLock()
	var cp devices.CompiledPattern
	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			track.RUnlock()
			return
		}
		kit := devices.GetKit(track.Kit)
		cp = drum.Compile(track.Drum, kit, seed)
	case model.DevicePiano:
		if track.Piano == nil {
			track.RUnlock()
			return
		}
		cp = piano.Compile(track.Piano, seed)
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			track.RUnlock()
			return
		}
		cp = metropolix.Compile(track.Metropolix, seed)
	default:
		track.RUnlock()
		return
	}
	track.RUnlock()

	cp.EditCounter = edit
	c.nextEvents[trackIdx].Store(&cp)

	debug.Log("compile", "track=%d seed=%d events=%d len=%d edit=%d",
		trackIdx, seed, len(cp.Events), cp.Length, edit)
}
