package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixMarks(t *testing.T) {
	cases := map[string]string{
		"+Это +это невозм+ожно.":                "+Это невозм+ожно.",
		"вступ+ает в конт+акт с с нечистот+ой.": "вступ+ает в конт+акт с нечистот+ой.",
		"н+аблюдать, н+аблюдать":                "н+аблюдать, н+аблюдать", // a comma: said twice on purpose
		"Вы вышли": "Вы вышли",
		"Т+а же законом​+ерность": "Та же законом+ерность",
		"вс+ё":     "всё",
		"м+ол+око": "м+олоко",
		"сл+н":     "слн",
		"а а а":    "а а",
	}
	for in, want := range cases {
		if got := fixMarks(in); got != want {
			t.Errorf("fixMarks(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarkWordPriority(t *testing.T) {
	fixes := map[string]int{"после": 1}
	counts := map[string]int{}
	cases := []struct{ claude, ruaccent, want string }{
		{"п+осле", "посл+е", "п+осле"},                // the reviewed list beats both
		{"реальн+ость", "ре+альность", "ре+альность"}, // RuAccent beats Claude
		{"инт+ересно", "интересно", "инт+ересно"},     // Claude only where RuAccent has no mark
		{"день", "д+ень", "день"},                     // one vowel: no mark
		{"Само", "+ам", "Само"},                       // RuAccent's mark at a consonant: none
	}
	for _, c := range cases {
		if got := markWord(c.claude, c.ruaccent, fixes, counts); got != c.want {
			t.Errorf("markWord(%q, %q) = %q, want %q", c.claude, c.ruaccent, got, c.want)
		}
	}
}

func TestMarkProblems(t *testing.T) {
	cases := map[string]bool{
		"Наблюд+айте дых+ание.": false,
		"Наблюдайте":            true,  // no mark
		"н++ет":                 true,  // two marks, one not before a vowel
		"вс+ё":                  true,  // a mark in a ё word
		"день 5":                true,  // a digit
		"день":                  false, // one vowel
	}
	for in, bad := range cases {
		if got := markProblems(in) != nil; got != bad {
			t.Errorf("markProblems(%q) = %v, want bad=%v", in, markProblems(in), bad)
		}
	}
}

func TestCyrillicShareCatchesTheEnglishDub(t *testing.T) {
	if s := cyrillicShare("The fourth day has ended. Субтитры"); s >= 0.9 {
		t.Errorf("English text passed as Russian: %v", s)
	}
	if s := cyrillicShare("Закончился четвёртый день. 10 дней, Vipassana"); s < 0.7 {
		t.Errorf("Russian text refused: %v", s)
	}
}

func word(text string, start, end float64) timedWord {
	return timedWord{Word: text, Start: start, End: &end}
}

func TestSplitPhrases(t *testing.T) {
	words := []timedWord{
		word(" Закончился", 0, 1), word(" четвёртый", 1, 2), word(" день.", 2, 3),
		word(" Да.", 3, 3.2), word(" Теперь", 5, 6), word(" ещё", 6, 7),
	}
	got := splitPhrases(words)
	if len(got) != 2 || got[0].Text != "Закончился четвёртый день." || got[1].Text != "Да. Теперь ещё" {
		t.Fatalf("got %+v", got)
	}
	if got[1].ID != 1 || got[1].Start != 3 || got[1].End != 7 {
		t.Errorf("ids or times: %+v", got[1])
	}
}

func TestBestWindowPrefersDenseSpeech(t *testing.T) {
	sparse := segment{Start: 400, End: 420, Words: []timedWord{word(" a", 400, 401), word(" b", 419, 420)}}
	dense := segment{Start: 500, End: 520, Words: []timedWord{word(" c", 500, 510), word(" d", 510, 520)}}
	early := segment{Start: 100, End: 120, Words: []timedWord{word(" e", 100, 110), word(" f", 110, 120)}}
	doubtful := segment{Start: 600, End: 620, NoSpeechProb: .5, Words: []timedWord{word(" g", 600, 610), word(" h", 610, 620)}}
	w, ok := bestWindow([]segment{early, sparse, dense, doubtful}, 25)
	if !ok || w.text != "c d" || w.start != 500 || w.end != 520 {
		t.Errorf("got %+v, %v", w, ok)
	}
}

func TestLayOutWaitsForATargetAndSpeedsUpOnlyWhenNeeded(t *testing.T) {
	second := make([]float32, sampleRate)
	for i := range second {
		second[i] = 0.5
	}
	buf, stats, err := layOut([][]float32{second, nil, second}, []float64{1, 2, 5}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Phrases != 2 || stats.TempoMean != 1 || stats.LateMax != 0 {
		t.Errorf("stats %+v", stats)
	}
	if buf[sampleRate/2] != 0 || buf[sampleRate*5+10] == 0 || buf[sampleRate*3] != 0 {
		t.Error("phrases are not at their targets")
	}
}

func TestServeOnlyThePlayers(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("lesson4", 0o755)
	os.WriteFile(filepath.Join("lesson4", "index.html"), []byte("page"), 0o644)
	os.WriteFile(filepath.Join("lesson4", "video.mp4"), []byte("0123456789"), 0o644)
	os.WriteFile(filepath.Join("lesson4", "lesson4.mkv"), []byte("x"), 0o644)
	os.WriteFile("full-ru.json", []byte("{}"), 0o644)
	s := httptest.NewServer(http.HandlerFunc(servePlayers))
	defer s.Close()

	get := func(path, rangeHeader string) (int, string) {
		req, _ := http.NewRequest("GET", s.URL+path, nil)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var b strings.Builder
		buf := make([]byte, 256)
		n, _ := res.Body.Read(buf)
		b.Write(buf[:n])
		return res.StatusCode, b.String()
	}
	if code, body := get("/", ""); code != 200 || !strings.Contains(body, "/lesson4/") {
		t.Errorf("index: %d %q", code, body)
	}
	if code, body := get("/lesson4/", ""); code != 200 || body != "page" {
		t.Errorf("player: %d %q", code, body)
	}
	if code, body := get("/lesson4/video.mp4", "bytes=2-4"); code != 206 || body != "234" {
		t.Errorf("range: %d %q", code, body)
	}
	for _, path := range []string{"/full-ru.json", "/lesson4/lesson4.mkv", "/lesson4/../full-ru.json", "/lesson5/index.html"} {
		if code, _ := get(path, ""); code == 200 {
			t.Errorf("%s must not be served", path)
		}
	}
}
