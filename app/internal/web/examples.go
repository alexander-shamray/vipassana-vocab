package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/web/views"
)

// word serves a word's forms and quotes: the dialog body for HTMX (desktop),
// a full page otherwise (phones, no JS). Quotes need a signed-in user.
func (s *server) word(w http.ResponseWriter, r *http.Request) {
	e, ok := s.byKey[r.PathValue("key")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, loggedIn := s.CurrentUser(r)
	p := views.WordPage{Row: views.Row{Entry: *e, AudioURL: s.audioURL(e)}, LoggedIn: loggedIn}
	if loggedIn {
		exs, err := dict.ExamplesFor(r.Context(), s.DB, e.Key)
		if err != nil {
			serverError(w, err)
			return
		}
		for _, ex := range exs {
			p.Examples = append(p.Examples, views.Quote{
				Parts: Highlight(ex.Text, append([]string{e.Lemma}, e.Forms...)),
				Talk:  ex.Talk,
				At:    clock(ex.T),
				Link:  fmt.Sprintf("https://youtu.be/%s?t=%d", ex.VideoID, ex.T),
			})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var err error
	if isHTMX(r) {
		err = views.WordBody(p).Render(r.Context(), w)
	} else {
		err = views.Word(p).Render(r.Context(), w)
	}
	if err != nil {
		slog.Error("render word", "err", err)
	}
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := views.Login(s.Providers).Render(r.Context(), w); err != nil {
		slog.Error("render login", "err", err)
	}
}

func (s *server) notInvited(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	if err := views.NotInvited().Render(r.Context(), w); err != nil {
		slog.Error("render not-invited", "err", err)
	}
}

// Highlight splits text into parts, marking whole-word, case-insensitive
// occurrences of any of the forms. The parts are rendered as text, never as
// HTML, so quotes cannot inject markup.
func Highlight(text string, forms []string) []views.Part {
	want := map[string]bool{}
	for _, f := range forms {
		if f = strings.ToLower(strings.TrimSpace(f)); f != "" {
			want[f] = true
		}
	}
	var parts []views.Part
	add := func(s string, mark bool) {
		if s == "" {
			return
		}
		if n := len(parts); n > 0 && parts[n-1].Mark == mark {
			parts[n-1].Text += s
			return
		}
		parts = append(parts, views.Part{Text: s, Mark: mark})
	}
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsMark(r) || r == '\'' }
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !isWord(r) {
			add(text[i:i+size], false)
			i += size
			continue
		}
		j := i
		for j < len(text) {
			r2, s2 := utf8.DecodeRuneInString(text[j:])
			if !isWord(r2) {
				break
			}
			j += s2
		}
		word := text[i:j]
		add(word, want[strings.ToLower(word)])
		i = j
	}
	// Multi-word lemmas ("mind and matter") are marked phrase by phrase.
	for f := range want {
		if strings.Contains(f, " ") {
			return highlightPhrase(parts, f)
		}
	}
	return parts
}

func highlightPhrase(parts []views.Part, phrase string) []views.Part {
	var plain strings.Builder
	for _, p := range parts {
		plain.WriteString(p.Text)
	}
	text := plain.String()
	lower := strings.ToLower(text)
	if len(lower) != len(text) {
		return parts // lower-casing changed byte offsets; skip phrase marking
	}
	idx := strings.Index(lower, phrase)
	if idx < 0 {
		return parts
	}
	end := idx + len(phrase)
	return []views.Part{{Text: text[:idx]}, {Text: text[idx:end], Mark: true}, {Text: text[end:]}}
}

// clock formats seconds as m:ss or h:mm:ss.
func clock(sec int) string {
	h, m, s := sec/3600, sec%3600/60, sec%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
