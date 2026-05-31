package launchpad

import (
	"testing"

	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// newTestSessionProject returns a project focused on the session view with
// every track carrying a drum device (so none is DeviceNone), except track 7
// which is intentionally left as DeviceNone to test that scenes skip it.
func newTestSessionProject() *model.Project {
	p := model.New()
	for i := 0; i < 7; i++ {
		track := p.Tracks[i]
		track.Type = model.DeviceDrum
		track.Kit = devices.DefaultKit
		track.PortName = "test-out"
		track.Channel = uint8(i)
		track.Device = &devices.Drum{EditingPatternIdx: 0}
	}
	// Track 7 stays DeviceNone (no device). model.New() already initializes
	// every cursor to QueuedPattern = -1.
	p.Validate()
	p.UI.Focus = model.FocusTarget{Kind: model.FocusSession}
	return p
}

func TestSessionPad_UnqueueToggle(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}

	// Physical row 7 is the TOP row = track 0 (the grid is flipped so track 0
	// sits on top). Tap an unqueued slot -> queues it.
	sessionPad(p, mock, 7, 3, true)
	if p.Playback.Cursors[0].QueuedPattern != 3 {
		t.Fatalf("expected QueuedPattern 3 after first tap, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	// Tap the same (now-queued) slot again -> cancels the queue.
	sessionPad(p, mock, 7, 3, true)
	if p.Playback.Cursors[0].QueuedPattern != -1 {
		t.Fatalf("expected QueuedPattern -1 after un-queue, got %d", p.Playback.Cursors[0].QueuedPattern)
	}
}

func TestSessionPad_QueueDifferentReplaces(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}

	sessionPad(p, mock, 7, 2, true) // row 7 = track 0
	if p.Playback.Cursors[0].QueuedPattern != 2 {
		t.Fatalf("expected QueuedPattern 2, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	sessionPad(p, mock, 7, 5, true)
	if p.Playback.Cursors[0].QueuedPattern != 5 {
		t.Fatalf("expected QueuedPattern 5 after queuing a different slot, got %d", p.Playback.Cursors[0].QueuedPattern)
	}
}

func TestSessionPad_RowFlip_Track0OnTopRow(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}

	// Top row (physical 7) must reach track 0.
	sessionPad(p, mock, 7, 1, true)
	if p.Playback.Cursors[0].QueuedPattern != 1 {
		t.Errorf("top row (7) should queue track 0, got Cursors[0]=%d", p.Playback.Cursors[0].QueuedPattern)
	}
	// Bottom row (physical 0) maps to track 7 (no device here) -> untouched.
	sessionPad(p, mock, 0, 1, true)
	if p.Playback.Cursors[7].QueuedPattern != -1 {
		t.Errorf("bottom row maps to track 7 (no device), should stay -1, got %d", p.Playback.Cursors[7].QueuedPattern)
	}
}

func TestSessionLEDs_Track0RendersOnTopRow(t *testing.T) {
	p := newTestSessionProject()
	p.Playback.Cursors[0].CurrentPattern = 0 // track 0 has a playing cell
	leds := sessionLEDs(p)

	// Track 0's playing cell must land on physical row 7 (top), not row 0.
	top := findLEDMsg(leds, 7, 0, t)
	if !colorEqual(top, sessionPlaying) {
		t.Errorf("track 0 playing cell should be on top row (7), got %+v", top)
	}
}

func TestSessionScene_QueuesColumnOnAllDeviceTracks(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}
	var saveOps save.SaveOps

	// Top-row scene trigger: Row 8, Col 3, press.
	HandlePad(p, mock, saveOps, PadEvent{Row: 8, Col: 3, Down: true})

	for i := 0; i < 7; i++ {
		if p.Playback.Cursors[i].QueuedPattern != 3 {
			t.Errorf("track %d: expected QueuedPattern 3 from scene, got %d", i, p.Playback.Cursors[i].QueuedPattern)
		}
	}
	// Track 7 has no device — must be untouched.
	if p.Playback.Cursors[7].QueuedPattern != -1 {
		t.Errorf("track 7 (no device): expected QueuedPattern untouched (-1), got %d", p.Playback.Cursors[7].QueuedPattern)
	}
}

func TestSessionScene_PressOnly(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}
	var saveOps save.SaveOps

	// Release (Down=false) must not queue anything.
	HandlePad(p, mock, saveOps, PadEvent{Row: 8, Col: 3, Down: false})
	for i := 0; i < 8; i++ {
		if p.Playback.Cursors[i].QueuedPattern != -1 {
			t.Errorf("track %d: scene release must not queue, got %d", i, p.Playback.Cursors[i].QueuedPattern)
		}
	}
}

func TestSessionLEDs_EmitsSceneTopRow(t *testing.T) {
	p := newTestSessionProject()
	leds := sessionLEDs(p)

	// All 8 top-row scene buttons must be present.
	for col := 0; col < 8; col++ {
		led := findLEDMsg(leds, 8, col, t)
		if !colorEqual(led, sessionSceneDim) {
			t.Errorf("scene button col %d: expected sessionSceneDim, got %+v", col, led)
		}
	}
}
