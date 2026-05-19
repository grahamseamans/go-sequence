package controller

import (
	"context"
	"sync/atomic"
	"time"

	"go-sequence/controller/devices/drum"
	"go-sequence/controller/devices/looper"
	"go-sequence/debug"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// RunPatternCompiler consumes project.Compile.Channel and fills the named
// Machine slot with a fresh CompiledPattern. Start it as its own goroutine;
// blocks until ctx is done (or the channel is closed).
//
// Contract: each request targets one (Track, Pattern, Slot) triple and
// unconditionally overwrites that slot. Callers that need both slots
// refilled (an edit, initial bootstrap, a wrap-promoted pattern) send two
// requests. The "fresh rolls every loop" invariant (DESIGN §3.10) falls
// out of every request getting its own seed.
func RunPatternCompiler(ctx context.Context, project *model.Project) {
	ch := project.Compile.Channel
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-ch:
			if !ok {
				return
			}
			compileSlot(project, req)
		}
	}
}

// compileSlot renders one Machine slot for one pattern on one track. Holds
// the track's RLock for the duration.
func compileSlot(project *model.Project, req model.CompileRequest) {
	if req.Track < 0 || req.Track >= 8 {
		return
	}
	if req.Pattern < 0 || req.Pattern >= devices.NumPatterns {
		return
	}
	if req.Slot < 0 || req.Slot > 1 {
		return
	}
	track := project.Tracks[req.Track]
	if track == nil || track.Type == model.DeviceNone {
		return
	}

	track.RLock()
	defer track.RUnlock()

	// Per-slot fresh seed. Mixes wall clock, the (track, pattern, slot)
	// triple, and the project's monotonic loop counter so simultaneous
	// compiles get distinct rolls.
	seed := uint64(time.Now().UnixNano()) ^
		uint64(req.Track)*31 ^
		uint64(req.Pattern)*7 ^
		uint64(req.Slot) ^
		project.Compile.LoopCounter.Add(1)

	var cp devices.CompiledPattern
	var machine *[2]atomic.Pointer[devices.CompiledPattern]

	switch d := track.Device.(type) {
	case *devices.Drum:
		if d == nil {
			return
		}
		pat := d.Patterns[req.Pattern]
		if pat == nil {
			return
		}
		cp = drum.Compile(pat, devices.GetKit(track.Kit), seed)
		machine = &pat.Machine
	case *devices.Looper:
		if d == nil {
			return
		}
		pat := d.Patterns[req.Pattern]
		if pat == nil {
			return
		}
		cp = looper.Compile(pat, seed)
		machine = &pat.Machine
	default:
		return
	}

	machine[req.Slot].Store(&cp)

	debug.Log("compile", "track=%d pattern=%d slot=%d seed=%d events=%d len=%d",
		req.Track, req.Pattern, req.Slot, seed, len(cp.Events), cp.Length)
}
