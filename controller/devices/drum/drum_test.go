package drum_test

import (
	"testing"

	"go-sequence/controller/devices/drum"
	"go-sequence/midi"
	"go-sequence/model/devices"
)

func TestEmptyPattern(t *testing.T) {
	pat := &devices.DrumPattern{Length: 0}
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if comp.Length != 1 {
		t.Errorf("expected length 1, got %d", comp.Length)
	}
	if len(comp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(comp.Events))
	}
}

func TestSingleActiveStep(t *testing.T) {
	pat := &devices.DrumPattern{Length: 240}
	pat.Notes[0].Steps[0].Active = true
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if len(comp.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(comp.Events))
	}
	if comp.Events[0].Tick != 0 {
		t.Errorf("expected tick 0, got %d", comp.Events[0].Tick)
	}
	if comp.Events[0].Event.Note != 36 {
		t.Errorf("expected MIDI note 36, got %d", comp.Events[0].Event.Note)
	}
	if comp.Events[0].Event.Velocity != 100 {
		t.Errorf("expected velocity 100, got %d", comp.Events[0].Event.Velocity)
	}
	if comp.Events[1].Tick != 60 {
		t.Errorf("expected tick 60, got %d", comp.Events[1].Tick)
	}
}

func TestMultipleLanes(t *testing.T) {
	pat := &devices.DrumPattern{Length: 240}
	pat.Notes[0].Steps[0].Active = true
	pat.Notes[1].Steps[0].Active = true
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if len(comp.Events) != 4 {
		t.Errorf("expected 4 events, got %d", len(comp.Events))
	}
	// lane 0 NoteOn, lane 1 NoteOn (both at tick 0, sorted by lane), lane 0 NoteOff, lane 1 NoteOff
	if comp.Events[0].Event.Note != 36 {
		t.Errorf("expected first note 36 (kick), got %d", comp.Events[0].Event.Note)
	}
	if comp.Events[0].Event.Type != midi.NoteOn {
		t.Errorf("expected first event NoteOn, got %v", comp.Events[0].Event.Type)
	}
	if comp.Events[1].Event.Note != 38 {
		t.Errorf("expected second note 38 (snare), got %d", comp.Events[1].Event.Note)
	}
	if comp.Events[1].Event.Type != midi.NoteOn {
		t.Errorf("expected second event NoteOn, got %v", comp.Events[1].Event.Type)
	}
}

func TestStepVelocity(t *testing.T) {
	pat := &devices.DrumPattern{Length: 240}
	pat.Notes[0].Steps[0].Active = true
	pat.Notes[0].Steps[0].Velocity = 75
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if comp.Events[0].Event.Velocity != 75 {
		t.Errorf("expected velocity 75, got %d", comp.Events[0].Event.Velocity)
	}
}

func TestPatternLength(t *testing.T) {
	pat := &devices.DrumPattern{Length: 240}
	pat.Notes[0].Steps[8].Active = true
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if len(comp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(comp.Events))
	}
}

func TestNudge(t *testing.T) {
	pat := &devices.DrumPattern{Length: 240}
	pat.Notes[0].Steps[0].Active = true
	pat.Notes[0].Steps[0].Nudge = 10
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if comp.Events[0].Tick != 10 {
		t.Errorf("expected tick 10, got %d", comp.Events[0].Tick)
	}
}

func TestNoteOffDuration(t *testing.T) {
	pat := &devices.DrumPattern{Length: 240}
	pat.Notes[0].Steps[0].Active = true
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if comp.Events[1].Tick != 60 {
		t.Errorf("expected tick 60, got %d", comp.Events[1].Tick)
	}
}

func TestEventsSorted(t *testing.T) {
	pat := &devices.DrumPattern{Length: 480}
	pat.Notes[0].Steps[1].Active = true
	pat.Notes[0].Steps[0].Active = true
	comp := drum.Compile(pat, devices.GetKit("gm"), 0)
	if len(comp.Events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(comp.Events))
	}
	// Expected order: NoteOn step 0 (tick 0), NoteOff step 0 (tick 60), NoteOn step 1 (tick 240), NoteOff step 1 (tick 300)
	if comp.Events[0].Tick != 0 {
		t.Errorf("expected first event at tick 0, got %d", comp.Events[0].Tick)
	}
	if comp.Events[1].Tick != 60 {
		t.Errorf("expected NoteOff at tick 60, got %d", comp.Events[1].Tick)
	}
	if comp.Events[2].Tick != 240 {
		t.Errorf("expected NoteOn step 1 at tick 240, got %d", comp.Events[2].Tick)
	}
	if comp.Events[3].Tick != 300 {
		t.Errorf("expected NoteOff step 1 at tick 300, got %d", comp.Events[3].Tick)
	}
}