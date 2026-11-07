package search

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
)

func sampleIndex(t testing.TB) *Index {
	t.Helper()
	f, err := os.Open("../../testdata/words.sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var es []dict.Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e dict.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		es = append(es, e)
	}
	return New(es)
}

func first(hits []Hit) string {
	if len(hits) == 0 {
		return ""
	}
	return hits[0].Key
}

func TestSearch_Typo(t *testing.T) {
	if got := first(sampleIndex(t).Search("equanimty", nil, 10)); got != "equanimity" {
		t.Errorf("equanimty → %q, want equanimity", got)
	}
}

func TestSearch_PaliWithAndWithoutDiacritics(t *testing.T) {
	ix := sampleIndex(t)
	for _, q := range []string{"panna", "PAÑÑĀ", "pañña"} {
		if got := first(ix.Search(q, nil, 10)); got != "panna" {
			t.Errorf("%q → %q, want panna", q, got)
		}
	}
}

func TestSearch_Russian(t *testing.T) {
	if got := first(sampleIndex(t).Search("страда", nil, 10)); got != "suffer" && got != "dukkha" {
		t.Errorf("страда → %q, want suffer or dukkha", got)
	}
	hits := sampleIndex(t).Search("страдать", nil, 10)
	if first(hits) != "suffer" {
		t.Errorf("страдать → %v, want suffer first", hits)
	}
}

func TestSearch_PrefixBeatsLongerWord(t *testing.T) {
	hits := sampleIndex(t).Search("suf", nil, 10)
	if len(hits) < 2 || hits[0].Key != "suffer" || hits[1].Key != "sufficient" {
		t.Errorf("suf → %v, want suffer then sufficient", hits)
	}
}

func TestSearch_KeepFilters(t *testing.T) {
	hidden := map[string]bool{"sufficient": true}
	hits := sampleIndex(t).Search("suf", func(k string) bool { return hidden[k] }, 10)
	if len(hits) != 1 || hits[0].Key != "sufficient" {
		t.Errorf("keep filter → %v, want only sufficient", hits)
	}
}

func TestSearch_DegenerateQueries(t *testing.T) {
	ix := sampleIndex(t)
	for _, q := range []string{"", "   ", strings.Repeat("a", 500), "\x00", "ё"} {
		_ = ix.Search(q, nil, 10) // must not panic
	}
	if ix.Search("", nil, 10) != nil || ix.Search("   ", nil, 10) != nil {
		t.Error("blank query must return nil")
	}
	if ix.Search(strings.Repeat("a", 500), nil, 10) != nil {
		t.Error("oversized query must return nil")
	}
}

func TestSearch_Limit(t *testing.T) {
	if n := len(sampleIndex(t).Search("a", nil, 3)); n != 3 {
		t.Errorf("limit 3 returned %d", n)
	}
}

func TestSearch_MultiwordTerm(t *testing.T) {
	if got := first(sampleIndex(t).Search("mind and mater", nil, 10)); got != "mind-and-matter" {
		t.Errorf("mind and mater → %q", got)
	}
}

func BenchmarkSearch8000(b *testing.B) {
	es := make([]dict.Entry, 8000)
	for i := range es {
		w := fmt.Sprintf("word%05dxyz", i)
		es[i] = dict.Entry{Key: w, Lemma: w, Forms: []string{w + "s", w + "ed"}, RU: []string{"слово номер", "перевод"}}
	}
	ix := New(es)
	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		ix.Search("wrd01234xyz", nil, 100)
	}
	if per := time.Since(start) / time.Duration(b.N); per > 5*time.Millisecond {
		b.Errorf("search took %v per query, want < 5ms", per)
	}
}
