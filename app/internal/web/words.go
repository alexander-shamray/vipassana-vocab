package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/auth"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/web/views"
)

const (
	markHidden    = "hidden"
	markImportant = "important"

	pageSize    = 100
	searchLimit = 200
)

type view struct {
	name, title, path string
	mark              string // the mark a word must carry to appear; "" = none
}

var (
	viewMain      = view{"main", "Словарь", "/", ""}
	viewImportant = view{"important", "Важные", "/important", markImportant}
	viewHidden    = view{"hidden", "Выученные", "/hidden", markHidden}
)

// list serves one of the three word lists: the full page, or only the rows
// when HTMX asks for the next page or a search result.
func (s *server) list(v view) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, loggedIn := s.CurrentUser(r)
		marks := map[string]string{}
		if loggedIn {
			var err error
			if marks, err = s.marksFor(r.Context(), user.ID); err != nil {
				serverError(w, err)
				return
			}
		}
		keep := func(key string) bool { return marks[key] == v.mark }

		q := strings.TrimSpace(r.FormValue("q"))
		var rows []*dict.Entry
		next := ""
		if q != "" {
			for _, h := range s.Index.Search(q, keep, searchLimit) {
				rows = append(rows, s.byKey[h.Key])
			}
		} else {
			rows, next = s.page(keep, r.FormValue("after"), r.FormValue("from"))
		}

		p := views.ListPage{
			View: v.name, Title: v.title, Path: v.path, Q: q,
			LoggedIn: loggedIn, UserName: displayName(user),
			Rows: s.rows(rows), NextURL: nextURL(v.path, next),
			Letters: letters(),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var err error
		if isHTMX(r) && r.Header.Get("HX-Target") != "body" {
			err = views.Rows(p).Render(r.Context(), w)
		} else {
			err = views.List(p).Render(r.Context(), w)
		}
		if err != nil {
			slog.Error("render list", "err", err)
		}
	}
}

// page returns up to pageSize kept entries in alphabetical order, starting
// after the entry `after` or at the first entry whose folded lemma is >= from.
func (s *server) page(keep func(string) bool, after, from string) ([]*dict.Entry, string) {
	start := 0
	switch {
	case after != "":
		if o, ok := s.order[after]; ok {
			start = sort.Search(len(s.sorted), func(i int) bool { return s.order[s.sorted[i].Key] > o })
		}
	case from != "":
		f := strings.ToLower(from)
		start = sort.Search(len(s.sorted), func(i int) bool { return s.order[s.sorted[i].Key] >= f })
	}
	var out []*dict.Entry
	for i := start; i < len(s.sorted); i++ {
		if !keep(s.sorted[i].Key) {
			continue
		}
		if len(out) == pageSize {
			return out, out[len(out)-1].Key
		}
		out = append(out, s.sorted[i])
	}
	return out, ""
}

func (s *server) rows(es []*dict.Entry) []views.Row {
	out := make([]views.Row, 0, len(es))
	for _, e := range es {
		out = append(out, views.Row{Entry: *e, AudioURL: s.audioURL(e)})
	}
	return out
}

func (s *server) audioURL(e *dict.Entry) string {
	if s.AudioBaseURL == "" || e.Audio == "" {
		return "" // the browser falls back to speech synthesis
	}
	return s.AudioBaseURL + "/" + url.PathEscape(e.Audio)
}

func nextURL(path, after string) string {
	if after == "" {
		return ""
	}
	return path + "?after=" + url.QueryEscape(after)
}

func letters() []string {
	out := make([]string, 0, 26)
	for c := 'a'; c <= 'z'; c++ {
		out = append(out, string(c))
	}
	return out
}

// mark sets (or with mark == "" clears) the signed-in user's mark on a word.
// HTMX gets an empty 200 so the row disappears; a plain form post is sent back.
func (s *server) mark(mark string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.CurrentUser(r)
		if !ok {
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		key := r.PathValue("key")
		if _, ok := s.byKey[key]; !ok {
			http.NotFound(w, r)
			return
		}
		if err := s.setMark(r.Context(), user.ID, key, mark); err != nil {
			serverError(w, err)
			return
		}
		if isHTMX(r) {
			w.WriteHeader(http.StatusOK)
			return
		}
		back := r.Referer()
		if back == "" {
			back = "/"
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// setMark sets a user's mark on a word; mark "" clears it.
func (s *server) setMark(ctx context.Context, userID int64, key, mark string) error {
	if mark == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM word_marks WHERE user_id = ? AND word_key = ?`, userID, key)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO word_marks (user_id, word_key, mark) VALUES (?, ?, ?)
		ON CONFLICT (user_id, word_key) DO UPDATE
		SET mark = excluded.mark, marked_at = excluded.marked_at`, userID, key, mark)
	return err
}

func (s *server) marksFor(ctx context.Context, userID int64) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT word_key, mark FROM word_marks WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

func displayName(u auth.User) string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

func serverError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
}
