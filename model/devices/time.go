package devices

const PPQ = 960 // ticks per quarter note

// StepToTick converts a 16th-note step index to ticks.
func StepToTick(step int) int64 { return int64(step) * (PPQ / 4) }

// TickToStep converts ticks back to a 16th-note step index.
func TickToStep(tick int64) int { return int(tick / (PPQ / 4)) }
