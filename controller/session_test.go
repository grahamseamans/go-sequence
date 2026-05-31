package controller_test

import (
	"testing"

	"go-sequence/controller"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// sessionTestProject gives tracks 0..6 a drum device and leaves track 7 as
// DeviceNone, so scene/queue helpers can be checked against device and
// no-device tracks. model.New() initializes every cursor's QueuedPattern to -1.
func sessionTestProject() *model.Project {
	p := model.New()
	for i := 0; i < 7; i++ {
		track := p.Tracks[i]
		track.Type = model.DeviceDrum
		track.Device = &devices.Drum{EditingPatternIdx: 0}
	}
	p.Validate()
	return p
}

func TestToggleQueue_QueuesThenUnqueues(t *testing.T) {
	p := sessionTestProject()

	controller.ToggleQueue(p, 0, 3)
	if got := p.Playback.Cursors[0].QueuedPattern; got != 3 {
		t.Fatalf("after first toggle: expected QueuedPattern 3, got %d", got)
	}

	// Same slot again cancels the queue.
	controller.ToggleQueue(p, 0, 3)
	if got := p.Playback.Cursors[0].QueuedPattern; got != -1 {
		t.Fatalf("after second toggle: expected QueuedPattern -1, got %d", got)
	}
}

func TestToggleQueue_DifferentSlotReplaces(t *testing.T) {
	p := sessionTestProject()

	controller.ToggleQueue(p, 0, 2)
	controller.ToggleQueue(p, 0, 5)
	if got := p.Playback.Cursors[0].QueuedPattern; got != 5 {
		t.Fatalf("expected QueuedPattern 5 after queuing a different slot, got %d", got)
	}
}

func TestToggleQueue_SkipsNoDeviceTrack(t *testing.T) {
	p := sessionTestProject()
	controller.ToggleQueue(p, 7, 3) // track 7 has no device
	if got := p.Playback.Cursors[7].QueuedPattern; got != -1 {
		t.Fatalf("no-device track should be untouched, got %d", got)
	}
}

func TestQueueScene_QueuesColumnOnAllDeviceTracks(t *testing.T) {
	p := sessionTestProject()

	controller.QueueScene(p, 4)

	for i := 0; i < 7; i++ {
		if got := p.Playback.Cursors[i].QueuedPattern; got != 4 {
			t.Errorf("track %d: expected QueuedPattern 4 from scene, got %d", i, got)
		}
	}
	if got := p.Playback.Cursors[7].QueuedPattern; got != -1 {
		t.Errorf("track 7 (no device): expected untouched -1, got %d", got)
	}
}
