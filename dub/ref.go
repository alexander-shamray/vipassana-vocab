package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	refSource  = "cz7QHNvNFfA" // the English talk of day 1
	refSeconds = 25.0
)

type refWindow struct {
	start, end float64
	text       string
}

// bestWindow finds the stretch of about n seconds with the densest speech: pauses and doubtful segments make a worse sample.
func bestWindow(segs []segment, n float64) (refWindow, bool) {
	var clean []segment
	for _, s := range segs {
		if s.Start > 300 && s.NoSpeechProb < .2 {
			clean = append(clean, s)
		}
	}
	var best refWindow
	bestScore, found := 0.0, false
	for i := range clean {
		var words []timedWord
		for j := i; j < len(clean) && clean[j].End-clean[i].Start <= n; j++ {
			words = append(words, clean[j].Words...)
		}
		if len(words) == 0 {
			continue
		}
		span := words[len(words)-1].end() - words[0].Start
		if span < n*.7 {
			continue
		}
		speech := 0.0
		for _, w := range words {
			speech += w.end() - w.Start
		}
		if score := speech / span; !found || score > bestScore {
			var b strings.Builder
			for _, w := range words {
				b.WriteString(w.Word)
			}
			best, bestScore, found = refWindow{words[0].Start, words[len(words)-1].end(), strings.TrimSpace(b.String())}, score, true
		}
	}
	return best, found
}

// runRef cuts the sample XTTS clones the voice from. Every lesson uses the same one.
func runRef() error {
	var file struct {
		Segments []segment `json:"segments"`
	}
	if err := readJSON("../transcripts/"+refSource+".json", &file); err != nil {
		return err
	}
	w, ok := bestWindow(file.Segments, refSeconds)
	if !ok {
		return fmt.Errorf("no clean %.0f-second stretch in %s", refSeconds, refSource)
	}
	if err := os.MkdirAll("ref", 0o755); err != nil {
		return err
	}
	_, err := ffmpeg(nil, "-y", "-ss", strconv.FormatFloat(w.start, 'f', -1, 64), "-to", strconv.FormatFloat(w.end, 'f', -1, 64),
		"-i", "../audio/"+refSource+".m4a", "-ac", "1", "-ar", "24000", "-af", "highpass=f=70,lowpass=f=9000", filepath.Join("ref", "ref25.wav"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("ref", "ref25.txt"), []byte(w.text), 0o644); err != nil {
		return err
	}
	fmt.Printf("sample %.1f-%.1f s: %s\n", w.start, w.end, w.text)
	return nil
}
