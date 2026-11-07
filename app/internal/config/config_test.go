package config

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoad_AllowList(t *testing.T) {
	c, err := Load(env(map[string]string{"ALLOWED_EMAILS": " A@x.com , b@y.org "}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.EmailAllowed("a@X.com") {
		t.Error("a@X.com should be allowed")
	}
	if !c.EmailAllowed(" b@y.org") {
		t.Error("b@y.org should be allowed")
	}
	if c.EmailAllowed("c@z.io") {
		t.Error("c@z.io should not be allowed")
	}
}

func TestLoad_EmptyListAllowsNobody(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.EmailAllowed("a@x.com") {
		t.Error("empty list must allow nobody")
	}
}

func TestLoad_AllowAny(t *testing.T) {
	c, err := Load(env(map[string]string{"ALLOW_ANY": "true"}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.EmailAllowed("anyone@x.com") {
		t.Error("ALLOW_ANY should allow anyone")
	}
	if c.EmailAllowed("") {
		t.Error("empty email must never be allowed")
	}
	if _, err := Load(env(map[string]string{"ALLOW_ANY": "maybe"})); err == nil {
		t.Error("ALLOW_ANY=maybe should fail")
	}
}

func TestLoad_Defaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || c.DatabasePath != "data/app.sqlite" {
		t.Errorf("defaults: got port %q, db %q", c.Port, c.DatabasePath)
	}
	if c.SecureCookies() {
		t.Error("localhost default must not force Secure cookies")
	}
}

func TestLoad_DevLoginOnlyOnLocalhost(t *testing.T) {
	c, err := Load(env(map[string]string{"DEV_LOGIN": "Dev@Local"}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.DevLoginEnabled() || !c.EmailAllowed("dev@local") {
		t.Error("DEV_LOGIN on the localhost default should be enabled and allowed")
	}
	_, err = Load(env(map[string]string{"DEV_LOGIN": "dev@local", "BASE_URL": "https://vocab.fly.dev"}))
	if err == nil {
		t.Error("DEV_LOGIN with a public BASE_URL must fail")
	}
}

func TestLoad_OAuthNeedsLongSessionKey(t *testing.T) {
	if _, err := Load(env(map[string]string{"GITHUB_CLIENT_ID": "id", "SESSION_KEY": "short"})); err == nil {
		t.Error("a short SESSION_KEY with OAuth configured must fail")
	}
	key := "0123456789abcdef0123456789abcdef"
	if _, err := Load(env(map[string]string{"GITHUB_CLIENT_ID": "id", "SESSION_KEY": key})); err != nil {
		t.Errorf("32-byte key: %v", err)
	}
}
