package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/auth"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/game"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/web/views"
)

const (
	gameSize      = 10
	maxFormBytes  = 8 << 10 // ten answers fit many times over
	maxAnswerRune = 200
)

// parseGameForm reads a small form; answers longer than maxAnswerRune are cut.
func parseGameForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return false
	}
	for i, a := range r.PostForm["answer"] {
		if rs := []rune(a); len(rs) > maxAnswerRune {
			r.PostForm["answer"][i] = string(rs[:maxAnswerRune])
		}
	}
	return true
}

const (
	modeListen = "listen" // hear English, say Russian
	modeSpeak  = "speak"  // hear Russian, say English
)

func (s *server) gameRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /games", s.games)
	mux.HandleFunc("GET /games/write", s.writeGame)
	mux.HandleFunc("POST /games/write", s.writeCheck)
	mux.HandleFunc("GET /games/listen", s.speechGame(modeListen))
	mux.HandleFunc("GET /games/speak", s.speechGame(modeSpeak))
	mux.HandleFunc("POST /games/check", s.speechCheck)
	mux.HandleFunc("POST /games/important/{key}", s.sendToImportant)
}

// player returns the signed-in user or redirects to the login page.
func (s *server) player(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := s.CurrentUser(r)
	if !ok {
		if isHTMX(r) {
			w.Header().Set("HX-Redirect", "/login")
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	}
	return u, ok
}

// pick returns up to gameSize random words the user has not learnt yet and
// that have a translation to check against.
func (s *server) pick(ctx context.Context, userID int64) ([]*dict.Entry, error) {
	marks, err := s.marksFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	var pool []*dict.Entry
	for _, e := range s.sorted {
		if marks[e.Key] != markHidden && len(e.RU) > 0 {
			pool = append(pool, e)
		}
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > gameSize {
		pool = pool[:gameSize]
	}
	return pool, nil
}

func (s *server) games(w http.ResponseWriter, r *http.Request) {
	u, ok := s.player(w, r)
	if !ok {
		return
	}
	s.render(w, r, views.GamesMenu(displayName(u)))
}

func (s *server) writeGame(w http.ResponseWriter, r *http.Request) {
	u, ok := s.player(w, r)
	if !ok {
		return
	}
	words, err := s.pick(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	s.render(w, r, views.WriteGame(displayName(u), s.rows(words)))
}

// writeCheck grades the typed answers: form fields key and answer, paired
// by position.
func (s *server) writeCheck(w http.ResponseWriter, r *http.Request) {
	u, ok := s.player(w, r)
	if !ok {
		return
	}
	if !parseGameForm(w, r) {
		return
	}
	keys, answers := r.PostForm["key"], r.PostForm["answer"]
	if len(keys) != len(answers) || len(keys) > gameSize {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	var results []views.WriteResult
	for i, k := range keys {
		e, ok := s.byKey[k]
		if !ok {
			continue
		}
		a := strings.TrimSpace(answers[i])
		results = append(results, views.WriteResult{
			Row: views.Row{Entry: *e, AudioURL: s.audioURL(e)}, Answer: a,
			OK: a != "" && game.CheckRussian(*e, a),
		})
	}
	if isHTMX(r) {
		s.render(w, r, views.WriteResults(results))
		return
	}
	s.render(w, r, views.WriteResultsPage(displayName(u), results))
}

type speechItem struct {
	Key   string `json:"key"`
	Say   string `json:"say"`
	Lang  string `json:"lang"`
	Audio string `json:"audio,omitempty"`
}

func (s *server) speechGame(mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.player(w, r)
		if !ok {
			return
		}
		words, err := s.pick(r.Context(), u.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		items := make([]speechItem, 0, len(words))
		for _, e := range words {
			if mode == modeListen {
				lang := "en-GB"
				if e.Lang == "pali" {
					lang = "en-IN"
				}
				items = append(items, speechItem{e.Key, e.Lemma, lang, s.audioURL(e)})
			} else {
				items = append(items, speechItem{Key: e.Key, Say: game.RussianPrompt(*e), Lang: "ru-RU"})
			}
		}
		js, err := json.Marshal(items)
		if err != nil {
			serverError(w, err)
			return
		}
		s.render(w, r, views.SpeechGame(displayName(u), mode, string(js)))
	}
}

type checkResult struct {
	OK       bool   `json:"ok"`
	Lemma    string `json:"lemma"`    // the English word
	Expected string `json:"expected"` // its Russian meanings
}

// speechCheck grades one spoken answer; answer may repeat, one value per
// recognition alternative.
func (s *server) speechCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.player(w, r); !ok {
		return
	}
	if !parseGameForm(w, r) {
		return
	}
	e, found := s.byKey[r.PostForm.Get("key")]
	mode := r.PostForm.Get("mode")
	if !found || (mode != modeListen && mode != modeSpeak) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	answers := r.PostForm["answer"]
	if len(answers) > 10 {
		answers = answers[:10]
	}
	// Either way the player sees the pair: the word and what it means.
	res := checkResult{Lemma: e.Lemma, Expected: strings.Join(e.RU, "; ")}
	if mode == modeListen {
		res.OK = game.CheckRussian(*e, answers...)
	} else {
		res.OK = game.CheckEnglish(*e, answers...)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		slog.Error("encode check", "err", err)
	}
}

// sendToImportant marks a missed word important and answers with a short
// confirmation that replaces the clicked button.
func (s *server) sendToImportant(w http.ResponseWriter, r *http.Request) {
	u, ok := s.player(w, r)
	if !ok {
		return
	}
	e, found := s.byKey[r.PathValue("key")]
	if !found {
		http.NotFound(w, r)
		return
	}
	if err := s.setMark(r.Context(), u.ID, e.Key, markImportant); err != nil {
		serverError(w, err)
		return
	}
	s.render(w, r, views.SentToImportant(e.Lemma))
}
