// Package config reads the server configuration from environment variables.
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Config holds everything the server needs at startup.
type Config struct {
	Port         string
	DatabasePath string
	BaseURL      string
	AudioBaseURL string

	// AllowedEmails is the invite list, already lower-cased and trimmed.
	AllowedEmails []string
	// AllowAny lifts the invite list. An empty list alone never means "everyone".
	AllowAny bool

	GoogleID     string
	GoogleSecret string
	GitHubID     string
	GitHubSecret string
	// SessionKey signs the short-lived OAuth state cookie.
	SessionKey string

	// DevLogin, when set on a localhost BaseURL, adds a one-click sign-in as
	// this address for local debugging without OAuth credentials.
	DevLogin string
}

// Load builds a Config from getenv (os.Getenv in production, a map in tests).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Port:         withDefault(getenv("PORT"), "8080"),
		DatabasePath: withDefault(getenv("DATABASE_PATH"), "data/app.sqlite"),
		BaseURL:      strings.TrimRight(withDefault(getenv("BASE_URL"), "http://localhost:8080"), "/"),
		AudioBaseURL: strings.TrimRight(getenv("AUDIO_BASE_URL"), "/"),
		GoogleID:     getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret: getenv("GOOGLE_CLIENT_SECRET"),
		GitHubID:     getenv("GITHUB_CLIENT_ID"),
		GitHubSecret: getenv("GITHUB_CLIENT_SECRET"),
		SessionKey:   getenv("SESSION_KEY"),
		DevLogin:     strings.ToLower(strings.TrimSpace(getenv("DEV_LOGIN"))),
	}
	if (c.GoogleID != "" || c.GitHubID != "") && len(c.SessionKey) < 32 {
		return Config{}, fmt.Errorf("config: SESSION_KEY must be at least 32 bytes when OAuth is configured")
	}
	if c.DevLogin != "" && !c.local() {
		return Config{}, fmt.Errorf("config: DEV_LOGIN is only allowed with a localhost BASE_URL, got %q", c.BaseURL)
	}
	for _, e := range strings.Split(getenv("ALLOWED_EMAILS"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			c.AllowedEmails = append(c.AllowedEmails, e)
		}
	}
	switch v := strings.ToLower(strings.TrimSpace(getenv("ALLOW_ANY"))); v {
	case "", "false", "0", "no":
	case "true", "1", "yes":
		c.AllowAny = true
	default:
		return Config{}, fmt.Errorf("config: ALLOW_ANY=%q is not a boolean", v)
	}
	return c, nil
}

// EmailAllowed reports whether email may sign up.
func (c Config) EmailAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	if c.AllowAny || email == c.DevLogin {
		return true
	}
	for _, a := range c.AllowedEmails {
		if a == email {
			return true
		}
	}
	return false
}

// DevLoginEnabled reports whether the local debugging sign-in is on.
func (c Config) DevLoginEnabled() bool { return c.DevLogin != "" && c.local() }

func (c Config) local() bool {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// SecureCookies reports whether cookies must carry the Secure flag.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func withDefault(v, def string) string {
	if v = strings.TrimSpace(v); v == "" {
		return def
	}
	return v
}
