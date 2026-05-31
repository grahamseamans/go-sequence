package launchpad

import (
	"testing"

	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

// Session grid orientation: column = instrument/track, row = pattern with
// pattern 0 on the TOP (physical row 7). So a press at physical row r, col c
// targets track c, pattern (7 - r).

// newTestSessionProject gives tracks 0..6 a drum device and leaves track 7 as
// DeviceNone, to check that scene/queue helpers skip device-less tracks.
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
	// Track 7 stays DeviceNone. model.New() initializes every cursor to
	// QueuedPattern = -1.
	p.Validate()
	p.UI.Focus = model.FocusTarget{Kind: model.FocusSession}
	return p
}

// allDeviceSessionProject gives all 8 tracks a device, for orientation tests
// that need every corner of the grid to register.
func allDeviceSessionProject() *model.Project {
	p := newTestSessionProject()
	p.Tracks[7].Type = model.DeviceDrum
	p.Tracks[7].Device = &devices.Drum{EditingPatternIdx: 0}
	p.Validate()
	return p
}

func TestSessionPad_UnqueueToggle(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}

	// Track 0 (col 0), pattern 3 -> physical row 7-3 = 4. Tap to queue.
	sessionPad(p, mock, 4, 0, true)
	if p.Playback.Cursors[0].QueuedPattern != 3 {
		t.Fatalf("expected QueuedPattern 3 after first tap, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	// Tap the same (now-queued) cell again -> cancels the queue.
	sessionPad(p, mock, 4, 0, true)
	if p.Playback.Cursors[0].QueuedPattern != -1 {
		t.Fatalf("expected QueuedPattern -1 after un-queue, got %d", p.Playback.Cursors[0].QueuedPattern)
	}
}

func TestSessionPad_QueueDifferentReplaces(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}

	sessionPad(p, mock, 5, 0, true) // track 0, pattern 2
	if p.Playback.Cursors[0].QueuedPattern != 2 {
		t.Fatalf("expected QueuedPattern 2, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	sessionPad(p, mock, 2, 0, true) // track 0, pattern 5
	if p.Playback.Cursors[0].QueuedPattern != 5 {
		t.Fatalf("expected QueuedPattern 5 after queuing a different pattern, got %d", p.Playback.Cursors[0].QueuedPattern)
	}
}

// TestSessionPad_Orientation locks the layout the user specified: top-left is
// instrument 1 / pattern 1, bottom-right is instrument 8 / pattern 8.
func TestSessionPad_Orientation(t *testing.T) {
	cases := []struct {
		name               string
		row, col           int
		wantTrack, wantPat int
	}{
		{"top-left = instr1/pat1", 7, 0, 0, 0},
		{"bottom-left = instr1/pat8", 0, 0, 0, 7},
		{"top-right = instr8/pat1", 7, 7, 7, 0},
		{"bottom-right = instr8/pat8", 0, 7, 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := allDeviceSessionProject()
			mock := &midi.MockToExternal{}
			sessionPad(p, mock, c.row, c.col, true)
			if got := p.Playback.Cursors[c.wantTrack].QueuedPattern; got != c.wantPat {
				t.Errorf("press (row=%d,col=%d): want track %d pattern %d, got Cursors[%d]=%d",
					c.row, c.col, c.wantTrack, c.wantPat, c.wantTrack, got)
			}
		})
	}
}

func TestSessionLEDs_Orientation(t *testing.T) {
	p := newTestSessionProject()
	p.Playback.Cursors[0].CurrentPattern = 0 // instrument 1, pattern 1 playing

	leds := sessionLEDs(p)

	// Instrument 1 / pattern 1 must light the TOP-LEFT cell: physical row 7,
	// col 0.
	cell := findLEDMsg(leds, 7, 0, t)
	if !colorEqual(cell, sessionPlaying) {
		t.Errorf("instr1/pat1 playing cell should be top-left (row 7, col 0), got %+v", cell)
	}
}

func TestSessionScene_QueuesPatternRowOnAllDeviceTracks(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}
	var saveOps save.SaveOps

	// Right-column "play all" button on physical row 4 -> pattern 3.
	HandlePad(p, mock, saveOps, PadEvent{Row: 4, Col: 8, Down: true})

	for i := 0; i < 7; i++ {
		if p.Playback.Cursors[i].QueuedPattern != 3 {
			t.Errorf("track %d: expected QueuedPattern 3 from scene, got %d", i, p.Playback.Cursors[i].QueuedPattern)
		}
	}
	// Track 7 has no device — must be untouched.
	if p.Playback.Cursors[7].QueuedPattern != -1 {
		t.Errorf("track 7 (no device): expected untouched (-1), got %d", p.Playback.Cursors[7].QueuedPattern)
	}
}

func TestSessionScene_PressOnly(t *testing.T) {
	p := newTestSessionProject()
	mock := &midi.MockToExternal{}
	var saveOps save.SaveOps

	// Release (Down=false) must not queue anything.
	HandlePad(p, mock, saveOps, PadEvent{Row: 4, Col: 8, Down: false})
	for i := 0; i < 8; i++ {
		if p.Playback.Cursors[i].QueuedPattern != -1 {
			t.Errorf("track %d: scene release must not queue, got %d", i, p.Playback.Cursors[i].QueuedPattern)
		}
	}
}

func TestSessionLEDs_EmitsSceneRightColumn(t *testing.T) {
	p := newTestSessionProject()
	leds := sessionLEDs(p)

	// All 8 right-column "play all" buttons (col 8, rows 0-7) must be present.
	for row := 0; row < 8; row++ {
		led := findLEDMsg(leds, row, 8, t)
		if !colorEqual(led, sessionSceneDim) {
			t.Errorf("scene button row %d: expected sessionSceneDim, got %+v", row, led)
		}
	}
}
