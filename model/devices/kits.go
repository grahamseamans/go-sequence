package devices

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
	// JoMoX AlphaBase. Source: AlphaBase manual §13.1.1 (MIDI Note Commands).
	// To play it from one MIDI channel, set the track to MIDI Channel 16 —
	// the AlphaBase's "keyboard" mode, where each voice has a base note spaced
	// 4 semitones apart: Kick 36, Snare/MBrane 40, CHH 44, OHH 48, Clap 52,
	// Rim 56, Crash 60, Ride 64, X1 sample 68, X2 sample 72, FM synth 76.
	//
	// The AlphaBase has only those 11 voices — no toms, cowbell, clave,
	// maracas, or congas. Those slots are substitutes: the three toms use the
	// tunable X1/X2/FM voices, and the rest reuse the closest real voice. We
	// avoid the in-between notes (37-39, 41-43, ...) on purpose — in keyboard
	// mode those are pitch-shifted variants of the preceding voice, so a
	// "tom" on note 41 would actually fire a pitched snare.
	"alphabase": {
		Name: "JoMoX AlphaBase",
		Notes: [16]uint8{
			36, // 0  Kick        → Kick
			40, // 1  Snare       → MBrane
			44, // 2  Closed HH   → Closed HH
			48, // 3  Open HH     → Open HH
			68, // 4  Low Tom     → X1 sample   (no native tom)
			72, // 5  Mid Tom     → X2 sample   (no native tom)
			76, // 6  High Tom    → FM synth    (no native tom)
			60, // 7  Crash       → Crash
			64, // 8  Ride        → Ride
			52, // 9  Clap        → Clap
			56, // 10 Rimshot     → Rim
			56, // 11 Cowbell     → Rim         (no native cowbell)
			56, // 12 Clave       → Rim         (no native clave)
			44, // 13 Maracas     → Closed HH   (no native maracas)
			68, // 14 Low Conga   → X1 sample   (no native conga)
			72, // 15 High Conga  → X2 sample   (no native conga)
		},
	},
}

// KitNames returns the list of available kit names, in stable order.
func KitNames() []string { return []string{"gm", "rd8", "tr8s", "er1", "alphabase"} }

// GetKit returns the named kit, defaulting to the GM kit when the name is
// unknown or empty.
func GetKit(name string) Kit {
	if k, ok := kits[name]; ok {
		return k
	}
	return kits[DefaultKit]
}
