package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/markbates/goth"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/config"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/db"
)

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
	u1, err := a.completeLogin(ctx, "google", goth.User{Email: "me@x.com", Name: "Me"})
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
	if _, err := a.completeLogin(context.Background(), "google", goth.User{Email: "x@y.z"}); err != nil {
		t.Fatal(err)
	}
	if users(t, a) != 1 {
		t.Error("ALLOW_ANY should create the user")
	}
}

func TestProviders_OnlyConfigured(t *testing.T) {
	a := newAuth(t, map[string]string{"GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET": "s"})
	if ps := a.Providers(); len(ps) != 1 || ps[0].Name != "github" {
		t.Errorf("providers = %+v, want only github", ps)
	}
}
