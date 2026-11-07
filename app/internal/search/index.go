// Package search is an in-memory fuzzy index over the dictionary.
//
// The dictionary is small (thousands of entries) and read-only between
// deploys, so a linear scan over pre-folded terms is fast enough and needs
// no external engine.
package search

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/textnorm"
)

// maxQueryRunes bounds the work a single query can cause.
const maxQueryRunes = 64

// Hit is one search result; lower Score is better.
type Hit struct {
	Key   string
	Score int
}

type term struct {
	s     string
	runes []rune
}

type doc struct {
	key   string
	order string // folded lemma, the tie-breaker
	terms []term
}

// Index is safe for concurrent reads after New returns.
type Index struct{ docs []doc }

// New indexes each entry's lemma, forms, aliases and Russian meanings (each meaning
// whole and word by word).
func New(entries []dict.Entry) *Index {
	ix := &Index{docs: make([]doc, 0, len(entries))}
	for _, e := range entries {
		seen := map[string]bool{}
		var terms []term
		add := func(s string) {
			if f := textnorm.Fold(s); f != "" && !seen[f] {
				seen[f] = true
				terms = append(terms, term{f, []rune(f)})
			}
		}
		add(e.Lemma)
		add(e.Key)
		for _, f := range e.Forms {
			add(f)
		}
		for _, a := range e.Aliases {
			add(a)
		}
		for _, m := range e.RU {
			add(m)
			for _, w := range strings.FieldsFunc(m, func(r rune) bool {
				return r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < 0x400 || r > 0x4FF)
			}) {
				add(w)
			}
		}
		ix.docs = append(ix.docs, doc{key: e.Key, order: textnorm.Fold(e.Lemma), terms: terms})
	}
	return ix
}

// Search returns up to limit entries matching q, best first. keep, when not
// nil, filters entries by key (for example: only hidden words).
func (ix *Index) Search(q string, keep func(key string) bool, limit int) []Hit {
	fq := textnorm.Fold(q)
	if fq == "" || limit <= 0 || utf8.RuneCountInString(fq) > maxQueryRunes {
		return nil
	}
	qr := []rune(fq)
	maxDist := allowedDistance(len(qr))
	query := query{term{fq, qr}, newPattern(qr)}

	type scored struct {
		Hit
		order string
	}
	var hits []scored
	for _, d := range ix.docs {
		if keep != nil && !keep(d.key) {
			continue
		}
		best := -1
		for _, t := range d.terms {
			if s := score(query, t, maxDist); s >= 0 && (best < 0 || s < best) {
				best = s
				if best == 0 {
					break
				}
			}
		}
		if best >= 0 {
			hits = append(hits, scored{Hit{d.key, best}, d.order})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score < hits[j].Score
		}
		return hits[i].order < hits[j].order
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Hit, len(hits))
	for i, h := range hits {
		out[i] = h.Hit
	}
	return out
}

// allowedDistance is the typo budget for a query of n runes.
func allowedDistance(n int) int {
	switch {
	case n <= 3:
		return 0
	case n <= 7:
		return 1
	default:
		return 2
	}
}

// score ranks how term matches q: 0 exact, 1 prefix, 2 substring,
// 2+d for an edit distance d within budget, -1 for no match.
type query struct {
	term
	pat *pattern
}

func score(q query, t term, maxDist int) int {
	switch {
	case q.s == t.s:
		return 0
	case strings.HasPrefix(t.s, q.s):
		return 1
	case strings.Contains(t.s, q.s):
		return 2
	}
	if maxDist == 0 {
		return -1
	}
	if d := len(q.runes) - len(t.runes); d > maxDist || -d > maxDist {
		return -1 // the length gap alone exceeds the budget
	}
	if d := q.pat.distance(t.runes); d <= maxDist {
		return 2 + d
	}
	return -1
}
