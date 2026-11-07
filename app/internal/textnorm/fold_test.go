package textnorm

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFold_Cases(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/fold_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ In, Fold, Key string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := Fold(c.In); got != c.Fold {
			t.Errorf("Fold(%q) = %q, want %q", c.In, got, c.Fold)
		}
		if got := Key(c.In); got != c.Key {
			t.Errorf("Key(%q) = %q, want %q", c.In, got, c.Key)
		}
	}
}
