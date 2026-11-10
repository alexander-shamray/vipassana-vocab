package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

const (
	claudeModel = "claude-opus-5-5"
	maxChars    = 165
	stressBatch = 50
)

const stressPrompt = `This is a Whisper transcript of a human Russian narration of a Vipassana talk by S. N. Goenka, for a speech synthesiser. For each phrase:
1. Correct ONLY recognition errors and punctuation ("о даме" is "о Дхамме", "собственное осуждение" is "собственное суждение"). Do not paraphrase, shorten, add, or move words between phrases.
2. Use only Cyrillic. Spell out numbers and times in words. Write Pali terms in Cyrillic (Дхамма, анапана, випассана, сила, самадхи, панья, санкхары).
3. Write ё wherever it belongs.
4. Put "+" immediately before the stressed vowel of EVERY word with two or more vowels, choosing by the meaning in context (зам+ок / з+амок). Example: "Наблюд+айте дых+ание так+им, как+ое он+о есть." No "+" in a word containing ё (ё is always stressed) or in a word with one vowel.
Return only a JSON array of {"id": n, "ru": "..."}, one per input phrase, same ids.`

const alignPrompt = `Two versions of the same talk by S. N. Goenka: "en" is the original English, "ru" a human Russian narration. Phrases are numbered and in order. For each ru phrase give the id of the en phrase where the same words begin. Keep the mapping non-decreasing. If the narration has no counterpart, give the id of the previous mapping. Return only a JSON array of [ru_id, en_id] pairs, one per ru phrase.`

type timedWord struct {
	Word  string   `json:"word"`
	Start float64  `json:"start"`
	End   *float64 `json:"end"`
}

type segment struct {
	Start        float64     `json:"start"`
	End          float64     `json:"end"`
	Words        []timedWord `json:"words"`
	AvgLogprob   float64     `json:"avg_logprob"`
	NoSpeechProb float64     `json:"no_speech_prob"`
}

// usableWords drops the segments Whisper itself doubts: mostly credits it invents in silence.
func usableWords(segs []segment) []timedWord {
	var out []timedWord
	for _, s := range segs {
		if s.AvgLogprob >= -1.0 && s.NoSpeechProb <= 0.9 {
			out = append(out, s.Words...)
		}
	}
	return out
}

func (w timedWord) end() float64 {
	if w.End != nil {
		return *w.End
	}
	return w.Start
}

func round2(x float64) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 2, 64), 64)
	return v
}

func lastRuneIn(s, set string) bool {
	if s == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune(set, r)
}

// splitPhrases cuts timed words into phrases the voices can read in one breath.
func splitPhrases(words []timedWord) []phrase {
	const pause = 0.45
	var out []phrase
	var cur []timedWord
	for i, w := range words {
		cur = append(cur, w)
		t := strings.TrimSpace(w.Word)
		gap := 9.0
		last := i+1 == len(words)
		if !last {
			gap = words[i+1].Start - w.end()
		}
		var b strings.Builder
		for _, c := range cur {
			b.WriteString(c.Word)
		}
		text := strings.TrimSpace(b.String())
		n := utf8.RuneCountInString(text)
		if last || (lastRuneIn(t, ".?!") && n >= 25) || (gap > pause && n >= 45) || (n > 110 && lastRuneIn(t, ",;:")) || n > maxChars-20 {
			out = append(out, phrase{ID: len(out), Start: round2(cur[0].Start), End: round2(cur[len(cur)-1].end()), Text: text})
			cur = nil
		}
	}
	return out
}

var latinOrDigit = regexp.MustCompile(`[a-zA-Z0-9]`)

// markProblems lists the words whose stress marks break the rules given to Claude.
func markProblems(text string) []string {
	var bad []string
	for _, word := range markedWord.FindAllString(text, -1) {
		low := []rune(strings.ToLower(word))
		marks, misplaced := 0, false
		for i, r := range low {
			if r == '+' {
				marks++
				if i+1 >= len(low) || !isVowel(low[i+1]) {
					misplaced = true
				}
			}
		}
		plain := strings.ReplaceAll(string(low), "+", "")
		yo := strings.Contains(plain, "ё")
		if misplaced || marks > 1 || (yo && marks > 0) || (vowelCount(plain) >= 2 && !yo && marks != 1) {
			bad = append(bad, word)
		}
	}
	if latinOrDigit.MatchString(text) {
		bad = append(bad, "latin-or-digit")
	}
	for _, r := range text {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Cyrillic, r) || unicode.Is(unicode.Mn, r) {
			bad = append(bad, "not-cyrillic") // once Claude wrote "ритु+алы", with a Devanagari vowel sign
			break
		}
	}
	return bad
}

// cyrillicShare guards against the wrong audio track: YouTube also serves an automatic English dub of the narration.
func cyrillicShare(text string) float64 {
	cyr, letters := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if unicode.Is(unicode.Cyrillic, r) {
				cyr++
			}
		}
	}
	if letters == 0 {
		return 0
	}
	return float64(cyr) / float64(letters)
}

type claudeCaller struct {
	client anthropic.Client
	mu     sync.Mutex
	input  int64
	output int64
}

func (c *claudeCaller) ask(ctx context.Context, prompt string, payload any, maxTokens int64, into any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	stream := c.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt + "\n\n" + string(data)))},
	})
	message := anthropic.Message{}
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return err
		}
	}
	if err := stream.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.input += message.Usage.InputTokens
	c.output += message.Usage.OutputTokens
	c.mu.Unlock()
	var text strings.Builder
	for _, block := range message.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	s := text.String()
	a, b := strings.Index(s, "["), strings.LastIndex(s, "]")
	if a < 0 || b < a {
		return errors.New("no JSON array in the answer")
	}
	return json.Unmarshal([]byte(s[a:b+1]), into)
}

type stressItem struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// stressBatchOnce asks Claude to clean and mark one batch, up to three times while the marks break the rules.
func (c *claudeCaller) stressOne(ctx context.Context, items []stressItem) map[int]string {
	for attempt := 0; attempt < 3; attempt++ {
		var answer []struct {
			ID int    `json:"id"`
			Ru string `json:"ru"`
		}
		if err := c.ask(ctx, stressPrompt, items, 9000, &answer); err != nil {
			fmt.Println("retry", items[0].ID, err)
			continue
		}
		got := make(map[int]string, len(answer))
		for _, a := range answer {
			got[a.ID] = a.Ru
		}
		complete := true
		bad := map[int][]string{}
		for _, it := range items {
			ru, ok := got[it.ID]
			if !ok {
				complete = false
				continue
			}
			if p := markProblems(ru); p != nil {
				bad[it.ID] = p
			}
		}
		if !complete {
			continue
		}
		if len(bad) <= 2 || attempt == 2 {
			if len(bad) > 0 {
				fmt.Println("left", bad)
			}
			return got
		}
		fmt.Println("recheck", items[0].ID, len(bad))
	}
	return nil
}

func readCache(path string) (map[int]string, error) {
	cache := map[int]string{}
	raw := map[string]string{}
	if err := readJSON(path, &raw); err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, err
	}
	for k, v := range raw {
		id, err := strconv.Atoi(k)
		if err != nil {
			return nil, err
		}
		cache[id] = v
	}
	return cache, nil
}

// runScript builds the lesson's phrases: Claude cleans the narrator's text, marks stress and maps each Russian phrase
// to the English one it translates. Finished batches are cached, so a stopped run costs nothing to resume.
func runScript(ctx context.Context, l lesson) error {
	var enFile struct {
		Segments []segment `json:"segments"`
	}
	if err := readJSON("../transcripts/"+l.En+".json", &enFile); err != nil {
		return err
	}
	var ruSegs []segment
	if err := readJSON(l.At("ru-transcript-full.json"), &ruSegs); err != nil {
		return err
	}
	en := splitPhrases(usableWords(enFile.Segments))
	ru := splitPhrases(usableWords(ruSegs))
	var all strings.Builder
	for _, p := range ru {
		all.WriteString(p.Text)
	}
	if share := cyrillicShare(all.String()); share < 0.9 {
		return fmt.Errorf("the narration is not Russian: %.0f%% Cyrillic. Wrong audio track? Nothing was sent to Claude", 100*share)
	}
	fmt.Println("en phrases", len(en), "ru phrases", len(ru))

	cachePath := l.At("full-stress-cache.json")
	cache, err := readCache(cachePath)
	if err != nil {
		return err
	}
	var todo []stressItem
	for _, p := range ru {
		if _, ok := cache[p.ID]; !ok {
			todo = append(todo, stressItem{p.ID, p.Text})
		}
	}
	if !exists(l.At("full-align.json")) || len(todo) > 0 {
		if os.Getenv("ANTHROPIC_API_KEY") == "" && os.Getenv("ANTHROPIC_AUTH_TOKEN") == "" {
			return errors.New("ANTHROPIC_API_KEY is not set: see docs/dub.md")
		}
	}
	caller := &claudeCaller{client: anthropic.NewClient()}
	var chunks [][]stressItem
	for i := 0; i < len(todo); i += stressBatch {
		chunks = append(chunks, todo[i:min(i+stressBatch, len(todo))])
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	done := 0
	var saveErr error
	for _, chunk := range chunks {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			got := caller.stressOne(ctx, chunk)
			mu.Lock()
			defer mu.Unlock()
			for id, ru := range got {
				cache[id] = ru
			}
			done++
			if err := writeJSON(cachePath, cache); err != nil && saveErr == nil {
				saveErr = err
			}
			fmt.Printf("stress %d/%d\n", done, len(chunks))
		}()
	}
	wg.Wait()
	if saveErr != nil {
		return saveErr
	}
	var missing []int
	for _, p := range ru {
		if _, ok := cache[p.ID]; !ok {
			missing = append(missing, p.ID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("no answer for phrases %v: run again, finished batches are kept", missing)
	}

	var pairs [][2]int
	alignPath := l.At("full-align.json")
	if err := readJSON(alignPath, &pairs); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		listing := func(ps []phrase) [][]any {
			out := make([][]any, len(ps))
			for i, p := range ps {
				out[i] = []any{p.ID, p.Text}
			}
			return out
		}
		payload := map[string]any{"en": listing(en), "ru": listing(ru)}
		if err := caller.ask(ctx, alignPrompt, payload, 24000, &pairs); err != nil {
			return fmt.Errorf("alignment: %w", err)
		}
		if err := writeJSON(alignPath, pairs); err != nil {
			return err
		}
	}
	mapping := make(map[int]int, len(pairs))
	for _, p := range pairs {
		mapping[p[0]] = p[1]
	}
	for i := range ru {
		ru[i].RuMarked = cache[ru[i].ID]
		if id, ok := mapping[ru[i].ID]; ok {
			ru[i].EnID = &id
		} else {
			return fmt.Errorf("Claude did not map Russian phrase %d: delete %s and run again", ru[i].ID, alignPath)
		}
	}
	if err := writeJSON(l.At("full-en.json"), en); err != nil {
		return err
	}
	if err := writeJSON(l.At("full-ru.json"), ru); err != nil {
		return err
	}
	for _, stale := range []string{"full-ru-accent.json", "full-ru.claude.json", "accent-diff.json"} { // made from the old text
		if err := os.Remove(l.At(stale)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	fmt.Printf("pairs %d, tokens %d in, %d out, about $%.2f\n", len(pairs), caller.input, caller.output, float64(caller.input)*4e-6+float64(caller.output)*20e-6)
	return nil
}
