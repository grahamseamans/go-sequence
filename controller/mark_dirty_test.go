package controller_test

import (
	"sync/atomic"
	"testing"

	"go-sequence/controller"
	"go-sequence/model"
	"go-sequence/model/devices"
)

func TestMarkPatternDirty_Drum(t *testing.T) {
	project := model.New()
	track := project.Tracks[0]
	track.Type = model.DeviceDrum
	track.Device = &devices.Drum{
		Patterns: [devices.NumPatterns]*devices.DrumPattern{
			0: {Machine: [2]atomic.Pointer[devices.CompiledPattern]{}},
		},
		EditingPatternIdx: 0,
	}

	// Pre-fill slots
	drum := track.Device.(*devices.Drum)
	drum.Patterns[0].Machine[0].Store(&devices.CompiledPattern{})
	drum.Patterns[0].Machine[1].Store(&devices.CompiledPattern{})

	controller.MarkPatternDirty(project, 0, 0)

	if drum.Patterns[0].Machine[0].Load() != nil {
		t.Error("slot 0 not niled")
	}
	if drum.Patterns[0].Machine[1].Load() != nil {
		t.Error("slot 1 not niled")
	}

	requests := drainCompileCh(project)
	if len(requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(requests))
	}
}

func TestMarkPatternDirty_Looper(t *testing.T) {
	project := model.New()
	track := project.Tracks[0]
	track.Type = model.DeviceLooper
	track.Device = &devices.Looper{
		Patterns: [devices.NumPatterns]*devices.LooperPattern{
			0: {Machine: [2]atomic.Pointer[devices.CompiledPattern]{}},
		},
		EditingPatternIdx: 0,
	}

	looper := track.Device.(*devices.Looper)
	looper.Patterns[0].Machine[0].Store(&devices.CompiledPattern{})
	looper.Patterns[0].Machine[1].Store(&devices.CompiledPattern{})

	controller.MarkPatternDirty(project, 0, 0)

	if looper.Patterns[0].Machine[0].Load() != nil {
		t.Error("slot 0 not niled")
	}
	if looper.Patterns[0].Machine[1].Load() != nil {
		t.Error("slot 1 not niled")
	}

	requests := drainCompileCh(project)
	if len(requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(requests))
	}
}

func TestMarkTrackDirty_Drum(t *testing.T) {
	project := model.New()
	track := project.Tracks[0]
	track.Type = model.DeviceDrum
	track.Device = &devices.Drum{
		Patterns: [devices.NumPatterns]*devices.DrumPattern{
			0: {Machine: [2]atomic.Pointer[devices.CompiledPattern]{}},
		},
		EditingPatternIdx: 0,
	}

	drum := track.Device.(*devices.Drum)
	drum.Patterns[0].Machine[0].Store(&devices.CompiledPattern{})
	drum.Patterns[0].Machine[1].Store(&devices.CompiledPattern{})

	controller.MarkTrackDirty(project, 0)

	if drum.Patterns[0].Machine[0].Load() != nil {
		t.Error("slot 0 not niled")
	}
	if drum.Patterns[0].Machine[1].Load() != nil {
		t.Error("slot 1 not niled")
	}

	requests := drainCompileCh(project)
	if len(requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(requests))
	}
}

func TestMarkTrackDirty_DeviceNone(t *testing.T) {
	project := model.New()
	track := project.Tracks[0]
	track.Type = model.DeviceNone
	track.Device = nil

	controller.MarkTrackDirty(project, 0)

	// Verify no compile requests were sent for DeviceNone
	requests := drainCompileCh(project)
	if len(requests) != 0 {
		t.Errorf("expected 0 requests for DeviceNone, got %d", len(requests))
	}
}

func TestMarkPlayingDirty(t *testing.T) {
	project := model.New()
	track := project.Tracks[0]
	track.Type = model.DeviceDrum
	track.Device = &devices.Drum{
		Patterns: [devices.NumPatterns]*devices.DrumPattern{
			0: {Machine: [2]atomic.Pointer[devices.CompiledPattern]{}},
		},
		EditingPatternIdx: 0,
	}
	project.Playback.Cursors[0].CurrentPattern = 0

	drum := track.Device.(*devices.Drum)
	drum.Patterns[0].Machine[0].Store(&devices.CompiledPattern{})
	drum.Patterns[0].Machine[1].Store(&devices.CompiledPattern{})

	controller.MarkPlayingDirty(project, 0)

	if drum.Patterns[0].Machine[0].Load() != nil {
		t.Error("slot 0 not niled")
	}
	if drum.Patterns[0].Machine[1].Load() != nil {
		t.Error("slot 1 not niled")
	}

	requests := drainCompileCh(project)
	if len(requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(requests))
	}
}

// drainCompileCh reads all pending CompileRequests from the project's compile
// channel without blocking.
func drainCompileCh(project *model.Project) []model.CompileRequest {
	var reqs []model.CompileRequest
	for {
		select {
		case r := <-project.Compile.Channel:
			reqs = append(reqs, r)
		default:
			return reqs
		}
	}
}