// Command dub builds a Russian dub of a lesson: every step except the neural models, which stay in py/.
// See docs/dub.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const usage = `dub: Russian dub of a lesson. Full guide: docs/dub.md

  dub run <lesson>       all steps below in order, skipping what is already done

  dub ref                cut the voice sample for XTTS from the English talk of day 1 (once, for all lessons)
  dub fetch <lesson>     download the English video and the Russian narration
  dub whisper <lesson>   recognize the narration
  dub script <lesson>    Claude cleans the text, marks stress, maps phrases to English (costs about $3-4)
  dub accent <lesson>    RuAccent: a second opinion on stress
  dub review <lesson>    print the stress disagreements no earlier lesson has shown
  dub marks <lesson>     final stress marks: stress-fixes.json, then RuAccent, then Claude
  dub silero <lesson>    voice: Silero eugene
  dub xtts <lesson>      voice: XTTS, cloned from the English talk; a Whisper check; fresh takes of the wrong phrases
  dub fit <lesson>       lay the tracks on the English timeline, build the files and the .mkv (-stats: only report)
  dub page <lesson>      write the player page
  dub serve              open the players to this Wi-Fi network (-addr, default :8765). No password

Options: -home <folder> working folder, default data/dub-pilot.
`

const (
	whisperPython = "../../pipeline/.venv/Scripts/python"
	pyDir         = "../../dub/py/"
)

func pythonEnv(l lesson) []string {
	return append(os.Environ(), "DUB_WORK="+l.Work(), "DUB_RU_MEDIA="+l.RuMediaName(), "PYTHONUTF8=1", "PYTHONWARNINGS=ignore")
}

func runPython(l lesson, python, script string, args ...string) error {
	cmd := exec.Command(python, append([]string{pyDir + script}, args...)...)
	cmd.Env = pythonEnv(l)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr // XTTS asks once to accept its licence
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", script, err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runFetch(l lesson) error {
	if err := os.MkdirAll(l.Work(), 0o755); err != nil {
		return err
	}
	ytdlp := "../../pipeline/.venv/Scripts/yt-dlp"
	download := func(format, path, id string) error {
		if exists(path) {
			return nil
		}
		cmd := exec.Command(ytdlp, "-q", "--no-warnings", "-f", format, "-o", path, "https://www.youtube.com/watch?v="+id)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := download("135/bv*[height<=480][ext=mp4]", l.At("en-video-480.mp4"), l.En); err != nil {
		return fmt.Errorf("English video: %w", err)
	}
	// The narration also comes with an automatic English dub under the same format numbers: ask for the language.
	if err := download("ba[ext=m4a][language^=ru]", l.At(l.RuMediaName()), l.Ru); err != nil {
		return fmt.Errorf("Russian narration: %w", err)
	}
	if !exists("../audio/" + l.En + ".m4a") {
		return fmt.Errorf("no English audio ../audio/%s.m4a: the dictionary pipeline downloads it", l.En)
	}
	if !exists("../transcripts/" + l.En + ".json") {
		return fmt.Errorf("no English transcript ../transcripts/%s.json: the dictionary pipeline makes it", l.En)
	}
	return nil
}

func runXTTS(l lesson) error {
	if err := runPython(l, ".venv-xtts/Scripts/python", "full_xtts.py"); err != nil {
		return err
	}
	if !exists(l.At("check-xtts-clone.first.json")) {
		if err := runPython(l, whisperPython, "check_phrases.py", "xtts-clone"); err != nil {
			return err
		}
		raw, err := os.ReadFile(l.At("check-xtts-clone.json"))
		if err != nil {
			return err
		}
		if err := os.WriteFile(l.At("check-xtts-clone.first.json"), raw, 0o644); err != nil {
			return err
		}
		if err := runPython(l, whisperPython, "pick_xtts.py", "init"); err != nil {
			return err
		}
	}
	for take := 1; take <= 4; take++ {
		s := strconv.Itoa(take)
		if err := runPython(l, ".venv-xtts/Scripts/python", "retry_xtts.py", s); err != nil {
			return err
		}
		if err := runPython(l, whisperPython, "pick_xtts.py", s); err != nil {
			return err
		}
	}
	return nil
}

func phrasesDone(l lesson, engine string) bool {
	var ru []phrase
	if readJSON(l.At("full-ru.json"), &ru) != nil {
		return false
	}
	entries, err := os.ReadDir(l.At(filepath.Join("out-full", engine)))
	return err == nil && len(entries) >= len(ru)
}

func runAll(ctx context.Context, l lesson) error {
	type step struct {
		name string
		done func() bool
		run  func() error
	}
	steps := []step{
		{"ref", func() bool { return exists(filepath.Join("ref", "ref25.wav")) }, runRef},
		{"fetch", func() bool { return exists(l.At("en-video-480.mp4")) && exists(l.At(l.RuMediaName())) }, func() error { return runFetch(l) }},
		{"whisper", func() bool { return exists(l.At("ru-transcript-full.json")) }, func() error { return runPython(l, whisperPython, "transcribe.py") }},
		{"script", func() bool { return exists(l.At("full-ru.json")) }, func() error { return runScript(ctx, l) }},
		{"accent", func() bool { return exists(l.At("full-ru-accent.json")) }, func() error { return runPython(l, ".venv-accent/Scripts/python", "accent_compare.py") }},
		{"marks", func() bool { return exists(l.At("full-ru.claude.json")) }, func() error { return runMarks(l) }},
		{"silero", func() bool { return phrasesDone(l, "silero-eugene") }, func() error { return runPython(l, ".venv-main/Scripts/python", "full_silero.py") }},
		{"xtts", func() bool { return exists(l.At("check-xtts-clone.first.json")) }, func() error { return runXTTS(l) }},
		{"fit", func() bool { return exists(filepath.Join(l.Out(), "tracks.json")) }, func() error { return runFit(l, false) }},
		{"page", func() bool { return exists(filepath.Join(l.Out(), "index.html")) }, func() error { return runPage(l) }},
	}
	for _, s := range steps {
		if s.done() {
			fmt.Println("==", s.name, "already done")
			continue
		}
		fmt.Println("==", s.name)
		if err := s.run(); err != nil {
			return fmt.Errorf("step %s: %w", s.name, err)
		}
	}
	return nil
}

func run(args []string) error {
	root := flag.NewFlagSet("dub", flag.ContinueOnError)
	root.Usage = func() { fmt.Print(usage) }
	home := root.String("home", filepath.Join("data", "dub-pilot"), "working folder")
	if err := root.Parse(args); err != nil {
		return err
	}
	rest := root.Args()
	if len(rest) == 0 {
		fmt.Print(usage)
		return nil
	}
	if err := os.Chdir(*home); err != nil {
		return fmt.Errorf("working folder: %w", err)
	}
	command, rest := rest[0], rest[1:]
	if command == "ref" {
		return runRef()
	}
	if command == "serve" {
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		addr := fs.String("addr", ":8765", "address to listen on")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return runServe(*addr)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	stats := fs.Bool("stats", false, "fit: only report how the tracks fit")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("dub %s needs a lesson number", command)
	}
	l, err := findLesson(fs.Arg(0))
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch command {
	case "run":
		return runAll(ctx, l)
	case "fetch":
		return runFetch(l)
	case "whisper":
		return runPython(l, whisperPython, "transcribe.py")
	case "script":
		return runScript(ctx, l)
	case "accent":
		return runPython(l, ".venv-accent/Scripts/python", "accent_compare.py")
	case "review":
		return runReview(l)
	case "marks":
		return runMarks(l)
	case "silero":
		return runPython(l, ".venv-main/Scripts/python", "full_silero.py")
	case "xtts":
		return runXTTS(l)
	case "fit":
		return runFit(l, *stats)
	case "page":
		return runPage(l)
	}
	return fmt.Errorf("unknown command %q", command)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dub:", err)
		os.Exit(1)
	}
}
