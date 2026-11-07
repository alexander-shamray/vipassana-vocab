package search

import (
	"math/rand"
	"testing"
)

func TestMyers_MatchesPlainDP(t *testing.T) {
	alphabet := []rune("abcñāей")
	rng := rand.New(rand.NewSource(1))
	gen := func(n int) []rune {
		r := make([]rune, n)
		for i := range r {
			r[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return r
	}
	for i := 0; i < 20000; i++ {
		a, b := gen(1+rng.Intn(12)), gen(rng.Intn(14))
		want := within(a, b, 100)
		if got := newPattern(a).distance(b); got != want {
			t.Fatalf("distance(%q, %q) = %d, want %d", string(a), string(b), got, want)
		}
	}
	long := gen(64)
	if got := newPattern(long).distance(long); got != 0 {
		t.Fatalf("64-rune self distance = %d", got)
	}
}
