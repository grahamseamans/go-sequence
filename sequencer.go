package main

import (
	"fmt"
	"sync"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

type Step struct {
	Active bool
	Note   uint8
}

type Sequencer struct {
	Steps    [16]Step
	Tempo    int // BPM
	Playhead int
	Playing  bool

	outPort  drivers.Out
	send     func(msg midi.Message) error
	stopChan chan struct{}
	mu       sync.Mutex

	// Channel to notify TUI of playhead updates
	PlayheadChan chan int
}

func NewSequencer() *Sequencer {
	s := &Sequencer{
		Tempo:        120,
		PlayheadChan: make(chan int, 1),
	}
	// Initialize all steps with default note (C4 = 60)
	for i := range s.Steps {
		s.Steps[i] = Step{Active: false, Note: 60}
	}
	return s
}

func ListMIDIPorts() []string {
	outs := midi.GetOutPorts()
	names := make([]string, len(outs))
	for i, out := range outs {
		names[i] = out.String()
	}
	return names
}

func (s *Sequencer) OpenPort(index int) error {
	outs := midi.GetOutPorts()
	if index < 0 || index >= len(outs) {
		return fmt.Errorf("invalid port index: %d", index)
	}
	s.outPort = outs[index]
	send, err := midi.SendTo(s.outPort)
	if err != nil {
		return fmt.Errorf("failed to open port: %w", err)
	}
	s.send = send
	return nil
}

func (s *Sequencer) Close() {
	s.Stop()
	midi.CloseDriver()
}

func (s *Sequencer) Play() {
	s.mu.Lock()
	if s.Playing {
		s.mu.Unlock()
		return
	}
	s.Playing = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.playLoop()
}

func (s *Sequencer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Playing {
		return
	}
	s.Playing = false
	close(s.stopChan)
	// Send note off for any playing note
	if s.send != nil {
		s.send(midi.NoteOff(0, s.Steps[s.Playhead].Note))
	}
}

func (s *Sequencer) playLoop() {
	for {
		s.mu.Lock()
		if !s.Playing {
			s.mu.Unlock()
			return
		}
		tempo := s.Tempo
		step := s.Steps[s.Playhead]
		playhead := s.Playhead
		s.mu.Unlock()

		// Calculate step duration (16th notes)
		// 1 beat = 4 sixteenth notes, so 16th = (60/BPM)/4 seconds
		stepDuration := time.Duration(float64(time.Second) * 60.0 / float64(tempo) / 4.0)

		// Send note
		if step.Active && s.send != nil {
			s.send(midi.NoteOn(0, step.Note, 100))
			// Note off after 80% of step duration
			go func(note uint8) {
				time.Sleep(stepDuration * 80 / 100)
				s.send(midi.NoteOff(0, note))
			}(step.Note)
		}

		// Notify TUI
		select {
		case s.PlayheadChan <- playhead:
		default:
		}

		// Wait for next step or stop
		select {
		case <-s.stopChan:
			return
		case <-time.After(stepDuration):
		}

		// Advance playhead
		s.mu.Lock()
		s.Playhead = (s.Playhead + 1) % 16
		s.mu.Unlock()
	}
}

func (s *Sequencer) ToggleStep(index int) {
	if index >= 0 && index < 16 {
		s.mu.Lock()
		s.Steps[index].Active = !s.Steps[index].Active
		s.mu.Unlock()
	}
}

func (s *Sequencer) SetNote(index int, note uint8) {
	if index >= 0 && index < 16 && note <= 127 {
		s.mu.Lock()
		s.Steps[index].Note = note
		s.mu.Unlock()
	}
}

func (s *Sequencer) AdjustNote(index int, delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index >= 0 && index < 16 {
		newNote := int(s.Steps[index].Note) + delta
		if newNote < 0 {
			newNote = 0
		}
		if newNote > 127 {
			newNote = 127
		}
		s.Steps[index].Note = uint8(newNote)
	}
}

func (s *Sequencer) SetTempo(bpm int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bpm < 20 {
		bpm = 20
	}
	if bpm > 300 {
		bpm = 300
	}
	s.Tempo = bpm
}

func (s *Sequencer) GetState() (steps [16]Step, playhead int, playing bool, tempo int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Steps, s.Playhead, s.Playing, s.Tempo
}
