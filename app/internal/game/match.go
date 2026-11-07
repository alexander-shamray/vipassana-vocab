// Package game checks answers in the vocabulary games.
//
// Answers are typed or come from speech recognition, so matching forgives
// case, diacritics, "ё", brackets, filler words around the answer and a
// typo or two in longer words.
package game

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/search"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/textnorm"
)

// minWordRunes is the shortest single word of a meaning accepted on its own
// ("желание" for "страстное желание", but not "ум" for "ум и материя").
const minWordRunes = 4

var brackets = regexp.MustCompile(`\([^)]*\)|«|»|"`)

// typos is how many edits a target of n runes tolerates.
func typos(n int) int {
	switch {
	case n >= 10:
		return 2
	case n >= 5:
		return 1
	default:
		return 0
	}
}

func near(answer, target string) bool {
	if answer == target {
		return true
	}
	budget := typos(utf8.RuneCountInString(target))
	if budget == 0 {
		return false
	}
	if d := utf8.RuneCountInString(answer) - utf8.RuneCountInString(target); d > budget || -d > budget {
		return false
	}
	return search.Distance(answer, target) <= budget
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && r != '-' })
}

// meaningTargets turns "санкхара — обусловленная реакция ума" or
// "ощущение (телесное, ведана)" into the phrases and words a learner may say.
func meaningTargets(meanings []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s = textnorm.Fold(s); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, m := range meanings {
		m = brackets.ReplaceAllString(m, " ")
		for _, part := range strings.FieldsFunc(m, func(r rune) bool { return r == '—' || r == ';' || r == ',' }) {
			add(part)
			for _, w := range words(part) {
				if utf8.RuneCountInString(w) >= minWordRunes {
					add(w)
				}
			}
		}
	}
	return out
}

// matches reports whether any answer, whole or word by word, is near a target.
func matches(answers []string, targets []string) bool {
	for _, a := range answers {
		fa := textnorm.Fold(brackets.ReplaceAllString(a, " "))
		if fa == "" {
			continue
		}
		tries := append([]string{fa}, words(fa)...)
		for _, t := range targets {
			for _, try := range tries {
				if near(try, t) {
					return true
				}
			}
		}
	}
	return false
}

// CheckRussian reports whether one of the answers translates e.
func CheckRussian(e dict.Entry, answers ...string) bool {
	return matches(answers, meaningTargets(e.RU))
}

// CheckEnglish reports whether one of the answers names e: its lemma, a
// form heard in the talks or a known spelling.
func CheckEnglish(e dict.Entry, answers ...string) bool {
	var targets []string
	for _, s := range append(append([]string{e.Lemma, e.Key}, e.Forms...), e.Aliases...) {
		if f := textnorm.Fold(strings.ReplaceAll(s, "-", " ")); f != "" {
			targets = append(targets, f)
		}
	}
	return matches(answers, targets)
}

// RussianPrompt is what the "say it in English" game speaks: the first
// meaning without brackets and without a leading Pali transcription
// ("санкхара — обусловленная реакция ума" → "обусловленная реакция ума").
func RussianPrompt(e dict.Entry) string {
	if len(e.RU) == 0 {
		return ""
	}
	m := e.RU[0]
	if i := strings.Index(m, "—"); i >= 0 {
		m = m[i+len("—"):]
	}
	return strings.Join(strings.Fields(brackets.ReplaceAllString(m, " ")), " ")
}
