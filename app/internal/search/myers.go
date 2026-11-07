package search

// pattern holds the per-character bit masks for Myers' bit-parallel edit
// distance (Hyyrö's formulation). It works for patterns of up to 64 runes,
// which maxQueryRunes guarantees.
type pattern struct {
	m     int
	ascii [128]uint64
	other map[rune]uint64
}

func newPattern(q []rune) *pattern {
	p := &pattern{m: len(q)}
	for i, r := range q {
		bit := uint64(1) << uint(i)
		if r < 128 {
			p.ascii[r] |= bit
		} else {
			if p.other == nil {
				p.other = map[rune]uint64{}
			}
			p.other[r] |= bit
		}
	}
	return p
}

func (p *pattern) eq(r rune) uint64 {
	if r < 128 {
		return p.ascii[r]
	}
	return p.other[r] // nil map reads are fine
}

// distance returns the Levenshtein distance between the pattern and t.
// One pass over t, a handful of word operations per rune.
func (p *pattern) distance(t []rune) int {
	pv, mv := ^uint64(0), uint64(0)
	score := p.m
	last := uint64(1) << uint(p.m-1)
	for _, c := range t {
		eq := p.eq(c)
		xv := eq | mv
		xh := (((eq & pv) + pv) ^ pv) | eq
		ph := mv | ^(xh | pv)
		mh := pv & xh
		if ph&last != 0 {
			score++
		} else if mh&last != 0 {
			score--
		}
		ph = ph<<1 | 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv
	}
	return score
}
