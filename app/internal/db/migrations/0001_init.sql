-- +goose Up
CREATE TABLE words (
    key      TEXT PRIMARY KEY,
    lemma    TEXT NOT NULL,
    lang     TEXT NOT NULL CHECK (lang IN ('en', 'pali')),
    pos      TEXT NOT NULL DEFAULT '',
    freq     INTEGER NOT NULL DEFAULT 0,
    ru_json  TEXT NOT NULL DEFAULT '[]',
    note     TEXT,
    audio    TEXT NOT NULL DEFAULT '',
    reviewed INTEGER NOT NULL DEFAULT 0,
    is_term  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE forms (
    word_key TEXT NOT NULL REFERENCES words (key) ON DELETE CASCADE,
    form     TEXT NOT NULL,
    PRIMARY KEY (word_key, form)
);

CREATE TABLE examples (
    word_key TEXT NOT NULL REFERENCES words (key) ON DELETE CASCADE,
    ord      INTEGER NOT NULL,
    talk     INTEGER NOT NULL,
    t        INTEGER NOT NULL,
    video_id TEXT NOT NULL,
    text     TEXT NOT NULL,
    PRIMARY KEY (word_key, ord)
);

CREATE TABLE users (
    id         INTEGER PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE COLLATE NOCASE,
    name       TEXT NOT NULL DEFAULT '',
    provider   TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- A user's mark on a word: 'hidden' (learnt) or 'important'. No row means
-- the word is on the main list. word_key deliberately has no foreign key to
-- words: rebuilding the dictionary must never delete a user's progress.
CREATE TABLE word_marks (
    user_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    word_key  TEXT NOT NULL,
    mark      TEXT NOT NULL CHECK (mark IN ('hidden', 'important')),
    marked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (user_id, word_key)
);

-- Schema required by github.com/alexedwards/scs/sqlite3store.
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- +goose Down
DROP TABLE sessions;
DROP TABLE word_marks;
DROP TABLE users;
DROP TABLE examples;
DROP TABLE forms;
DROP TABLE words;
