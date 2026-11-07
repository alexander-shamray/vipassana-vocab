package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/markbates/goth"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/config"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/db"
)

var verified = map[string]any{"verified_email": true}

func newAuth(t *testing.T, env map[string]string) *Auth {
	t.Helper()
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "a.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return New(cfg, d, scs.New())
}

func users(t *testing.T, a *Auth) int {
	t.Helper()
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCompleteLogin_InvitedCreatesOnce(t *testing.T) {
	a := newAuth(t, map[string]string{"ALLOWED_EMAILS": "me@x.com"})
	ctx := context.Background()
	u1, err := a.completeLogin(ctx, "google", goth.User{Email: "me@x.com", Name: "Me", RawData: verified})
	if err != nil {
		t.Fatal(err)
	}
	u2, err := a.completeLogin(ctx, "github", goth.User{Email: "ME@X.com", NickName: "me"})
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID != u2.ID || users(t, a) != 1 {
		t.Errorf("same address in another case must map to one user: %d vs %d", u1.ID, u2.ID)
	}
}

func TestCompleteLogin_NotInvited(t *testing.T) {
	a := newAuth(t, map[string]string{"ALLOWED_EMAILS": "me@x.com"})
	_, err := a.completeLogin(context.Background(), "google", goth.User{Email: "stranger@y.com"})
	if !errors.Is(err, ErrNotInvited) {
		t.Fatalf("err = %v, want ErrNotInvited", err)
	}
	if users(t, a) != 0 {
		t.Error("uninvited login must not create a user")
	}
}

func TestCompleteLogin_EmptyEmail(t *testing.T) {
	a := newAuth(t, map[string]string{"ALLOW_ANY": "true"})
	if _, err := a.completeLogin(context.Background(), "github", goth.User{}); !errors.Is(err, ErrNotInvited) {
		t.Fatalf("err = %v, want ErrNotInvited for an empty address", err)
	}
}

func TestCompleteLogin_AllowAny(t *testing.T) {
	a := newAuth(t, map[string]string{"ALLOW_ANY": "true"})
	if _, err := a.completeLogin(context.Background(), "google", goth.User{Email: "x@y.z", RawData: verified}); err != nil {
		t.Fatal(err)
	}
	if users(t, a) != 1 {
		t.Error("ALLOW_ANY should create the user")
	}
}

func TestProviders_OnlyConfigured(t *testing.T) {
	a := newAuth(t, map[string]string{"GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET": "s", "SESSION_KEY": "0123456789abcdef0123456789abcdef"})
	if ps := a.Providers(); len(ps) != 1 || ps[0].Name != "github" {
		t.Errorf("providers = %+v, want only github", ps)
	}
}

func TestCompleteLogin_GoogleUnverifiedEmailRejected(t *testing.T) {
	a := newAuth(t, map[string]string{"ALLOWED_EMAILS": "me@x.com"})
	for _, raw := range []map[string]any{nil, {"verified_email": false}, {"verified_email": "true"}} {
		_, err := a.completeLogin(context.Background(), "google", goth.User{Email: "me@x.com", RawData: raw})
		if !errors.Is(err, ErrNotInvited) {
			t.Errorf("RawData %v: err = %v, want ErrNotInvited", raw, err)
		}
	}
	if users(t, a) != 0 {
		t.Error("an unverified Google address must not create a user")
	}
	if _, err := a.completeLogin(context.Background(), "unknown", goth.User{Email: "me@x.com"}); err == nil {
		t.Error("an unknown provider must not be trusted")
	}
}

func TestCurrentUser_RevokedWhenRemovedFromInviteList(t *testing.T) {
	a := newAuth(t, map[string]string{"ALLOWED_EMAILS": "me@x.com"})
	u, err := a.completeLogin(context.Background(), "github", goth.User{Email: "me@x.com"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(a *Auth) (got bool) {
		h := a.sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a.sm.Put(r.Context(), sessionUserKey, u.ID)
			_, got = a.CurrentUser(r)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		return got
	}
	if !check(a) {
		t.Fatal("invited user should be signed in")
	}
	a.cfg.AllowedEmails = nil // owner removed the address
	if check(a) {
		t.Error("a session must stop working once the address is off the invite list")
	}
}
