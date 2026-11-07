package web

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/auth"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/db"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
	"github.com/alexander-shamray/vipassana-vocab/app/internal/search"
)

// testUserHeader stands in for a session in tests: its value is a user id.
const testUserHeader = "X-Test-User"

type fixture struct {
	h  http.Handler
	db *sql.DB
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "w.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	importFile(t, d, "words.sample.jsonl", dict.ImportWords)
	importFile(t, d, "examples.sample.jsonl", dict.ImportExamples)
	if _, err := d.Exec(`INSERT INTO users (id, email, provider) VALUES (1, 'a@x.com', 't'), (2, 'b@x.com', 't')`); err != nil {
		t.Fatal(err)
	}
	entries, err := dict.LoadAll(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Deps{
		DB:      d,
		Entries: entries,
		Index:   search.New(entries),
		CurrentUser: func(r *http.Request) (auth.User, bool) {
			id, err := strconv.ParseInt(r.Header.Get(testUserHeader), 10, 64)
			if err != nil {
				return auth.User{}, false
			}
			return auth.User{ID: id, Email: "u@x.com"}, true
		},
		Providers:    []auth.Provider{{Name: "dev", Title: "локально (dev)"}},
		AudioBaseURL: "https://audio.example",
	})
	return fixture{h, d}
}

func importFile(t *testing.T, d *sql.DB, name string, f func(context.Context, *sql.DB, io.Reader) (dict.Stats, error)) {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := f(context.Background(), d, file); err != nil {
		t.Fatal(err)
	}
}

type req struct {
	method, path string
	user         int64
	htmx         bool
}

func (f fixture) do(t *testing.T, r req) (int, string, http.Header) {
	t.Helper()
	if r.method == "" {
		r.method = http.MethodGet
	}
	hr := httptest.NewRequest(r.method, r.path, nil)
	if r.user != 0 {
		hr.Header.Set(testUserHeader, strconv.FormatInt(r.user, 10))
	}
	if r.htmx {
		hr.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, hr)
	return rec.Code, rec.Body.String(), rec.Header()
}

func mustContain(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(body, s) {
			t.Errorf("body lacks %q", s)
		}
	}
}

func mustNotContain(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(body, s) {
			t.Errorf("body unexpectedly contains %q", s)
		}
	}
}

func TestList_AnonymousAlphabeticalNoActions(t *testing.T) {
	f := newFixture(t)
	code, body, _ := f.do(t, req{path: "/"})
	if code != 200 {
		t.Fatalf("GET / = %d", code)
	}
	a, b := strings.Index(body, `id="w-abandon"`), strings.Index(body, `id="w-vipassana"`)
	if a < 0 || b < 0 || a > b {
		t.Error("words are not in alphabetical order")
	}
	mustContain(t, body, `href="/login"`, "черновик перевода", "suffered")
	mustNotContain(t, body, "/hide", "/words/anicca/important")
}

func TestMarks_HideImportantRestore(t *testing.T) {
	f := newFixture(t)
	if code, body, _ := f.do(t, req{method: "POST", path: "/words/suffer/hide", user: 1, htmx: true}); code != 200 || body != "" {
		t.Fatalf("hide = %d %q, want 200 and an empty body", code, body)
	}
	if code, _, _ := f.do(t, req{method: "POST", path: "/words/anicca/important", user: 1, htmx: true}); code != 200 {
		t.Fatalf("important = %d", code)
	}

	_, main, _ := f.do(t, req{path: "/", user: 1})
	mustNotContain(t, main, `id="w-suffer"`, `id="w-anicca"`)
	mustContain(t, main, `id="w-sufficient"`)

	_, hidden, _ := f.do(t, req{path: "/hidden", user: 1})
	mustContain(t, hidden, `id="w-suffer"`, "/words/suffer/restore")
	mustNotContain(t, hidden, `id="w-anicca"`, `id="w-sufficient"`)

	_, imp, _ := f.do(t, req{path: "/important", user: 1})
	mustContain(t, imp, `id="w-anicca"`)
	mustNotContain(t, imp, `id="w-suffer"`)

	_, found, _ := f.do(t, req{path: "/important?q=anica", user: 1, htmx: true})
	mustContain(t, found, `id="w-anicca"`)
	mustNotContain(t, found, "<html")

	// Other users are unaffected.
	_, other, _ := f.do(t, req{path: "/", user: 2})
	mustContain(t, other, `id="w-suffer"`, `id="w-anicca"`)

	// Hiding an important word moves it to the hidden list.
	f.do(t, req{method: "POST", path: "/words/anicca/hide", user: 1, htmx: true})
	_, imp, _ = f.do(t, req{path: "/important", user: 1})
	mustNotContain(t, imp, `id="w-anicca"`)
	_, hidden, _ = f.do(t, req{path: "/hidden", user: 1})
	mustContain(t, hidden, `id="w-anicca"`)

	f.do(t, req{method: "POST", path: "/words/suffer/restore", user: 1, htmx: true})
	_, main, _ = f.do(t, req{path: "/", user: 1})
	mustContain(t, main, `id="w-suffer"`)
}

func TestMarks_RepeatedHideIsIdempotent(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 2; i++ {
		if code, _, _ := f.do(t, req{method: "POST", path: "/words/suffer/hide", user: 1, htmx: true}); code != 200 {
			t.Fatalf("hide #%d = %d", i+1, code)
		}
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM word_marks WHERE user_id = 1`).Scan(&n)
	if n != 1 {
		t.Errorf("marks = %d, want 1", n)
	}
}

func TestMarks_RequireLoginAndKnownKey(t *testing.T) {
	f := newFixture(t)
	code, _, hdr := f.do(t, req{method: "POST", path: "/words/suffer/hide", htmx: true})
	if code != http.StatusUnauthorized || hdr.Get("HX-Redirect") != "/login" {
		t.Errorf("anonymous HTMX hide = %d, HX-Redirect %q", code, hdr.Get("HX-Redirect"))
	}
	if code, _, hdr := f.do(t, req{method: "POST", path: "/words/suffer/hide"}); code != http.StatusSeeOther || hdr.Get("Location") != "/login" {
		t.Errorf("anonymous form hide = %d → %q", code, hdr.Get("Location"))
	}
	for _, p := range []string{"/words/no-such-word/hide", "/words/%20/important", "/words/x/restore"} {
		if code, _, _ := f.do(t, req{method: "POST", path: p, user: 1, htmx: true}); code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", p, code)
		}
	}
}

func TestList_SearchAndFragments(t *testing.T) {
	f := newFixture(t)
	_, body, _ := f.do(t, req{path: "/?q=equanimty", htmx: true})
	mustContain(t, body, `id="w-equanimity"`)
	mustNotContain(t, body, "<html")

	_, body, _ = f.do(t, req{path: "/?q=" + strings.Repeat("x", 500)})
	mustContain(t, body, "Ничего нет")

	_, body, _ = f.do(t, req{path: "/?q=%20%20"})
	mustContain(t, body, `id="w-abandon"`)

	_, body, _ = f.do(t, req{path: "/?from=s"})
	mustNotContain(t, body, `id="w-abandon"`)
	mustContain(t, body, `id="w-samadhi"`)

	_, body, _ = f.do(t, req{path: "/hidden?q=suffer", user: 1, htmx: true})
	mustNotContain(t, body, `id="w-suffer"`)
}

func TestWord_FormsExamplesAndEscaping(t *testing.T) {
	f := newFixture(t)
	code, body, _ := f.do(t, req{path: "/words/suffer"})
	if code != 200 {
		t.Fatalf("GET /words/suffer = %d", code)
	}
	mustContain(t, body, "Формы в беседах", "<li>suffers</li>", "<li>suffering</li>", "Войдите")
	mustNotContain(t, body, "You suffer")

	_, body, _ = f.do(t, req{path: "/words/suffer", user: 1, htmx: true})
	mustNotContain(t, body, "<html")
	mustContain(t, body, "https://youtu.be/vid01?t=612", "Беседа 1, 10:12", "<mark>suffer</mark>")
	if n := strings.Count(body, "<blockquote>"); n != 3 {
		t.Errorf("quotes = %d, want 3", n)
	}

	_, body, _ = f.do(t, req{path: "/words/sensation", user: 1})
	mustContain(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;", "&amp; co")
	mustNotContain(t, body, "<script>alert(1)")

	_, body, _ = f.do(t, req{path: "/words/abandon", user: 1})
	mustContain(t, body, "Примеров нет")

	if code, _, _ := f.do(t, req{path: "/words/nope"}); code != 404 {
		t.Errorf("unknown word = %d, want 404", code)
	}
}

func TestPaging(t *testing.T) {
	f := newFixture(t)
	// The fixture has 30 words; shrink nothing, just check the sentinel logic.
	_, body, _ := f.do(t, req{path: "/"})
	mustNotContain(t, body, `class="more"`)
	_, body, _ = f.do(t, req{path: "/?after=suffer", htmx: true})
	mustContain(t, body, `id="w-sufficient"`)
	mustNotContain(t, body, `id="w-suffer"`, `id="w-abandon"`)
}

func TestHighlight(t *testing.T) {
	parts := Highlight("Suffering? You suffer, unsuffering.", []string{"suffer", "Suffering"})
	var marked []string
	for _, p := range parts {
		if p.Mark {
			marked = append(marked, p.Text)
		}
	}
	if strings.Join(marked, "|") != "Suffering|suffer" {
		t.Errorf("marked = %q", marked)
	}
	parts = Highlight("Observe mind and matter now", []string{"mind and matter"})
	if len(parts) != 3 || !parts[1].Mark || parts[1].Text != "mind and matter" {
		t.Errorf("phrase parts = %+v", parts)
	}
}

func TestStaticAndHealth(t *testing.T) {
	f := newFixture(t)
	for _, p := range []string{"/static/app.css", "/static/app.js", "/static/htmx.min.js", "/healthz", "/login"} {
		if code, _, _ := f.do(t, req{path: p}); code != 200 {
			t.Errorf("GET %s = %d", p, code)
		}
	}
}

func TestCrossSiteFormPostRejected(t *testing.T) {
	f := newFixture(t)
	r := httptest.NewRequest("POST", "/words/suffer/hide", nil)
	r.Header.Set(testUserHeader, "1")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site POST = %d, want 403", rec.Code)
	}
}
