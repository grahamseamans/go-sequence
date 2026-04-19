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

// Compiler consumes CompileRequest on compileCh and fills the named
// Machine slot with a fresh CompiledPattern. Runs as its own goroutine so
// playback never blocks on compile.
//
// Contract: each request targets one (Track, Pattern, Slot) triple and
// unconditionally overwrites that slot. Callers that need both slots
// refilled (an edit, initial bootstrap, a wrap-promoted pattern) send two
// requests. The "fresh rolls every loop" invariant (DESIGN §3.10) falls
// out of every request getting its own seed.
type Compiler struct {
	project   *model.Project
	compileCh <-chan model.CompileRequest

	// loopCounter mixes into the seed so two compiles in the same nanosecond
	// for the same (track, pattern, slot) still get distinct seeds.
	loopCounter atomic.Uint64
}

// NewCompiler wires the compiler up to the shared compileCh. The sender end
// of compileCh is held by InputManager (edits, queue) and PlaybackEngine
// (wrap promotion, Play bootstrap) — both signal "please render this one
// slot".
func NewCompiler(project *model.Project, compileCh <-chan model.CompileRequest) *Compiler {
	return &Compiler{project: project, compileCh: compileCh}
}

// Run consumes compileCh until ctx is done or the channel is closed.
// Should be called in its own goroutine.
func (c *Compiler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-c.compileCh:
			if !ok {
				return
			}
			c.compileSlot(req)
		}
	}
}

// compileSlot renders one Machine slot for one pattern on one track. Holds
// the track's RLock for the duration — InputManager's writes briefly block
// compiles, which is fine because writes are sub-millisecond.
func (c *Compiler) compileSlot(req model.CompileRequest) {
	if req.Track < 0 || req.Track >= 8 {
		return
	}
	if req.Pattern < 0 || req.Pattern >= devices.NumPatterns {
		return
	}
	if req.Slot < 0 || req.Slot > 1 {
		return
	}
	track := c.project.Tracks[req.Track]
	if track == nil || track.Type == model.DeviceNone {
		return
	}

	track.RLock()
	defer track.RUnlock()

	// Per-slot fresh seed. Mixes wall clock, the (track, pattern, slot)
	// triple, and a monotonic counter so simultaneous compiles on the same
	// nanosecond get distinct rolls. Odd-looking mix of XOR and multiply-
	// by-small-primes is intentional: it keeps the slot/track component in
	// the low bits where the rng consumes them first.
	seed := uint64(time.Now().UnixNano()) ^
		uint64(req.Track)*31 ^
		uint64(req.Pattern)*7 ^
		uint64(req.Slot) ^
		c.loopCounter.Add(1)

	var cp devices.CompiledPattern
	var machine *[2]atomic.Pointer[devices.CompiledPattern]

	switch track.Type {
	case model.DeviceDrum:
		if track.Drum == nil {
			return
		}
		pat := track.Drum.Patterns[req.Pattern]
		if pat == nil {
			return
		}
		cp = drum.Compile(pat, devices.GetKit(track.Kit), seed)
		machine = &pat.Machine
	case model.DevicePiano:
		if track.Piano == nil {
			return
		}
		pat := track.Piano.Patterns[req.Pattern]
		if pat == nil {
			return
		}
		cp = piano.Compile(pat, seed)
		machine = &pat.Machine
	case model.DeviceMetropolix:
		if track.Metropolix == nil {
			return
		}
		pat := track.Metropolix.Patterns[req.Pattern]
		if pat == nil {
			return
		}
		cp = metropolix.Compile(pat, seed)
		machine = &pat.Machine
	default:
		return
	}

	machine[req.Slot].Store(&cp)

	debug.Log("compile", "track=%d pattern=%d slot=%d seed=%d events=%d len=%d",
		req.Track, req.Pattern, req.Slot, seed, len(cp.Events), cp.Length)
}
