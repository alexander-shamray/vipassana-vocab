package dict

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ImportWords replaces the dictionary with the entries read from r, in one
// transaction. Words missing from r are deleted, but word_marks rows are
// keyed by word_key without a foreign key, so user progress survives and
// reappears if the word comes back.
func ImportWords(ctx context.Context, db *sql.DB, r io.Reader) (Stats, error) {
	var st Stats
	entries, err := readJSONL[Entry](r)
	if err != nil {
		return st, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return st, err
	}
	defer tx.Rollback() // no-op after Commit

	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if err := validate(e); err != nil {
			return Stats{}, fmt.Errorf("words line %d: %w", i+1, err)
		}
		if seen[e.Key] {
			return Stats{}, fmt.Errorf("words line %d: duplicate key %q", i+1, e.Key)
		}
		seen[e.Key] = true
		ru, _ := json.Marshal(nonNil(e.RU))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO words (key, lemma, lang, pos, freq, ru_json, note, audio, reviewed, is_term)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET
				lemma = excluded.lemma, lang = excluded.lang, pos = excluded.pos,
				freq = excluded.freq, ru_json = excluded.ru_json, note = excluded.note,
				audio = excluded.audio, reviewed = excluded.reviewed, is_term = excluded.is_term`,
			e.Key, e.Lemma, e.Lang, e.POS, e.Freq, string(ru), e.Note, e.Audio, e.Reviewed, e.IsTerm); err != nil {
			return Stats{}, fmt.Errorf("upsert %q: %w", e.Key, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM forms WHERE word_key = ?`, e.Key); err != nil {
			return Stats{}, err
		}
		for _, f := range e.Forms {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO forms (word_key, form) VALUES (?, ?)`, e.Key, f); err != nil {
				return Stats{}, err
			}
		}
		st.Upserted++
	}

	stale, err := staleKeys(ctx, tx, seen)
	if err != nil {
		return Stats{}, err
	}
	for _, k := range stale {
		if _, err := tx.ExecContext(ctx, `DELETE FROM words WHERE key = ?`, k); err != nil {
			return Stats{}, err
		}
		st.Deleted++
	}
	return st, tx.Commit()
}

// ImportExamples replaces all examples with those read from r, keeping at
// most MaxExamples per word in file order and skipping unknown words.
func ImportExamples(ctx context.Context, db *sql.DB, r io.Reader) (Stats, error) {
	var st Stats
	exs, err := readJSONL[Example](r)
	if err != nil {
		return st, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return st, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM examples`); err != nil {
		return st, err
	}
	known, err := allKeys(ctx, tx)
	if err != nil {
		return st, err
	}
	count := map[string]int{}
	for _, ex := range exs {
		if !known[ex.Key] || count[ex.Key] >= MaxExamples {
			st.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO examples (word_key, ord, talk, t, video_id, text) VALUES (?, ?, ?, ?, ?, ?)`,
			ex.Key, count[ex.Key], ex.Talk, ex.T, ex.VideoID, ex.Text); err != nil {
			return Stats{}, fmt.Errorf("insert example for %q: %w", ex.Key, err)
		}
		count[ex.Key]++
		st.Upserted++
	}
	return st, tx.Commit()
}

func validate(e Entry) error {
	switch {
	case strings.TrimSpace(e.Key) == "":
		return errors.New("empty key")
	case strings.TrimSpace(e.Lemma) == "":
		return errors.New("empty lemma")
	case e.Lang != "en" && e.Lang != "pali":
		return fmt.Errorf("lang %q is not en or pali", e.Lang)
	}
	return nil
}

// readJSONL decodes one JSON value per non-blank line; any bad line fails
// the whole read, so a broken file never half-replaces the dictionary.
func readJSONL[T any](r io.Reader) ([]T, error) {
	var out []T
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func allKeys(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT key FROM words`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys[k] = true
	}
	return keys, rows.Err()
}

func staleKeys(ctx context.Context, tx *sql.Tx, seen map[string]bool) ([]string, error) {
	all, err := allKeys(ctx, tx)
	if err != nil {
		return nil, err
	}
	var stale []string
	for k := range all {
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	return stale, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
