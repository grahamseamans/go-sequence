package devices

// Rng is an xorshift64 PRNG. Deterministic given a seed. Fast, decent
// statistical quality, much better than math/rand's default for our purposes.
//
// Not safe for concurrent use — give each Compile call its own Rng.
type Rng struct{ state uint64 }

func NewRng(seed uint64) *Rng {
	r := &Rng{}
	r.Seed(seed)
	return r
}

func (r *Rng) Seed(s uint64) { r.state = s | 1 } // avoid 0 state

func (r *Rng) Next() uint32 {
	r.state ^= r.state << 13
	r.state ^= r.state >> 7
	r.state ^= r.state << 17
	return uint32(r.state)
}

// Probability returns true with probability pct/100. pct clamped to [0,100].
func (r *Rng) Probability(pct uint8) bool {
	if pct == 0 {
		return false
	}
	if pct >= 100 {
		return true
	}
	return r.Next()%100 < uint32(pct)
}

// IntN returns a uniformly distributed integer in [0, n). Panics if n <= 0.
func (r *Rng) IntN(n int) int {
	if n <= 0 {
		panic("IntN: n must be positive")
	}
	return int(r.Next()) % n
}
