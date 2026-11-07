// Package views holds the templ components and the data they render.
package views

import (
	"net/url"
	"strings"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/textnorm"
)

// Row is one word in a list.
type Row struct {
	Entry    dict.Entry
	AudioURL string // empty: the browser speaks the lemma itself
}

// ListPage is the main, important or hidden list.
type ListPage struct {
	View     string // "main", "important", "hidden"
	Title    string
	Path     string
	Q        string
	LoggedIn bool
	UserName string
	Rows     []Row
	NextURL  string // empty when there is no next page
	Letters  []string
}

// Part is a run of quote text, highlighted when it is the word itself.
type Part struct {
	Text string
	Mark bool
}

// Quote is one example from a talk.
type Quote struct {
	Parts []Part
	Talk  int
	At    string // m:ss
	Link  string // YouTube at that second
}

// WordPage shows a word's forms and quotes.
type WordPage struct {
	Row      Row
	LoggedIn bool
	UserName string
	Examples []Quote
}

func wordPath(key, action string) string {
	p := "/words/" + url.PathEscape(key)
	if action != "" {
		p += "/" + action
	}
	return p
}

func rowID(key string) string { return "w-" + url.PathEscape(key) }

func meanings(e dict.Entry) string { return strings.Join(e.RU, "; ") }

// otherForms lists, comma-separated, the forms that differ from the lemma
// beyond case and diacritics (so "anatta" is not repeated under "anattā").
func otherForms(e dict.Entry) string {
	lemma := textnorm.Fold(e.Lemma)
	var out []string
	for _, f := range e.Forms {
		if textnorm.Fold(f) != lemma {
			out = append(out, f)
		}
	}
	return strings.Join(out, ", ")
}

// allForms lists every form heard in the talks; the lemma alone when the
// pipeline recorded none.
func allForms(e dict.Entry) []string {
	if len(e.Forms) == 0 {
		return []string{e.Lemma}
	}
	return e.Forms
}

func speakLang(e dict.Entry) string {
	if e.Lang == "pali" {
		return "en-IN"
	}
	return "en-GB"
}
