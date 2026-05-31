package looper_test

import (
	"testing"

	"go-sequence/controller/devices/looper"
	"go-sequence/midi"
	"go-sequence/model/devices"
)

func TestEmptyPattern(t *testing.T) {
	compiled := looper.Compile(nil, 0)
	if compiled.Length != 1 {
		t.Errorf("Expected length 1, got %d", compiled.Length)
	}
	if len(compiled.Events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(compiled.Events))
	}
}

func TestNoEvents(t *testing.T) {
	pat := &devices.LooperPattern{Length: 5}
	compiled := looper.Compile(pat, 0)
	if compiled.Length != 5 {
		t.Errorf("Expected length 5, got %d", compiled.Length)
	}
	if len(compiled.Events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(compiled.Events))
	}
}

func TestEventsCopied(t *testing.T) {
	events := []devices.TimedEvent{{Tick: 100, Event: midi.Event{Type: midi.NoteOn}}}
	pat := &devices.LooperPattern{Events: events, Length: 1}
	compiled := looper.Compile(pat, 0)

	pat.Events[0].Tick = 200
	if compiled.Events[0].Tick != 100 {
		t.Errorf("Expected original tick 100, got %d", compiled.Events[0].Tick)
	}
}

func TestEventSorting(t *testing.T) {
	events := []devices.TimedEvent{{Tick: 3}, {Tick: 1}, {Tick: 2}}
	pat := &devices.LooperPattern{Events: events, Length: 1}
	compiled := looper.Compile(pat, 0)

	for i := 1; i < len(compiled.Events); i++ {
		if compiled.Events[i-1].Tick >= compiled.Events[i].Tick {
			t.Errorf("Events not sorted: %d >= %d", compiled.Events[i-1].Tick, compiled.Events[i].Tick)
		}
	}
}

func TestDefaultLength(t *testing.T) {
	pat := &devices.LooperPattern{Length: 0}
	compiled := looper.Compile(pat, 0)
	if compiled.Length != 1 {
		t.Errorf("Expected length 1, got %d", compiled.Length)
	}
}