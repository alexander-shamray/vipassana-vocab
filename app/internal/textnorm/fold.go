// Package textnorm folds text for matching and builds stable word keys.
//
// The rules must stay identical to pipeline/vv_pipeline/textnorm.py; both
// are tested against the shared cases in testdata/fold_cases.json.
package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold lower-cases s, strips diacritics (paññā → panna, ё → е), collapses
// whitespace and trims it.
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue // a combining mark: the diacritic itself
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// Key is the stable identifier of a dictionary entry: Fold with spaces
// replaced by hyphens ("Mind and Matter" → "mind-and-matter").
func Key(s string) string { return strings.ReplaceAll(Fold(s), " ", "-") }
