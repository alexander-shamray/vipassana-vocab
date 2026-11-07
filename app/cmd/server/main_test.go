package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServe_Smoke(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "smoke.sqlite")
	env := map[string]string{"DATABASE_PATH": dbPath, "DEV_LOGIN": "dev@localhost"}
	getenv := func(k string) string { return env[k] }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := dispatch(ctx, []string{"import", "--words", "../../testdata/words.sample.jsonl",
		"--examples", "../../testdata/examples.sample.jsonl"}, getenv); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, getenv, ln, "") }()
	base := "http://" + ln.Addr().String()

	client := &http.Client{Timeout: 5 * time.Second}
	for _, tc := range []struct{ path, want string }{
		{"/healthz", "ok"},
		{"/", "equanimity"},
		{"/static/app.css", "--accent"},
		{"/login", "/auth/dev"},
	} {
		resp, err := client.Get(base + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), tc.want) {
			t.Errorf("GET %s = %d, body lacks %q", tc.path, resp.StatusCode, tc.want)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v after shutdown", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("serve did not shut down")
	}
}

func TestDispatch_Errors(t *testing.T) {
	getenv := func(string) string { return "" }
	for _, args := range [][]string{nil, {"bogus"}, {"import"}, {"serve"}} {
		if err := dispatch(context.Background(), args, getenv); err == nil {
			t.Errorf("dispatch(%q) succeeded, want an error", args)
		}
	}
}

func TestServe_RefusesDevLogin(t *testing.T) {
	env := map[string]string{"BASE_URL": "http://localhost:8080", "DEV_LOGIN": "dev@localhost"}
	err := dispatch(context.Background(), []string{"serve"}, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "DEV_LOGIN") {
		t.Errorf("serve with DEV_LOGIN: err = %v", err)
	}
}

func TestWithLocalAudio(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "suffer-abc.mp3"), []byte("ID3"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "site") })
	h := withLocalAudio(site, dir)
	for path, want := range map[string]string{"/audio/suffer-abc.mp3": "ID3", "/": "site", "/words/x": "site"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 || rec.Body.String() != want {
			t.Errorf("GET %s = %d %q, want %q", path, rec.Code, rec.Body.String(), want)
		}
	}
	for _, path := range []string{"/audio/missing.mp3", "/audio/", "/audio/../main.go"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == 200 {
			t.Errorf("GET %s = 200, want an error (no listing, no escape)", path)
		}
	}
}
