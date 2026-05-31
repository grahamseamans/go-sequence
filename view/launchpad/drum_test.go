package launchpad

import (
	"testing"

	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/model/devices"
)

func newTestProject() *model.Project {
	p := model.New()
	track := p.Tracks[0]
	track.Type = model.DeviceDrum
	track.Kit = devices.DefaultKit
	track.PortName = "test-out"
	track.Channel = 0
	track.Device = &devices.Drum{
		EditingPatternIdx: 0,
	}
	p.Validate()
	d := track.Device.(*devices.Drum)
	pat := d.Patterns[0]
	// Set pattern length to 16 sixteenth notes
	pat.Length = 16 * (devices.PPQ / 4)
	// Lane 5, step 3 active
	pat.Notes[5].Steps[3].Active = true
	pat.Notes[5].Steps[3].Velocity = 100
	// Lane 1, step 0 active
	pat.Notes[1].Steps[0].Active = true
	pat.Notes[1].Steps[0].Velocity = 100
	d.SelectedNoteIdx = 0
	return p
}

func TestDrumPad_ToggleStep(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	mock := &midi.MockToExternal{}

	stepIdx := (7-4)*8 + 0 // row 4, col 0 => step 24
	drumPad(track, p, mock, 4, 0, true)

	d := track.Device.(*devices.Drum)
	if !d.Patterns[0].Notes[0].Steps[stepIdx].Active {
		t.Error("expected step 24 to become active after press")
	}
	if d.Cursor != stepIdx {
		t.Errorf("expected cursor %d, got %d", stepIdx, d.Cursor)
	}

	drumPad(track, p, mock, 4, 0, true)
	if d.Patterns[0].Notes[0].Steps[stepIdx].Active {
		t.Error("expected step 24 to become inactive after second press")
	}
}

func TestDrumPad_LaneSelect(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	mock := &midi.MockToExternal{}

	drumPad(track, p, mock, 0, 0, true)
	d := track.Device.(*devices.Drum)
	if d.SelectedNoteIdx != 0 {
		t.Errorf("expected SelectedNoteIdx 0, got %d", d.SelectedNoteIdx)
	}

	drumPad(track, p, mock, 1, 2, true)
	if d.SelectedNoteIdx != 6 {
		t.Errorf("expected SelectedNoteIdx 6, got %d", d.SelectedNoteIdx)
	}
}

func TestDrumPad_PreviewSendsNote(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	mock := &midi.MockToExternal{}

	d := track.Device.(*devices.Drum)
	d.Preview = true
	drumPad(track, p, mock, 0, 0, true)

	if len(mock.Sends) != 1 {
		t.Fatalf("expected 1 MIDI send, got %d", len(mock.Sends))
	}
	s := mock.Sends[0]
	if s.Event.Type != midi.NoteOn {
		t.Errorf("expected NoteOn, got %v", s.Event.Type)
	}
	if s.Event.Note != 36 {
		t.Errorf("expected note 36 (kick), got %d", s.Event.Note)
	}
	if s.PortName != "test-out" {
		t.Errorf("expected port test-out, got %s", s.PortName)
	}
}

func TestDrumPad_PreviewNoSendWhenOff(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	mock := &midi.MockToExternal{}

	drumPad(track, p, mock, 0, 0, true)
	if len(mock.Sends) != 0 {
		t.Errorf("expected 0 MIDI sends when preview off, got %d", len(mock.Sends))
	}
}

func TestDrumPad_ReleaseIgnored(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	mock := &midi.MockToExternal{}

	drumPad(track, p, mock, 4, 0, false)
	d := track.Device.(*devices.Drum)
	if d.Patterns[0].Notes[0].Steps[24].Active {
		t.Error("expected no change on release (down=false)")
	}
}

func TestDrumLEDs_ActiveStepsLit(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]

	leds := drumLEDs(track, p)
	if len(leds) < 32 {
		t.Fatalf("expected at least 32 step LEDs, got %d", len(leds))
	}
	stepLEDs := leds[:32]

	// Selected lane is 0. Lane 0 has no active steps, all should be drumStepsEmpty
	for i := 0; i < 16; i++ {
		if stepLEDs[i].R == 234 && stepLEDs[i].G == 73 && stepLEDs[i].B == 116 {
			t.Errorf("step %d should be empty (lane 0 has no hits), got active color", i)
		}
	}
}

func TestDrumLEDs_SelectedLaneWhite(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]

	leds := drumLEDs(track, p)
	if len(leds) < 48 {
		t.Fatalf("expected at least 48 LEDs, got %d", len(leds))
	}

	sel := findLEDMsg(leds, 0, 0, t)
	if !colorEqual(sel, drumNoteSelected) {
		t.Errorf("expected lane (0,0) selected color, got %+v", sel)
	}

	hasContent := findLEDMsg(leds, 0, 1, t)
	if !colorEqual(hasContent, drumNoteHasContent) {
		t.Errorf("expected lane (0,1) to have content color, got %+v", hasContent)
	}
}

func TestDrumLEDs_EmptyLaneDim(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]

	leds := drumLEDs(track, p)
	empty := findLEDMsg(leds, 0, 2, t)
	if !colorEqual(empty, drumNoteEmpty) {
		t.Errorf("expected lane (0,2) empty color, got %+v", empty)
	}
}

func TestDrumLEDs_BeyondPatternLengthOff(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	d := track.Device.(*devices.Drum)

	pat := d.Patterns[d.EditingPatternIdx]
	ppq4 := devices.PPQ / 4
	if pat.Length != 16*ppq4 {
		t.Fatalf("expected pattern length %d, got %d", 16*ppq4, pat.Length)
	}

	leds := drumLEDs(track, p)

	// Step grid is rows 4..7. Step idx = (7-row)*8 + col.
	// Steps 16..31 are beyond the 16-step pattern length.
	beyond := []struct{ row, col int }{}
	for row := 4; row <= 7; row++ {
		for col := 0; col < 8; col++ {
			if (7-row)*8+col >= 16 {
				beyond = append(beyond, struct{ row, col int }{row, col})
			}
		}
	}
	if len(beyond) != 16 {
		t.Fatalf("expected 16 steps beyond pattern, got %d", len(beyond))
	}
	for _, b := range beyond {
		led := findLEDMsg(leds, b.row, b.col, t)
		if !colorEqual(led, drumOffColor) {
			t.Errorf("step at (row=%d,col=%d) expected drumOffColor, got %+v", b.row, b.col, led)
		}
	}
}

func TestDrumLEDs_PreviewToggleBright(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	d := track.Device.(*devices.Drum)

	leds := drumLEDs(track, p)
	// Preview off: (3,4) should be command color
	cmd := findLED(leds, 3, 4)
	if !colorEqual(cmd, drumCommandsColor) {
		t.Errorf("expected preview off at (3,4) to be command color, got %+v", cmd)
	}

	d.Preview = true
	leds2 := drumLEDs(track, p)
	cmd2 := findLED(leds2, 3, 4)
	if !colorEqual(cmd2, drumPreviewActive) {
		t.Errorf("expected preview on at (3,4) to be green, got %+v", cmd2)
	}
}

func TestDrumLaneHasAnyHit_True(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	d := track.Device.(*devices.Drum)

	if !drumLaneHasAnyHit(&d.Patterns[0].Notes[5]) {
		t.Error("expected lane 5 to have hits")
	}
}

func TestDrumLaneHasAnyHit_False(t *testing.T) {
	p := newTestProject()
	track := p.Tracks[0]
	d := track.Device.(*devices.Drum)

	if drumLaneHasAnyHit(&d.Patterns[0].Notes[3]) {
		t.Error("expected lane 3 to have no hits")
	}
}

func colorEqual(a, b LED) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B
}

func findLED(leds []LED, row, col int) LED {
	for _, l := range leds {
		if l.Row == row && l.Col == col {
			return l
		}
	}
	return LED{}
}

func findLEDMsg(leds []LED, row, col int, t *testing.T) LED {
	for _, l := range leds {
		if l.Row == row && l.Col == col {
			return l
		}
	}
	t.Fatalf("no LED at row=%d col=%d", row, col)
	return LED{}
}