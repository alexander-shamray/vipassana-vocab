// Command server runs the Vipassana Vocab site.
//
//	server serve                         run the site (config from the environment)
//	server import --words F [--examples F]  load the dictionary into the database
//	server dev                           import sample or real data and serve on localhost
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/auth"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/config"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/db"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/search"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := dispatch(ctx, os.Args[1:], os.Getenv); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, args []string, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: server serve | import --words FILE [--examples FILE] | dev")
	}
	switch args[0] {
	case "serve":
		// Production must say where it lives (cookies, OAuth callbacks) and
		// never carries the passwordless local sign-in.
		if getenv("DEV_LOGIN") != "" {
			return errors.New("serve: DEV_LOGIN is only for `server dev`")
		}
		if getenv("BASE_URL") == "" {
			return errors.New("serve: BASE_URL is required, e.g. https://<app>.fly.dev")
		}
		return serve(ctx, getenv, nil)
	case "import":
		return importCmd(ctx, args[1:], getenv)
	case "dev":
		return dev(ctx, getenv)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func importCmd(ctx context.Context, args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	words := fs.String("words", "", "path to words.jsonl")
	examples := fs.String("examples", "", "path to examples.jsonl (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *words == "" {
		return errors.New("import: --words is required")
	}
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	d, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer d.Close()
	return importFiles(ctx, d, *words, *examples)
}

func importFiles(ctx context.Context, d *sql.DB, words, examples string) error {
	if err := importOne(ctx, d, words, dict.ImportWords); err != nil {
		return err
	}
	if examples == "" {
		return nil
	}
	return importOne(ctx, d, examples, dict.ImportExamples)
}

func importOne(ctx context.Context, d *sql.DB, path string,
	f func(context.Context, *sql.DB, io.Reader) (dict.Stats, error)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	st, err := f(ctx, d, file)
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}
	slog.Info("imported", "file", path, "upserted", st.Upserted, "deleted", st.Deleted, "skipped", st.Skipped)
	return nil
}

// dev runs the site locally: sample data (or the real dictionary when it
// exists), a separate database file and the one-click DEV_LOGIN sign-in.
func dev(ctx context.Context, getenv func(string) string) error {
	defaults := map[string]string{
		"DATABASE_PATH":  "data/dev.sqlite",
		"BASE_URL":       "http://localhost:8080",
		"DEV_LOGIN":      "dev@localhost",
		"ALLOWED_EMAILS": "",
	}
	env := func(k string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return defaults[k]
	}
	cfg, err := config.Load(env)
	if err != nil {
		return err
	}
	d, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	words, examples := "testdata/words.sample.jsonl", "testdata/examples.sample.jsonl"
	if _, err := os.Stat("../dictionary/words.jsonl"); err == nil {
		words = "../dictionary/words.jsonl"
		examples = ""
		if _, err := os.Stat("../data/examples.jsonl"); err == nil {
			examples = "../data/examples.jsonl"
		}
	}
	err = importFiles(ctx, d, words, examples)
	d.Close()
	if err != nil {
		return err
	}
	// Loopback only: the passwordless sign-in must not be reachable from the LAN.
	ln, err := net.Listen("tcp", "127.0.0.1:"+cfg.Port)
	if err != nil {
		return err
	}
	slog.Info("dev server", "url", cfg.BaseURL, "login", cfg.DevLogin)
	return serve(ctx, env, ln)
}

// serve runs the site until ctx is cancelled. ln is for tests; nil listens
// on cfg.Port.
func serve(ctx context.Context, getenv func(string) string, ln net.Listener) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	d, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer d.Close()

	entries, err := dict.LoadAll(ctx, d)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		slog.Warn("dictionary is empty: run `server import --words ...` first")
	}

	sm := scs.New()
	sm.Store = sqlite3store.New(d)
	sm.Lifetime = 90 * 24 * time.Hour
	sm.Cookie.Name = "vv_session"
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.SecureCookies()
	sm.Cookie.Persist = true

	if cfg.SessionKey == "" {
		slog.Warn("SESSION_KEY is empty: OAuth state cookies are signed with an empty key; set it in production")
	}
	a := auth.New(cfg, d, sm)
	a.NotInvited = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/not-invited", http.StatusSeeOther)
	}

	handler := web.New(web.Deps{
		DB:           d,
		Entries:      entries,
		Index:        search.New(entries),
		CurrentUser:  a.CurrentUser,
		Providers:    a.Providers(),
		AudioBaseURL: cfg.AudioBaseURL,
		AuthRoutes:   a.Routes,
	})

	srv := &http.Server{
		Handler:           sm.LoadAndSave(handler),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if ln == nil {
		if ln, err = net.Listen("tcp", ":"+cfg.Port); err != nil {
			return err
		}
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	slog.Info("listening", "addr", ln.Addr().String(), "words", len(entries))

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
