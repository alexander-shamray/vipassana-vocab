package dict

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/db"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func sample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func count(t *testing.T, d *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestImportWords_Idempotent(t *testing.T) {
	ctx, d := context.Background(), openTestDB(t)
	words := sample(t, "words.sample.jsonl")
	if _, err := ImportWords(ctx, d, strings.NewReader(words)); err != nil {
		t.Fatal(err)
	}
	st, err := ImportWords(ctx, d, strings.NewReader(words))
	if err != nil {
		t.Fatal(err)
	}
	if st.Upserted != 30 || st.Deleted != 0 {
		t.Errorf("second import stats = %+v, want 30 upserted, 0 deleted", st)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM words`); n != 30 {
		t.Errorf("words = %d, want 30", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM forms WHERE word_key = 'suffer'`); n != 4 {
		t.Errorf("forms of suffer = %d, want 4", n)
	}
}

func TestImportWords_RemovedWordKeepsHiddenRow(t *testing.T) {
	ctx, d := context.Background(), openTestDB(t)
	words := sample(t, "words.sample.jsonl")
	if _, err := ImportWords(ctx, d, strings.NewReader(words)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO users (id, email, provider) VALUES (1, 'a@x.com', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO word_marks (user_id, word_key, mark) VALUES (1, 'suffer', 'hidden')`); err != nil {
		t.Fatal(err)
	}

	var without []string
	for _, l := range strings.Split(words, "\n") {
		if !strings.Contains(l, `"key": "suffer"`) && !strings.Contains(l, `"key":"suffer"`) {
			without = append(without, l)
		}
	}
	st, err := ImportWords(ctx, d, strings.NewReader(strings.Join(without, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if st.Deleted != 1 {
		t.Errorf("deleted = %d, want 1", st.Deleted)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM words WHERE key = 'suffer'`); n != 0 {
		t.Error("suffer should be gone from words")
	}
	if n := count(t, d, `SELECT COUNT(*) FROM word_marks WHERE user_id = 1 AND word_key = 'suffer'`); n != 1 {
		t.Error("hidden row for suffer must survive the rebuild")
	}

	if _, err := ImportWords(ctx, d, strings.NewReader(words)); err != nil {
		t.Fatal(err)
	}
	n := count(t, d, `SELECT COUNT(*) FROM words w JOIN word_marks m ON m.word_key = w.key WHERE m.user_id = 1 AND m.mark = 'hidden'`)
	if n != 1 {
		t.Error("suffer should be hidden again for user 1 after it returns")
	}
}

func TestImportExamples_CapsAtThreeAndSkipsUnknown(t *testing.T) {
	ctx, d := context.Background(), openTestDB(t)
	if _, err := ImportWords(ctx, d, strings.NewReader(sample(t, "words.sample.jsonl"))); err != nil {
		t.Fatal(err)
	}
	st, err := ImportExamples(ctx, d, strings.NewReader(sample(t, "examples.sample.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	// 5 for suffer (2 over the cap) + 1 unknown key.
	if st.Skipped != 3 || st.Upserted != 6 {
		t.Errorf("stats = %+v, want 6 upserted, 3 skipped", st)
	}
	exs, err := ExamplesFor(ctx, d, "suffer")
	if err != nil {
		t.Fatal(err)
	}
	if len(exs) != MaxExamples || exs[0].T != 612 {
		t.Errorf("suffer examples = %+v, want first 3 in file order", exs)
	}
}

func TestImportWords_BadLineFailsWholeImport(t *testing.T) {
	ctx, d := context.Background(), openTestDB(t)
	words := sample(t, "words.sample.jsonl")
	if _, err := ImportWords(ctx, d, strings.NewReader(words)); err != nil {
		t.Fatal(err)
	}
	broken := `{"key":"new","lemma":"new","lang":"en"}` + "\n{not json\n"
	_, err := ImportWords(ctx, d, strings.NewReader(broken))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want a line 2 error", err)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM words`); n != 30 {
		t.Errorf("words = %d after failed import, want 30 unchanged", n)
	}
}

func TestImportWords_RejectsBadLang(t *testing.T) {
	ctx, d := context.Background(), openTestDB(t)
	_, err := ImportWords(ctx, d, strings.NewReader(`{"key":"x","lemma":"x","lang":"hi"}`))
	if err == nil {
		t.Fatal("lang hi must be rejected")
	}
}

func TestLoadAll_RoundTrip(t *testing.T) {
	ctx, d := context.Background(), openTestDB(t)
	if _, err := ImportWords(ctx, d, strings.NewReader(sample(t, "words.sample.jsonl"))); err != nil {
		t.Fatal(err)
	}
	es, err := LoadAll(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 30 {
		t.Fatalf("loaded %d entries, want 30", len(es))
	}
	for _, e := range es {
		if e.Key == "panna" && (e.Lemma != "paññā" || !e.Draft() || e.RU[0] != "мудрость") {
			t.Errorf("panna round trip: %+v", e)
		}
	}
}
