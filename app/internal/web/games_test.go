package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func (f fixture) post(t *testing.T, path string, user int64, form url.Values, htmx bool) (int, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != 0 {
		r.Header.Set(testUserHeader, "1")
	}
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return rec.Code, rec.Body.String()
}

func TestGames_RequireLogin(t *testing.T) {
	f := newFixture(t)
	for _, p := range []string{"/games", "/games/write", "/games/listen", "/games/speak"} {
		if code, _, hdr := f.do(t, req{path: p}); code != http.StatusSeeOther || hdr.Get("Location") != "/login" {
			t.Errorf("anonymous GET %s = %d → %q", p, code, hdr.Get("Location"))
		}
	}
	if code, _ := f.post(t, "/games/check", 0, url.Values{"key": {"suffer"}, "mode": {"listen"}}, false); code != http.StatusSeeOther {
		t.Errorf("anonymous check = %d", code)
	}
}

var keyField = regexp.MustCompile(`name="key" value="([^"]+)"`)

func TestWriteGame_TenUnlearntWords(t *testing.T) {
	f := newFixture(t)
	f.do(t, req{method: "POST", path: "/words/suffer/hide", user: 1, htmx: true})
	for i := 0; i < 5; i++ {
		_, body, _ := f.do(t, req{path: "/games/write", user: 1})
		keys := keyField.FindAllStringSubmatch(body, -1)
		if len(keys) != gameSize {
			t.Fatalf("write game has %d words, want %d", len(keys), gameSize)
		}
		seen := map[string]bool{}
		for _, k := range keys {
			if k[1] == "suffer" {
				t.Fatal("a hidden (learnt) word must not be asked")
			}
			if seen[k[1]] {
				t.Fatalf("word %q asked twice", k[1])
			}
			seen[k[1]] = true
		}
	}
}

func TestWriteGame_Grading(t *testing.T) {
	f := newFixture(t)
	form := url.Values{
		"key":    {"suffer", "breath", "equanimity"},
		"answer": {"Страдать", "", "злость"},
	}
	code, body := f.post(t, "/games/write", 1, form, true)
	if code != 200 {
		t.Fatalf("POST /games/write = %d", code)
	}
	mustContain(t, body, "Верно 1 из 3", `hx-post="/games/important/breath"`, `hx-post="/games/important/equanimity"`,
		"ваш ответ: злость", "нет ответа")
	mustNotContain(t, body, `/games/important/suffer`, "<html")

	if code, _ := f.post(t, "/games/write", 1, url.Values{"key": {"a", "b"}, "answer": {"x"}}, true); code != 400 {
		t.Errorf("mismatched form = %d, want 400", code)
	}
}

func TestSendToImportant(t *testing.T) {
	f := newFixture(t)
	code, body := f.post(t, "/games/important/breath", 1, nil, true)
	if code != 200 || !strings.Contains(body, "breath") || !strings.Contains(body, "Важных") {
		t.Fatalf("send = %d %q", code, body)
	}
	_, imp, _ := f.do(t, req{path: "/important", user: 1})
	mustContain(t, imp, `id="w-breath"`)
	if code, _ := f.post(t, "/games/important/nope", 1, nil, true); code != 404 {
		t.Errorf("unknown word = %d, want 404", code)
	}
}

func TestSpeechCheck(t *testing.T) {
	f := newFixture(t)
	check := func(form url.Values) (int, checkResult) {
		code, body := f.post(t, "/games/check", 1, form, false)
		var res checkResult
		if code == 200 {
			if err := json.Unmarshal([]byte(body), &res); err != nil {
				t.Fatalf("bad JSON %q: %v", body, err)
			}
		}
		return code, res
	}
	_, res := check(url.Values{"key": {"equanimity"}, "mode": {"listen"}, "answer": {"злость", "невозмутимость"}})
	if !res.OK || res.Lemma != "equanimity" {
		t.Errorf("listen with a matching alternative = %+v", res)
	}
	_, res = check(url.Values{"key": {"equanimity"}, "mode": {"speak"}, "answer": {"equanimity"}})
	if !res.OK || res.Lemma != "equanimity" || !strings.Contains(res.Expected, "невозмутимость") {
		t.Errorf("speak = %+v", res)
	}
	_, res = check(url.Values{"key": {"equanimity"}, "mode": {"speak"}})
	if res.OK {
		t.Error("no answer must be wrong")
	}
	if code, _ := check(url.Values{"key": {"nope"}, "mode": {"speak"}}); code != 400 {
		t.Errorf("unknown key = %d, want 400", code)
	}
	if code, _ := check(url.Values{"key": {"equanimity"}, "mode": {"other"}}); code != 400 {
		t.Errorf("bad mode = %d, want 400", code)
	}
}

func TestGameForms_Bounded(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("я", 300)
	_, body := f.post(t, "/games/write", 1, url.Values{"key": {"breath"}, "answer": {long}}, true)
	if strings.Contains(body, strings.Repeat("я", maxAnswerRune+1)) || !strings.Contains(body, strings.Repeat("я", maxAnswerRune)) {
		t.Error("an answer must be cut to maxAnswerRune runes")
	}
	huge := url.Values{"key": {"breath"}, "mode": {"listen"}, "answer": {strings.Repeat("a", maxFormBytes)}}
	if code, _ := f.post(t, "/games/check", 1, huge, false); code != http.StatusBadRequest {
		t.Errorf("oversized form = %d, want 400", code)
	}
}

func TestSpeechGame_Items(t *testing.T) {
	f := newFixture(t)
	_, body, _ := f.do(t, req{path: "/games/speak", user: 1})
	mustContain(t, body, `data-mode="speak"`, `data-answer-lang="en-US"`, "/static/games.js", "&#34;lang&#34;:&#34;ru-RU&#34;")
	_, body, _ = f.do(t, req{path: "/games/listen", user: 1})
	mustContain(t, body, `data-answer-lang="ru-RU"`, "https://audio.example/")
}
