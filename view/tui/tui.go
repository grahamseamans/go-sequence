// Package tui is the terminal-UI view surface. Renders text via Bubbletea,
// captures keystrokes, and dispatches to per-domain key handlers.
//
// Architecture (DESIGN §3.6): views own UI input loops. Run starts the
// Bubbletea program; keystrokes flow through HandleKey which dispatches to
// the focused domain's keyboard handler. Render dispatches per-domain.
//
// This file is a skeleton — the actual rendering bodies are stubbed in
// per-domain files (drum.go, piano.go, ...). Full wiring of the Bubbletea
// model against those stubs will land in a later commit.
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-sequence/controller"
	"go-sequence/controller/devices/save"
	"go-sequence/midi"
	"go-sequence/model"
	"go-sequence/theme"
)

// tickMsg is the periodic redraw trigger. Bubbletea doesn't auto-redraw;
// Init seeds the first tick and Update re-arms it on every fire.
type tickMsg time.Time

// Run starts the Bubbletea program and blocks until it exits. The context
// is held by the Model so a cancellation from main propagates into the
// program.
func Run(ctx context.Context, project *model.Project, out midi.ToExternal, saveOps save.SaveOps, th *theme.Theme) error {
	m := newModel(ctx, project, out, saveOps, th)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Model is the Bubbletea Model for the TUI. Holds a pointer to the live
// project — never a copy.
type Model struct {
	ctx     context.Context
	project *model.Project
	out     midi.ToExternal
	saveOps save.SaveOps
	theme   *theme.Theme
}

func newModel(ctx context.Context, project *model.Project, out midi.ToExternal, saveOps save.SaveOps, th *theme.Theme) *Model {
	return &Model{
		ctx:     ctx,
		project: project,
		out:     out,
		saveOps: saveOps,
		theme:   th,
	}
}

// Init is Bubbletea's startup hook. Seeds the 30 Hz redraw tick.
func (m *Model) Init() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update handles tea.Msg events. Keystrokes route through HandleKey; a
// ctx cancellation exits the program.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case <-m.ctx.Done():
		return m, tea.Quit
	default:
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" || key == "q" {
			return m, tea.Quit
		}
		HandleKey(m.project, m.out, m.saveOps, key)
	case tickMsg:
		return m, tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
	}
	return m, nil
}

// View renders the TUI frame by dispatching to the focused domain's render.
func (m *Model) View() string {
	return Render(m.project, m.saveOps, m.theme)
}

// HandleKey dispatches one keystroke to the focused domain's handler.
// Global hotkeys (transport, focus) are handled directly; everything else
// routes to the focused device.
func HandleKey(project *model.Project, out midi.ToExternal, saveOps save.SaveOps, key string) {
	// Global hotkeys: never forwarded to devices.
	switch key {
	case " ", "space":
		if controller.Playing(project) {
			controller.Pause(project)
		} else {
			controller.Play(project)
		}
		return
	case "+", "=":
		controller.SetTempo(project, project.Tempo+1)
		return
	case "-":
		controller.SetTempo(project, project.Tempo-1)
		return
	case "1", "2", "3", "4", "5", "6", "7", "8":
		idx := int(key[0] - '1')
		project.UI.Focus = model.FocusTarget{Kind: model.FocusTrack, Track: idx}
		project.UI.LastFocusedTrack = idx
		return
	case ",":
		project.UI.Focus.Kind = model.FocusSettings
		return
	case "S":
		project.UI.Focus.Kind = model.FocusProject
		// Refresh the save-list cache on entry so the browser shows the
		// current state of disk, not whatever was cached last time we were
		// focused here.
		save.Refresh(&project.UI.SystemProject, saveOps)
		return
	case "tab":
		project.UI.Focus.Kind = model.FocusSession
		return
	}

	// Not a global hotkey — route to the focused device.
	switch project.UI.Focus.Kind {
	case model.FocusTrack:
		idx := project.UI.Focus.Track
		track := project.Tracks[idx]
		if track == nil {
			return
		}
		track.Lock()
		switch track.Type {
		case model.DeviceDrum:
			drumKey(track, project, out, key)
		case model.DevicePiano:
			pianoKey(track, project, out, key)
		case model.DeviceMetropolix:
			metropolixKey(track, project, out, key)
		default:
			emptyKey(project, out, key)
		}
		track.Unlock()
		controller.MarkTrackDirty(project, idx)

	case model.FocusSession:
		// No state change possible today — no dirty-mark. Dispatched for
		// symmetry.
		sessionKey(project, out, key)

	case model.FocusSettings:
		idx := project.UI.LastFocusedTrack
		tgt := project.Tracks[idx]
		if tgt != nil {
			tgt.Lock()
		}
		settingsKey(project, out, idx, key)
		if tgt != nil {
			tgt.Unlock()
		}
		controller.MarkTrackDirty(project, idx)

	case model.FocusProject:
		// An "enter" while focused on the project browser is a potential
		// load. If we're in edit-commit mode (IsEditing returns true), enter
		// commits a name rather than loading — no need for the post-load
		// Pause + recompile in that case.
		wasLoad := (key == "enter" || key == "return") && !save.IsEditing(&project.UI.SystemProject)
		projectKey(&project.UI.SystemProject, project, saveOps, out, key)
		if wasLoad {
			// Project contents may have been replaced. Per DESIGN §4.5: stop
			// playback and recompile every track's currently-playing pattern
			// so playback has something to read on Play.
			controller.Pause(project)
			controller.ResetCursors(project)
			for i := 0; i < 8; i++ {
				controller.MarkPlayingDirty(project, i)
			}
		}
	}
}

// Render returns the composed TUI frame for the focused domain.
func Render(project *model.Project, saveOps save.SaveOps, th *theme.Theme) string {
	switch project.UI.Focus.Kind {
	case model.FocusTrack:
		idx := project.UI.Focus.Track
		track := project.Tracks[idx]
		if track == nil {
			return ""
		}
		switch track.Type {
		case model.DeviceDrum:
			return drumRender(track, project, th)
		case model.DevicePiano:
			return pianoRender(track, project, th)
		case model.DeviceMetropolix:
			return metropolixRender(track, project, th)
		default:
			return emptyRender(project, th)
		}
	case model.FocusSession:
		return sessionRender(project, th)
	case model.FocusSettings:
		return settingsRender(project, th)
	case model.FocusProject:
		return projectRender(&project.UI.SystemProject, project, saveOps, th)
	}
	return ""
}
