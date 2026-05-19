package tui

import (
	"fmt"
	"testing"

	"go-sequence/controller"
	"go-sequence/model"
	"go-sequence/theme"
)

func TestSettingsRenderAlignment(t *testing.T) {
	project := model.New()
	project.UI.Focus.Kind = model.FocusSettings
	project.UI.Focus.Track = 0

	palette, err := theme.LoadGPL("../../palettes/plasma.gpl")
	if err != nil {
		t.Fatalf("palette: %v", err)
	}
	th := theme.New(palette)

	saveOps := controller.NewControllerSaveOps()
	output := Render(project, saveOps, th)
	_ = output

	// Manually build what the builder should contain
	var b strings.Builder
	b.WriteString("SETTINGS  Track & MIDI Configuration\n\n")
	header := fmt.Sprintf("  %-4s  %-12s  %-7s  %-20s  %s\n", "Trk", "Type", "Channel", "Output", "Kit")
	b.WriteString(header)
	
	row := 0
	fmt.Fprintf(&b, "  %2d  ", row+1)
	settingsWriteCell(&b, "empty", 12, 0, 0, 0, &system.Settings{}, cursorStyleDummy)
	b.WriteString("  ")
	settingsWriteCell(&b, "1", 7, 0, 1, 0, &system.Settings{}, cursorStyleDummy)
	b.WriteString("  ")
	settingsWriteCell(&b, "-", 20, 0, 2, 0, &system.Settings{}, cursorStyleDummy)
	b.WriteString("  ")
	settingsWriteCell(&b, "-", 10, 0, 3, 0, &system.Settings{}, cursorStyleDummy)
	b.WriteString("\n")

	expected := b.String()
	
	// TODO: compare
	fmt.Printf("manual output: %q\n", expected[:80])
	_ = output
}
