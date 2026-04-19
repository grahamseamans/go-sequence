package model

// Kit maps 16 drum slot indices to MIDI notes.
// Slot order: 0 Kick, 1 Snare, 2 Closed HH, 3 Open HH, 4 Low Tom, 5 Mid Tom,
// 6 High Tom, 7 Crash, 8 Ride, 9 Clap, 10 Rimshot, 11 Cowbell, 12 Clave,
// 13 Maracas, 14 Low Conga, 15 High Conga.
type Kit struct {
	Name  string
	Notes [16]uint8
}

// DefaultKit is the kit name used when a track has no explicit kit set.
const DefaultKit = "gm"

var kits = map[string]Kit{
	"gm": {
		Name: "General MIDI",
		Notes: [16]uint8{
			36, 38, 42, 46, 41, 43, 45, 49,
			51, 39, 37, 56, 75, 70, 64, 63,
		},
	},
	"rd8": {
		Name: "Behringer RD-8",
		Notes: [16]uint8{
			36, 40, 42, 46, 45, 48, 50, 49,
			51, 39, 37, 56, 75, 70, 64, 63,
		},
	},
	"tr8s": {
		Name: "Roland TR-8S",
		Notes: [16]uint8{
			36, 38, 42, 46, 41, 43, 45, 49,
			51, 39, 37, 56, 75, 70, 62, 63,
		},
	},
	"er1": {
		Name: "Korg ER-1",
		Notes: [16]uint8{
			36, 38, 42, 46, 40, 41, 43, 49,
			45, 39, 37, 56, 75, 70, 64, 63,
		},
	},
}

// KitNames returns the list of available kit names, in stable order.
func KitNames() []string { return []string{"gm", "rd8", "tr8s", "er1"} }

// GetKit returns the named kit, defaulting to the GM kit when the name is
// unknown or empty.
func GetKit(name string) Kit {
	if k, ok := kits[name]; ok {
		return k
	}
	return kits[DefaultKit]
}
