// Package auth signs users in with Google or GitHub and keeps them signed in
// with a server-side session. Only invited e-mail addresses may sign up.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/config"
)

// ErrNotInvited means the provider vouched for an address that is not on
// the invite list.
var ErrNotInvited = errors.New("auth: e-mail is not invited")

const sessionUserKey = "uid"

// User is a signed-in person.
type User struct {
	ID    int64
	Email string
	Name  string
}

// Provider is a configured sign-in option for the login page.
type Provider struct{ Name, Title string }

// Auth wires OAuth providers to the session manager.
type Auth struct {
	cfg       config.Config
	db        *sql.DB
	sm        *scs.SessionManager
	providers []Provider

	// NotInvited renders the page shown to an uninvited address.
	NotInvited http.HandlerFunc
}

// New configures goth with the providers that have credentials. goth keeps
// its providers and cookie store in package globals; this is the only place
// that touches them.
func New(cfg config.Config, db *sql.DB, sm *scs.SessionManager) *Auth {
	a := &Auth{cfg: cfg, db: db, sm: sm, NotInvited: func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Нет приглашения", http.StatusForbidden)
	}}
	var ps []goth.Provider
	if cfg.GoogleID != "" && cfg.GoogleSecret != "" {
		ps = append(ps, google.New(cfg.GoogleID, cfg.GoogleSecret, cfg.BaseURL+"/auth/google/callback", "email", "profile"))
		a.providers = append(a.providers, Provider{"google", "Google"})
	}
	if cfg.GitHubID != "" && cfg.GitHubSecret != "" {
		ps = append(ps, github.New(cfg.GitHubID, cfg.GitHubSecret, cfg.BaseURL+"/auth/github/callback", "user:email"))
		a.providers = append(a.providers, Provider{"github", "GitHub"})
	}
	goth.UseProviders(ps...)
	if cfg.DevLoginEnabled() {
		a.providers = append(a.providers, Provider{"dev", "локально (dev)"})
	}

	// The cookie store only carries the OAuth state across the redirect.
	store := sessions.NewCookieStore([]byte(cfg.SessionKey))
	store.Options = &sessions.Options{
		Path: "/", MaxAge: 600, HttpOnly: true, Secure: cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	}
	gothic.Store = store
	gothic.GetProviderName = func(r *http.Request) (string, error) {
		if p := r.PathValue("provider"); p != "" {
			return p, nil
		}
		return "", errors.New("auth: no provider in path")
	}
	return a
}

// Providers lists the sign-in options that are configured.
func (a *Auth) Providers() []Provider { return a.providers }

// Routes registers the OAuth and logout endpoints.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/{provider}", a.begin)
	mux.HandleFunc("GET /auth/{provider}/callback", a.callback)
	mux.HandleFunc("POST /logout", a.logout)
	if a.cfg.DevLoginEnabled() {
		mux.HandleFunc("POST /auth/dev", a.devLogin)
	}
}

// devLogin signs in as DEV_LOGIN without OAuth; registered only on localhost.
func (a *Auth) devLogin(w http.ResponseWriter, r *http.Request) {
	u, err := a.completeLogin(r.Context(), "dev", goth.User{Email: a.cfg.DevLogin, Name: "dev"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.signIn(w, r, u)
}

func (a *Auth) signIn(w http.ResponseWriter, r *http.Request, u User) {
	// A fresh token on privilege change prevents session fixation.
	if err := a.sm.RenewToken(r.Context()); err != nil {
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	a.sm.Put(r.Context(), sessionUserKey, u.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// CurrentUser returns the signed-in user, if any.
func (a *Auth) CurrentUser(r *http.Request) (User, bool) {
	id := a.sm.GetInt64(r.Context(), sessionUserKey)
	if id == 0 {
		return User{}, false
	}
	var u User
	err := a.db.QueryRowContext(r.Context(), `SELECT id, email, name FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.Name)
	if err != nil {
		return User{}, false
	}
	return u, true
}

func (a *Auth) configured(name string) bool {
	for _, p := range a.providers {
		if p.Name == name && name != "dev" {
			return true
		}
	}
	return false
}

func (a *Auth) begin(w http.ResponseWriter, r *http.Request) {
	if !a.configured(r.PathValue("provider")) {
		http.NotFound(w, r)
		return
	}
	gothic.BeginAuthHandler(w, r)
}

func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !a.configured(provider) {
		http.NotFound(w, r)
		return
	}
	gu, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		slog.Warn("oauth callback failed", "provider", provider, "err", err)
		http.Error(w, "Не удалось войти, попробуйте ещё раз", http.StatusBadRequest)
		return
	}
	u, err := a.completeLogin(r.Context(), provider, gu)
	if errors.Is(err, ErrNotInvited) {
		a.NotInvited(w, r)
		return
	}
	if err != nil {
		slog.Error("login failed", "err", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	a.signIn(w, r, u)
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.sm.Destroy(r.Context()); err != nil {
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// completeLogin checks the invite list and creates or refreshes the user.
func (a *Auth) completeLogin(ctx context.Context, provider string, gu goth.User) (User, error) {
	email := strings.ToLower(strings.TrimSpace(gu.Email))
	if !a.cfg.EmailAllowed(email) {
		return User{}, ErrNotInvited
	}
	name := strings.TrimSpace(gu.Name)
	if name == "" {
		name = gu.NickName
	}
	u := User{Email: email, Name: name}
	err := a.db.QueryRowContext(ctx, `
		INSERT INTO users (email, name, provider) VALUES (?, ?, ?)
		ON CONFLICT (email) DO UPDATE SET name = excluded.name
		RETURNING id`, email, name, provider).Scan(&u.ID)
	if err != nil {
		return User{}, fmt.Errorf("auth: upsert user: %w", err)
	}
	return u, nil
}
