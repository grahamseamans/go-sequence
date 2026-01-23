package sequencer

// DrumKit maps 16 drum slots to MIDI notes
type DrumKit struct {
	Name  string
	Notes [16]uint8
}

// Slot names for reference (not used in code, just documentation)
// 0: Kick
// 1: Snare
// 2: Closed HH
// 3: Open HH
// 4: Low Tom
// 5: Mid Tom
// 6: High Tom
// 7: Crash
// 8: Ride
// 9: Clap
// 10: Rimshot
// 11: Cowbell
// 12: Clave
// 13: Maracas
// 14: Low Conga
// 15: High Conga

// Kits contains all available drum kit mappings
var Kits = map[string]DrumKit{
	"gm": {
		Name: "General MIDI",
		Notes: [16]uint8{
			36, // Kick
			38, // Snare
			42, // Closed HH
			46, // Open HH
			41, // Low Tom
			43, // Mid Tom
			45, // High Tom
			49, // Crash
			51, // Ride
			39, // Clap
			37, // Rimshot
			56, // Cowbell
			75, // Clave
			70, // Maracas
			64, // Low Conga
			63, // High Conga
		},
	},
	"rd8": {
		Name: "Behringer RD-8",
		Notes: [16]uint8{
			36, // Kick (BD)
			40, // Snare (SD) - note: RD-8 uses 40, not 38!
			42, // Closed HH (CH)
			46, // Open HH (OH)
			45, // Low Tom (LT)
			48, // Mid Tom (MT)
			50, // High Tom (HT)
			49, // Crash (CY)
			51, // Ride (RC)
			39, // Clap (CP)
			37, // Rimshot (RS)
			56, // Cowbell (CB)
			75, // Clave (CL)
			70, // Maracas (MA)
			64, // Low Conga (LC)
			63, // High Conga (HC)
		},
	},
	"tr8s": {
		Name: "Roland TR-8S",
		Notes: [16]uint8{
			36, // Kick
			38, // Snare
			42, // Closed HH
			46, // Open HH
			41, // Low Tom
			43, // Mid Tom
			45, // High Tom
			49, // Crash
			51, // Ride
			39, // Clap
			37, // Rimshot
			56, // Cowbell
			75, // Clave
			70, // Maracas
			62, // Low Conga
			63, // High Conga
		},
	},
	"er1": {
		Name: "Korg ER-1",
		Notes: [16]uint8{
			36, // Perc Synth 1 (Kick)
			38, // Perc Synth 2 (Snare)
			42, // Closed HH (PCM)
			46, // Open HH (PCM)
			40, // Perc Synth 3 (Tom)
			41, // Perc Synth 4 (Zap/Cowbell)
			43, // Audio In 1
			49, // Crash (PCM)
			45, // Audio In 2
			39, // Hand Clap (PCM)
			37, // (unused - rimshot placeholder)
			56, // (unused - cowbell placeholder)
			75, // (unused)
			70, // (unused)
			64, // (unused)
			63, // (unused)
		},
	},
}

// KitNames returns the list of available kit names
func KitNames() []string {
	return []string{"gm", "rd8", "tr8s", "er1"}
}

// GetKit returns a kit by name, defaulting to GM if not found
func GetKit(name string) DrumKit {
	if kit, ok := Kits[name]; ok {
		return kit
	}
	return Kits["gm"]
}

// DefaultKit is the default kit name
const DefaultKit = "gm"
