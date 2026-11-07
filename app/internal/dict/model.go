// Package dict holds the dictionary model and imports it into SQLite.
package dict

// Entry is one line of dictionary/words.jsonl.
type Entry struct {
	Key      string   `json:"key"`
	Lemma    string   `json:"lemma"`
	Lang     string   `json:"lang"` // "en" or "pali"
	POS      string   `json:"pos"`
	Forms    []string `json:"forms"`
	Aliases  []string `json:"aliases"` // search-only spellings, not heard forms
	Freq     int      `json:"freq"`
	RU       []string `json:"ru"`
	Note     *string  `json:"note"`
	Audio    string   `json:"audio"`
	Reviewed bool     `json:"reviewed"`
	IsTerm   bool     `json:"is_term"`
}

// Draft reports whether the translation of a Dhamma or Pali term still
// awaits the owner's review.
func (e Entry) Draft() bool { return e.IsTerm && !e.Reviewed }

// Example is one line of examples.jsonl: a short quote from a talk.
type Example struct {
	Key     string `json:"key"`
	Talk    int    `json:"talk"`
	T       int    `json:"t"` // seconds from the start of the video
	VideoID string `json:"video_id"`
	Text    string `json:"text"`
}

// MaxExamples is the most quotes the site keeps per word (copyright limit).
const MaxExamples = 3

// Stats reports what an import changed.
type Stats struct {
	Upserted, Deleted, Skipped int
}
