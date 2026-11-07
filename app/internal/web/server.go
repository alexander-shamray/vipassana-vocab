// Package web serves the dictionary pages.
package web

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"
	"sort"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/auth"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/search"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/textnorm"
)

//go:embed static
var staticFS embed.FS

// Deps is everything the handlers need. Tests replace CurrentUser.
type Deps struct {
	DB           *sql.DB
	Entries      []dict.Entry
	Index        *search.Index
	CurrentUser  func(*http.Request) (auth.User, bool)
	Providers    []auth.Provider
	AudioBaseURL string
	// AuthRoutes registers /auth/* and /logout; nil in tests.
	AuthRoutes func(*http.ServeMux)
}

type server struct {
	Deps
	byKey  map[string]*dict.Entry
	sorted []*dict.Entry // alphabetical by folded lemma, then key
	order  map[string]string
}

// New builds the HTTP handler.
func New(d Deps) http.Handler {
	s := &server{Deps: d, byKey: map[string]*dict.Entry{}, order: map[string]string{}}
	for i := range d.Entries {
		e := &d.Entries[i]
		s.byKey[e.Key] = e
		s.order[e.Key] = sortKey(e)
		s.sorted = append(s.sorted, e)
	}
	sort.Slice(s.sorted, func(i, j int) bool {
		return s.order[s.sorted[i].Key] < s.order[s.sorted[j].Key]
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.list(viewMain))
	mux.HandleFunc("GET /important", s.list(viewImportant))
	mux.HandleFunc("GET /hidden", s.list(viewHidden))
	mux.HandleFunc("POST /words/{key}/hide", s.mark(markHidden))
	mux.HandleFunc("POST /words/{key}/important", s.mark(markImportant))
	mux.HandleFunc("POST /words/{key}/restore", s.mark(""))
	mux.HandleFunc("GET /words/{key}", s.word)
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("GET /not-invited", s.notInvited)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static))))
	if d.AuthRoutes != nil {
		d.AuthRoutes(mux)
	}
	// Rejects cross-site POSTs (CSRF) using Sec-Fetch-Site / Origin.
	return http.NewCrossOriginProtection().Handler(securityHeaders(mux))
}

func sortKey(e *dict.Entry) string { return textnorm.Fold(e.Lemma) + "\x00" + e.Key }

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		h.ServeHTTP(w, r)
	})
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }
