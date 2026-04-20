// Package keyboard is the external-MIDI-keyboard view surface. Per DESIGN
// (§3.7 and the vibe in lines 130-150): the keyboard is ONE input, routed
// to ONE track at a time. The route is stored on model.UIState.KeyboardRoute
// — an index into project.Tracks[].
//
// RunRoutine subscribes to the routed track's (PortName, Channel) and
// dispatches incoming MIDI events to the device-specific handler
// (drumMIDI / pianoMIDI / metropolixMIDI). When the route changes, the
// goroutine re-subscribes; the old subscription's goroutine shuts down via
// its per-subscription context.
package keyboard

import (
	"context"

	"go-sequence/controller"
	"go-sequence/debug"
	"go-sequence/midi"
	"go-sequence/model"
)

// RunRoutine is the keyboard goroutine entry point. Subscribes to the
// currently-routed track's MIDI port, dispatches events until the route
// changes or ctx is canceled, then loops to re-subscribe with the new
// route. Blocks until ctx is done.
//
// Route changes are picked up via project.UI.KeyboardRouteChanged — writers
// call model.SetKeyboardRoute which non-blocking-sends on that channel.
func RunRoutine(ctx context.Context, project *model.Project, out midi.ToExternal, in midi.FromExternal) {
	// subCancel is nil until the first route takes effect; after that, every
	// overwrite of subCancel pairs with exactly one cancel() call.
	var subCancel context.CancelFunc = func() {}
	defer func() { subCancel() }()

	curRoute := -1
	apply := func() {
		desired := project.UI.KeyboardRoute
		if desired == curRoute {
			return
		}
		subCancel()
		curRoute = desired
		subCtx, cancel := context.WithCancel(ctx)
		subCancel = cancel
		startSubscription(subCtx, project, out, in, desired)
	}

	// Prime the initial subscription.
	apply()

	for {
		select {
		case <-ctx.Done():
			return
		case <-project.UI.KeyboardRouteChanged:
			apply()
		}
	}
}

// startSubscription spins up a goroutine that forwards MIDI-in events for
// the routed track through HandleMIDI. Skips tracks that have no device or
// no configured port — silent, because those tracks legitimately shouldn't
// receive MIDI.
func startSubscription(ctx context.Context, project *model.Project, out midi.ToExternal, in midi.FromExternal, trackIdx int) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := project.Tracks[trackIdx]
	if track == nil || track.Type == model.DeviceNone || track.PortName == "" {
		return
	}
	ch, err := in.Subscribe(track.PortName, track.Channel)
	if err != nil {
		debug.Log("keyboard", "midi subscribe failed track=%d port=%q err=%v",
			trackIdx, track.PortName, err)
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				HandleMIDI(project, out, trackIdx, ev)
			}
		}
	}()
}

// HandleMIDI dispatches one MIDI event to the routed track's device handler
// and marks the track dirty so the compile routine refills its Machine
// slots. Non-recording devices are no-ops in the per-domain handlers so
// the extra recompile on an idle MIDI stream is a cheap false positive we
// accept for simplicity.
func HandleMIDI(project *model.Project, out midi.ToExternal, trackIdx int, ev midi.Event) {
	if trackIdx < 0 || trackIdx >= 8 {
		return
	}
	track := project.Tracks[trackIdx]
	if track == nil {
		return
	}
	track.Lock()
	switch track.Type {
	case model.DeviceDrum:
		drumMIDI(track, project, trackIdx, out, ev)
	case model.DevicePiano:
		pianoMIDI(track, project, trackIdx, out, ev)
	case model.DeviceMetropolix:
		metropolixMIDI(track, project, trackIdx, out, ev)
	}
	track.Unlock()
	controller.MarkTrackDirty(project, trackIdx)
}
