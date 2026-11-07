package dict

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// LoadAll reads every dictionary entry with its forms, ordered by key.
func LoadAll(ctx context.Context, db *sql.DB) ([]Entry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, lemma, lang, pos, freq, ru_json, aliases_json, note, audio, reviewed, is_term
		FROM words ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	idx := map[string]int{}
	for rows.Next() {
		var e Entry
		var ru, aliases string
		if err := rows.Scan(&e.Key, &e.Lemma, &e.Lang, &e.POS, &e.Freq, &ru, &aliases, &e.Note,
			&e.Audio, &e.Reviewed, &e.IsTerm); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ru), &e.RU); err != nil {
			return nil, fmt.Errorf("word %q: bad ru_json: %w", e.Key, err)
		}
		if err := json.Unmarshal([]byte(aliases), &e.Aliases); err != nil {
			return nil, fmt.Errorf("word %q: bad aliases_json: %w", e.Key, err)
		}
		idx[e.Key] = len(out)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	frows, err := db.QueryContext(ctx, `SELECT word_key, form FROM forms ORDER BY word_key, form`)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var k, f string
		if err := frows.Scan(&k, &f); err != nil {
			return nil, err
		}
		if i, ok := idx[k]; ok {
			out[i].Forms = append(out[i].Forms, f)
		}
	}
	return out, frows.Err()
}

// ExamplesFor returns the stored examples of one word in order.
func ExamplesFor(ctx context.Context, db *sql.DB, key string) ([]Example, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT word_key, talk, t, video_id, text FROM examples WHERE word_key = ? ORDER BY ord`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Example
	for rows.Next() {
		var ex Example
		if err := rows.Scan(&ex.Key, &ex.Talk, &ex.T, &ex.VideoID, &ex.Text); err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}
