package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const vowels = "аеиоуыэюя"

var markedWord = regexp.MustCompile(`[а-яёА-ЯЁ+]+`)

func isVowel(r rune) bool { return strings.ContainsRune(vowels, unicode.ToLower(r)) }

func vowelCount(word string) int {
	n := 0
	for _, r := range strings.ToLower(word) {
		if strings.ContainsRune(vowels+"ё", r) {
			n++
		}
	}
	return n
}

func isWordRune(r rune) bool {
	return r == '+' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// dropRepeats removes a word said twice with only a space between ("это это"): the narrator's stumble, not the text.
func dropRepeats(text string) string {
	rs := []rune(text)
	var out []rune
	for i := 0; i < len(rs); {
		if !isWordRune(rs[i]) {
			out = append(out, rs[i])
			i++
			continue
		}
		end := i
		for end < len(rs) && isWordRune(rs[end]) {
			end++
		}
		out = append(out, rs[i:end]...)
		next := end + 1
		if end < len(rs) && rs[end] == ' ' {
			last := next
			for last < len(rs) && isWordRune(rs[last]) {
				last++
			}
			if last > next && strings.EqualFold(string(rs[i:end]), string(rs[next:last])) {
				end = last
			}
		}
		i = end
	}
	return string(out)
}

// fixMarks repairs stress marks: a "+" not before a vowel, in a one-vowel word, in a word with ё, or a second one.
func fixMarks(text string) string {
	text = strings.NewReplacer(string(rune(0x200b)), "", string(rune(0x200c)), "", string(rune(0x200d)), "", string(rune(0xfeff)), "").Replace(text)
	text = dropRepeats(text)
	return markedWord.ReplaceAllStringFunc(text, func(w string) string {
		low := strings.ToLower(w)
		if vowelCount(low) < 2 || strings.Contains(low, "ё") {
			return strings.ReplaceAll(w, "+", "")
		}
		rs := []rune(w)
		var out []rune
		seen := false
		for i, r := range rs {
			if r == '+' {
				if !seen && i+1 < len(rs) && isVowel(rs[i+1]) {
					out = append(out, r)
					seen = true
				}
				continue
			}
			out = append(out, r)
		}
		return string(out)
	})
}

// markAt is the position of the marked vowel in the word without "+", or -1.
func markAt(word string) int {
	for i, r := range []rune(word) {
		if r == '+' {
			return i
		}
	}
	return -1
}

type phrase struct {
	ID       int     `json:"id"`
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Text     string  `json:"text"`
	RuMarked string  `json:"ru_marked,omitempty"`
	EnID     *int    `json:"en_id,omitempty"`
	RuAccent string  `json:"ruaccent,omitempty"`
}

func readJSON(path string, into any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	return enc.Encode(v)
}

func loadFixes() (map[string]int, error) {
	var list []string
	if err := readJSON(pyDir+"../stress-fixes.json", &list); err != nil {
		return nil, err
	}
	fixes := make(map[string]int, len(list))
	for _, f := range list {
		fixes[strings.ToLower(strings.ReplaceAll(f, "+", ""))] = markAt(f)
	}
	return fixes, nil
}

// markWord chooses the stress of one word: the reviewed list first, then RuAccent, then Claude.
func markWord(claude, ruaccent string, fixes map[string]int, counts map[string]int) string {
	plain := strings.ReplaceAll(claude, "+", "")
	low := strings.ToLower(plain)
	if vowelCount(low) < 2 || strings.Contains(low, "ё") {
		return plain
	}
	at, source := -1, ""
	if f, ok := fixes[low]; ok {
		at, source = f, "fixed"
	} else if m := markAt(ruaccent); m >= 0 {
		at, source = m, "ruaccent"
	} else {
		at, source = markAt(claude), "claude"
	}
	rs := []rune(plain)
	if at < 0 || at >= len(rs) || !isVowel(rs[at]) {
		counts["none"]++
		return plain
	}
	counts[source]++
	return string(rs[:at]) + "+" + string(rs[at:])
}

// runMarks writes the final stress marks. RuAccent's win over Claude's: on 137 disagreeing word forms in lesson 1
// Claude's were mostly wrong. Words reviewed by hand in stress-fixes.json win over both.
func runMarks(l lesson) error {
	var src []phrase
	if err := readJSON(l.At("full-ru-accent.json"), &src); err != nil {
		return err
	}
	fixes, err := loadFixes()
	if err != nil {
		return err
	}
	backup := l.At("full-ru.claude.json")
	if _, err := os.Stat(backup); err != nil {
		raw, err := os.ReadFile(l.At("full-ru.json"))
		if err != nil {
			return err
		}
		if err := os.WriteFile(backup, raw, 0o644); err != nil {
			return err
		}
	}
	counts := map[string]int{}
	var out []phrase
	var dropped []int
	for _, p := range src {
		theirs := markedWord.FindAllString(p.RuAccent, -1)
		k := 0
		marked := markedWord.ReplaceAllStringFunc(fixMarks(p.RuMarked), func(w string) string {
			other := ""
			if k < len(theirs) {
				other = theirs[k]
			}
			k++
			return markWord(w, other, fixes, counts)
		})
		if !markedWord.MatchString(marked) { // Whisper heard a subtitle credit in silence and Claude emptied it
			dropped = append(dropped, p.ID)
			continue
		}
		p.RuMarked, p.RuAccent = marked, ""
		out = append(out, p)
	}
	if err := writeJSON(l.At("full-ru.json"), out); err != nil {
		return err
	}
	fmt.Println("phrases", len(out), "dropped as empty", dropped, "marks", counts)
	return nil
}

// runReview prints the stress disagreements no earlier lesson has shown, to check by ear or by a dictionary.
// Add the right form to stress-fixes.json, then run "dub marks" again.
func runReview(l lesson) error {
	var diff [][]any
	if err := readJSON(l.At("accent-diff.json"), &diff); err != nil {
		return err
	}
	seen := map[string]bool{}
	others, _ := filepath.Glob("work*/accent-diff.json")
	others = append(others, "accent-diff.json")
	for _, path := range others {
		if filepath.Clean(path) == filepath.Clean(l.At("accent-diff.json")) {
			continue
		}
		var rows [][]any
		if readJSON(path, &rows) != nil {
			continue
		}
		for _, r := range rows {
			seen[fmt.Sprint(r[0], "|", r[1])] = true
		}
	}
	fixes, err := loadFixes()
	if err != nil {
		return err
	}
	var lines []string
	for _, r := range diff {
		claude, ruaccent := fmt.Sprint(r[0]), fmt.Sprint(r[1])
		word := strings.ReplaceAll(claude, "+", "")
		_, fixed := fixes[word]
		if vowelCount(word) < 2 || strings.Contains(word, "ё") || fixed || seen[claude+"|"+ruaccent] || !strings.Contains(ruaccent, "+") {
			continue
		}
		lines = append(lines, fmt.Sprintf("%-28s RuAccent (it wins): %-28s times: %v", "Claude: "+claude, ruaccent, r[2]))
	}
	sort.Strings(lines)
	fmt.Println(strings.Join(lines, "\n"))
	fmt.Println(len(lines), "new disagreements")
	return nil
}
