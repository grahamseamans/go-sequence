package devices

// NumPatterns is the fixed number of patterns per device, per track.
const NumPatterns = 16

// Schedule describes which pattern is currently playing on a track and
// which (if any) is queued to play next. Queued == -1 means "nothing queued".
type Schedule struct {
	Playing int `json:"playing"`
	Queued  int `json:"queued"`
}

// Validate clamps Schedule fields into valid ranges given the total pattern
// count. Playing is clamped to [0, numPatterns-1]. Queued is clamped to
// [-1, numPatterns-1] — -1 meaning "nothing queued".
func (s *Schedule) Validate(numPatterns int) {
	if numPatterns < 1 {
		numPatterns = 1
	}
	if s.Playing < 0 {
		s.Playing = 0
	} else if s.Playing > numPatterns-1 {
		s.Playing = numPatterns - 1
	}
	if s.Queued < -1 {
		s.Queued = -1
	} else if s.Queued > numPatterns-1 {
		s.Queued = numPatterns - 1
	}
}
