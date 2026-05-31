package model_test

import (
	"testing"

	"go-sequence/model"
	"go-sequence/model/devices"
)

func TestNew(t *testing.T) {
	project := model.New()
	if project.Tempo != 120 {
		t.Errorf("expected tempo 120, got %d", project.Tempo)
	}
	for i := 0; i < 8; i++ {
		track := project.Tracks[i]
		if track == nil {
			t.Errorf("track %d is nil", i)
			continue
		}
		if track.Type != model.DeviceNone {
			t.Errorf("track %d type: expected DeviceNone, got %v", i, track.Type)
		}
		if track.Channel != uint8(i) {
			t.Errorf("track %d channel: expected %d, got %d", i, i, track.Channel)
		}
		if project.Playback.Cursors[i].QueuedPattern != -1 {
			t.Errorf("track %d QueuedPattern: expected -1, got %d", i, project.Playback.Cursors[i].QueuedPattern)
		}
	}
}

func TestValidate_NilTrack(t *testing.T) {
	project := &model.Project{Tracks: [8]*model.Track{}}
	project.Validate()
	if project.Tracks[0] == nil {
		t.Error("expected nil track to be filled in")
	}
}

func TestValidate_TempoClamp(t *testing.T) {
	project := &model.Project{Tempo: 10, Tracks: [8]*model.Track{}}
	project.Validate()
	if project.Tempo != 20 {
		t.Errorf("expected tempo clamped to 20, got %d", project.Tempo)
	}
}

func TestValidate_DeviceNone(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceNone}
	p.Validate()
	if p.Tracks[0].Device != nil {
		t.Error("expected DeviceNone track to have nil Device")
	}
}

func TestValidate_DeviceDrum(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceDrum}
	p.Validate()
	if p.Tracks[0].Device == nil {
		t.Fatal("expected DeviceDrum track to have non-nil Device")
	}
	if _, ok := p.Tracks[0].Device.(*devices.Drum); !ok {
		t.Fatal("expected Device to be *devices.Drum")
	}
}

func TestValidate_DeviceLooper(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceLooper}
	p.Validate()
	if p.Tracks[0].Device == nil {
		t.Fatal("expected DeviceLooper track to have non-nil Device")
	}
	if _, ok := p.Tracks[0].Device.(*devices.Looper); !ok {
		t.Fatal("expected Device to be *devices.Looper")
	}
}

func TestValidate_DrumDefaults(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceDrum}
	p.Validate()
	d := p.Tracks[0].Device.(*devices.Drum)
	if d.Patterns[0] == nil {
		t.Fatal("expected pattern 0 to be non-nil")
	}
	expectedLength := 16 * (devices.PPQ / 4)
	if d.Patterns[0].Length != expectedLength {
		t.Errorf("expected pattern length %d, got %d", expectedLength, d.Patterns[0].Length)
	}
}

func TestPatternHasContent_Drum_Empty(t *testing.T) {
	track := &model.Track{Type: model.DeviceDrum, Device: &devices.Drum{}}
	// Need to init the pattern with Validate
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = track
	p.Validate()

	if model.PatternHasContent(track, 0) {
		t.Error("expected empty drum pattern to have no content")
	}
}

func TestPatternHasContent_Drum_Active(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceDrum}
	p.Validate()
	d := p.Tracks[0].Device.(*devices.Drum)
	d.Patterns[0].Notes[0].Steps[0].Active = true

	if !model.PatternHasContent(p.Tracks[0], 0) {
		t.Error("expected pattern with active step to have content")
	}
}

func TestPatternHasContent_Looper_Empty(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceLooper}
	p.Validate()

	if model.PatternHasContent(p.Tracks[0], 0) {
		t.Error("expected empty looper pattern to have no content")
	}
}

func TestPatternHasContent_Looper_HasEvent(t *testing.T) {
	p := &model.Project{Tracks: [8]*model.Track{}}
	p.Tracks[0] = &model.Track{Type: model.DeviceLooper}
	p.Validate()
	l := p.Tracks[0].Device.(*devices.Looper)
	l.Patterns[0].Events = append(l.Patterns[0].Events, devices.TimedEvent{})

	if !model.PatternHasContent(p.Tracks[0], 0) {
		t.Error("expected looper pattern with event to have content")
	}
}

func TestTrackIndex(t *testing.T) {
	project := model.New()
	track := project.Tracks[0]
	if idx := project.TrackIndex(track); idx != 0 {
		t.Errorf("expected index 0, got %d", idx)
	}
}

func TestReplacePersisted(t *testing.T) {
	original := model.New()
	original.Tempo = 60
	project := model.New()
	project.ReplacePersisted(original)
	if project.Tempo != 60 {
		t.Errorf("expected tempo 60, got %d", project.Tempo)
	}
	if project.Tracks != original.Tracks {
		t.Error("expected tracks to be replaced")
	}
}