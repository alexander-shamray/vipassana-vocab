package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_MigratesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sub", "app.sqlite")
	for i := 0; i < 2; i++ {
		d, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		var n int
		if err := d.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='word_marks'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("word_marks table missing after open #%d", i+1)
		}
		d.Close()
	}
}

func TestOpen_ForeignKeysOn(t *testing.T) {
	d, err := Open(context.Background(), filepath.Join(t.TempDir(), "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var on int
	if err := d.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatal("foreign_keys pragma is off")
	}
}
