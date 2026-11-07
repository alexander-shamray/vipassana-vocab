package main

import (
	"context"
	"io"
	"net"
	"net/http"
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
	go func() { done <- serve(ctx, getenv, ln) }()
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
	for _, args := range [][]string{nil, {"bogus"}, {"import"}} {
		if err := dispatch(context.Background(), args, getenv); err == nil {
			t.Errorf("dispatch(%q) succeeded, want an error", args)
		}
	}
}
