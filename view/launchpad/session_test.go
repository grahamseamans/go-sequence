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

	// Tap an unqueued slot -> queues it.
	sessionPad(p, mock, 0, 3, true)
	if p.Playback.Cursors[0].QueuedPattern != 3 {
		t.Fatalf("expected QueuedPattern 3 after first tap, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	// Tap the same (now-queued) slot again -> cancels the queue.
	sessionPad(p, mock, 0, 3, true)
	if p.Playback.Cursors[0].QueuedPattern != -1 {
		t.Fatalf("expected QueuedPattern -1 after un-queue, got %d", p.Playback.Cursors[0].QueuedPattern)
	}
}

func TestSessionPad_QueueDifferentReplaces(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}

	sessionPad(p, mock, 0, 2, true)
	if p.Playback.Cursors[0].QueuedPattern != 2 {
		t.Fatalf("expected QueuedPattern 2, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	sessionPad(p, mock, 0, 5, true)
	if p.Playback.Cursors[0].QueuedPattern != 5 {
		t.Fatalf("expected QueuedPattern 5 after queuing a different slot, got %d", p.Playback.Cursors[0].QueuedPattern)
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
