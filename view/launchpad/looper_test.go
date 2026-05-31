package launchpad

import (
	"testing"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

func newTestLooperProject() *model.Project {
	p := model.New()
	track := p.Tracks[0]
	track.Type = model.DeviceLooper
	track.PortName = "test-out"
	track.Channel = 0
	track.Device = &devices.Looper{}
	p.Validate()
	return p
}

func looperDevice(p *model.Project) *devices.Looper {
	return p.Tracks[0].Device.(*devices.Looper)
}

// gridPad sends a grid press (or release) and returns nothing; trackIdx 0.
func gridPress(p *model.Project, out midi.ToExternal, row, col int, vel uint8) {
	looperPad(p.Tracks[0], p, out, PadEvent{Row: row, Col: col, Down: true, Velocity: vel})
}

func gridRelease(p *model.Project, out midi.ToExternal, row, col int) {
	looperPad(p.Tracks[0], p, out, PadEvent{Row: row, Col: col, Down: false})
}

func TestLooperPad_NoteMath(t *testing.T) {
	cases := []struct {
		row, col int
		want     uint8
	}{
		{0, 0, 60},  // root, octave 0
		{1, 0, 65},  // up a row = +5
		{0, 1, 61},  // right one = +1
		{7, 7, 102}, // 60 + 5*7 + 7
	}
	for _, c := range cases {
		p := newTestLooperProject()
		mock := &midi.MockToExternal{}
		gridPress(p, mock, c.row, c.col, 100)
		if len(mock.Sends) != 1 {
			t.Fatalf("row=%d col=%d: expected 1 send, got %d", c.row, c.col, len(mock.Sends))
		}
		if mock.Sends[0].Event.Note != c.want {
			t.Errorf("row=%d col=%d: expected note %d, got %d", c.row, c.col, c.want, mock.Sends[0].Event.Note)
		}
		if mock.Sends[0].Event.Type != midi.NoteOn {
			t.Errorf("row=%d col=%d: expected NoteOn, got %v", c.row, c.col, mock.Sends[0].Event.Type)
		}
		if mock.Sends[0].Event.Velocity != 100 {
			t.Errorf("row=%d col=%d: expected velocity 100, got %d", c.row, c.col, mock.Sends[0].Event.Velocity)
		}
	}
}

func TestLooperPad_OctaveShift(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)

	// Octave+ (row 8, col 5).
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 5, Down: true})
	if d.Octave != 1 {
		t.Fatalf("expected octave 1 after Octave+, got %d", d.Octave)
	}
	gridPress(p, mock, 0, 0, 100)
	if mock.Sends[0].Event.Note != 72 { // 60 + 12
		t.Errorf("expected note 72 at octave +1, got %d", mock.Sends[0].Event.Note)
	}

	// Octave- twice (row 8, col 4) -> octave -1.
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 4, Down: true})
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 4, Down: true})
	if d.Octave != -1 {
		t.Errorf("expected octave -1, got %d", d.Octave)
	}
}

func TestLooperPad_OctaveClamp(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	for i := 0; i < 10; i++ {
		looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 5, Down: true})
	}
	if d.Octave != 4 {
		t.Errorf("expected octave clamped to 4, got %d", d.Octave)
	}
	for i := 0; i < 20; i++ {
		looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 4, Down: true})
	}
	if d.Octave != -4 {
		t.Errorf("expected octave clamped to -4, got %d", d.Octave)
	}
}

func TestLooperPad_RecordArmClearsEvents(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	pat := d.Patterns[d.EditingPatternIdx]
	pat.Events = []devices.TimedEvent{{Tick: 1, Event: midi.Event{Type: midi.NoteOn, Note: 60}}}
	d.Overdub = true

	// Record (row 8, col 0): arms, wipes loop, clears overdub.
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 0, Down: true})
	if !d.Recording {
		t.Error("expected Recording true after arming")
	}
	if d.Overdub {
		t.Error("expected Overdub cleared when Record armed")
	}
	if len(pat.Events) != 0 {
		t.Errorf("expected events wiped on record arm, got %d", len(pat.Events))
	}
}

func TestLooperPad_OverdubPreservesEvents(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	pat := d.Patterns[d.EditingPatternIdx]
	pat.Events = []devices.TimedEvent{{Tick: 1, Event: midi.Event{Type: midi.NoteOn, Note: 60}}}
	d.Recording = true

	// Overdub (row 8, col 1): arms, keeps events, clears record.
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 1, Down: true})
	if !d.Overdub {
		t.Error("expected Overdub true after arming")
	}
	if d.Recording {
		t.Error("expected Recording cleared when Overdub armed")
	}
	if len(pat.Events) != 1 {
		t.Errorf("expected events preserved on overdub arm, got %d", len(pat.Events))
	}
}

func TestLooperPad_ClearWipesAndDisarms(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	pat := d.Patterns[d.EditingPatternIdx]
	pat.Events = []devices.TimedEvent{{Tick: 1}}
	d.Recording = true
	d.Overdub = true

	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 2, Down: true}) // Clear
	if len(pat.Events) != 0 {
		t.Errorf("expected events cleared, got %d", len(pat.Events))
	}
	if d.Recording || d.Overdub {
		t.Error("expected both disarmed after Clear")
	}
}

func TestLooperPad_RecordingStampsTick(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	pat := d.Patterns[d.EditingPatternIdx]
	d.Recording = true

	// Position the transport so pos = 480 within the pattern.
	p.Playback.Cursors[0].T0Tick = 0
	p.Playback.Tick.Store(480)

	gridPress(p, mock, 0, 0, 100)

	if len(mock.Sends) != 0 {
		t.Errorf("expected no preview send while recording, got %d", len(mock.Sends))
	}
	if len(pat.Events) != 1 {
		t.Fatalf("expected 1 recorded event, got %d", len(pat.Events))
	}
	if pat.Events[0].Tick != 480 {
		t.Errorf("expected stamped tick 480, got %d", pat.Events[0].Tick)
	}
	if pat.Events[0].Event.Note != 60 || pat.Events[0].Event.Type != midi.NoteOn {
		t.Errorf("expected NoteOn note 60, got %+v", pat.Events[0].Event)
	}
}

func TestLooperPad_GridReleaseRecordsNoteOff(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	pat := d.Patterns[d.EditingPatternIdx]
	d.Recording = true
	p.Playback.Tick.Store(0)

	gridPress(p, mock, 0, 0, 100)
	gridRelease(p, mock, 0, 0)

	if len(pat.Events) != 2 {
		t.Fatalf("expected 2 recorded events (on+off), got %d", len(pat.Events))
	}
	if pat.Events[1].Event.Type != midi.NoteOff {
		t.Errorf("expected second event NoteOff, got %v", pat.Events[1].Event.Type)
	}
	if pat.Events[1].Event.Velocity != 0 {
		t.Errorf("expected NoteOff velocity 0, got %d", pat.Events[1].Event.Velocity)
	}
}

func TestLooperPad_RightColumnSlotSelect(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)

	// col 8, row 7 -> slot 0.
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 7, Col: 8, Down: true})
	if d.EditingPatternIdx != 0 {
		t.Errorf("expected EditingPatternIdx 0, got %d", d.EditingPatternIdx)
	}
	if p.Playback.Cursors[0].QueuedPattern != 0 {
		t.Errorf("expected QueuedPattern 0, got %d", p.Playback.Cursors[0].QueuedPattern)
	}

	// col 8, row 0 -> slot 7.
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 0, Col: 8, Down: true})
	if d.EditingPatternIdx != 7 {
		t.Errorf("expected EditingPatternIdx 7, got %d", d.EditingPatternIdx)
	}
	if p.Playback.Cursors[0].QueuedPattern != 7 {
		t.Errorf("expected QueuedPattern 7, got %d", p.Playback.Cursors[0].QueuedPattern)
	}
}

func TestLooperPad_QuantizeToggle(t *testing.T) {
	p := newTestLooperProject()
	mock := &midi.MockToExternal{}
	d := looperDevice(p)
	pat := d.Patterns[d.EditingPatternIdx]

	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 3, Down: true})
	if !pat.Quantize {
		t.Error("expected Quantize on after toggle")
	}
	looperPad(p.Tracks[0], p, mock, PadEvent{Row: 8, Col: 3, Down: true})
	if pat.Quantize {
		t.Error("expected Quantize off after second toggle")
	}
}

func TestLooperLEDs_FrameSize(t *testing.T) {
	p := newTestLooperProject()
	leds := looperLEDs(p.Tracks[0], p)
	if len(leds) != 80 {
		t.Fatalf("expected 80 LEDs (64 grid + 8 top + 8 right), got %d", len(leds))
	}
}

func TestLooperLEDs_RootHighlighted(t *testing.T) {
	p := newTestLooperProject()
	leds := looperLEDs(p.Tracks[0], p)
	// Octave 0: row0col0 = note 60 (root). Should be looperRoot.
	root := findLEDMsg(leds, 0, 0, t)
	if !colorEqual(root, looperRoot) {
		t.Errorf("expected (0,0) root color, got %+v", root)
	}
	// row0col1 = note 61, non-root.
	key := findLEDMsg(leds, 0, 1, t)
	if !colorEqual(key, looperKey) {
		t.Errorf("expected (0,1) non-root key color, got %+v", key)
	}
}

func TestLooperLEDs_RightColumnSessionColors(t *testing.T) {
	p := newTestLooperProject()
	d := looperDevice(p)

	// Give slot 0 content (blue), set cursor playing slot 1 (green), queued slot 2 (amber).
	d.Patterns[0].Events = []devices.TimedEvent{{Tick: 0, Event: midi.Event{Type: midi.NoteOn, Note: 60}}}
	p.Playback.Cursors[0].CurrentPattern = 1
	p.Playback.Cursors[0].QueuedPattern = 2

	leds := looperLEDs(p.Tracks[0], p)

	// slot = 7 - row, so slot 0 is at row 7, slot 1 at row 6, slot 2 at row 5.
	blue := findLEDMsg(leds, 7, 8, t)
	if !colorEqual(blue, sessionHasContent) {
		t.Errorf("expected slot 0 (row7,col8) has-content color, got %+v", blue)
	}
	green := findLEDMsg(leds, 6, 8, t)
	if !colorEqual(green, sessionPlaying) {
		t.Errorf("expected slot 1 (row6,col8) playing color, got %+v", green)
	}
	amber := findLEDMsg(leds, 5, 8, t)
	if !colorEqual(amber, sessionQueued) {
		t.Errorf("expected slot 2 (row5,col8) queued color, got %+v", amber)
	}
}

func TestLooperLEDs_RecordTopRowDimToBright(t *testing.T) {
	p := newTestLooperProject()
	d := looperDevice(p)

	leds := looperLEDs(p.Tracks[0], p)
	rec := findLEDMsg(leds, 8, 0, t)
	if !colorEqual(rec, looperRecDim) {
		t.Errorf("expected Record dim when disarmed, got %+v", rec)
	}

	d.Recording = true
	leds2 := looperLEDs(p.Tracks[0], p)
	rec2 := findLEDMsg(leds2, 8, 0, t)
	if !colorEqual(rec2, looperRecOn) {
		t.Errorf("expected Record bright when armed, got %+v", rec2)
	}
}
