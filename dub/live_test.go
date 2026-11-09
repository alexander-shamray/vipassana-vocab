package main

import (
	"context"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// TestLiveClaude makes one real request for two phrases, about a cent. It runs only with DUB_LIVE=1.
func TestLiveClaude(t *testing.T) {
	if os.Getenv("DUB_LIVE") != "1" {
		t.Skip("set DUB_LIVE=1 to call the API")
	}
	c := &claudeCaller{client: anthropic.NewClient()}
	got := c.stressOne(context.Background(), []stressItem{{7, "Закончился четвертый день."}, {8, "Вам осталось работать еще 6 дней."}})
	if got[7] == "" || got[8] == "" {
		t.Fatalf("no answer: %v", got)
	}
	for id, ru := range got {
		if p := markProblems(ru); p != nil {
			t.Errorf("phrase %d %q: %v", id, ru, p)
		}
	}
	t.Logf("%v, tokens %d in, %d out", got, c.input, c.output)
}
