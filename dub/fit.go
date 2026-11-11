package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"unicode/utf8"
)

const (
	sampleRate   = 48000
	phraseGap    = 0.12
	maxTempo     = 1.3 // faster blurs the words
	tempoSpan    = 60.0
	catchUp      = 30.0 // a track this many seconds late speeds up by a whole 1.0: falling behind costs more than speed
	longestPause = 0.2  // inside a synthesized phrase
)

// ffmpegPath prefers the copy in bin/, then the one on PATH.
func ffmpegPath() string {
	if abs, err := filepath.Abs(filepath.Join("bin", "ffmpeg.exe")); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return "ffmpeg"
}

func ffmpeg(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command(ffmpegPath(), append([]string{"-hide_banner", "-loglevel", "error"}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg %v: %w: %s", args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func toSamples(raw []byte) []float32 {
	out := make([]float32, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out
}

func toBytes(a []float32) []byte {
	out := make([]byte, len(a)*4)
	for i, v := range a {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

// readPCM decodes any media file to mono samples at the working rate.
func readPCM(path string) ([]float32, error) {
	raw, err := ffmpeg(nil, "-i", path, "-ac", "1", "-af", "aresample="+strconv.Itoa(sampleRate), "-f", "f32le", "-")
	if err != nil {
		return nil, err
	}
	return toSamples(raw), nil
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// trimSilence cuts the silence around a synthesized phrase, so the gaps between phrases are ours.
func trimSilence(a []float32) []float32 {
	var peak float32
	for _, v := range a {
		peak = max(peak, abs32(v))
	}
	limit := 0.01 * max(peak, 1e-6)
	first, last := -1, -1
	for i, v := range a {
		if abs32(v) > limit {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return a
	}
	return a[max(0, first-int(.02*sampleRate)):min(len(a), last+int(.08*sampleRate))]
}

// shortenPauses cuts every pause inside a phrase down to longest seconds, keeping its two ends. XTTS pauses long at
// commas: over a lesson that is eight minutes the English has no room for.
func shortenPauses(a []float32, longest float64) []float32 {
	const frame = sampleRate / 100
	var peak float32
	for _, v := range a {
		peak = max(peak, abs32(v))
	}
	limit := 0.02 * max(peak, 1e-6)
	silent := func(f int) bool {
		for _, v := range a[f*frame : min((f+1)*frame, len(a))] {
			if abs32(v) > limit {
				return false
			}
		}
		return true
	}
	keep := int(longest * sampleRate / 2)
	frames := (len(a) + frame - 1) / frame
	out := make([]float32, 0, len(a))
	from := 0 // start of the audio not yet copied
	for f := 0; f < frames; {
		if !silent(f) {
			f++
			continue
		}
		g := f
		for g < frames && silent(g) {
			g++
		}
		start, end := f*frame, min(g*frame, len(a))
		if f > 0 && g < frames && end-start > 2*keep {
			out = append(out, a[from:start+keep]...)
			from = end - keep
		}
		f = g
	}
	return append(out, a[from:]...)
}

// speedUp changes tempo, not pitch.
func speedUp(a []float32, tempo float64) ([]float32, error) {
	if tempo <= 1.001 {
		return a, nil
	}
	raw, err := ffmpeg(toBytes(a), "-f", "f32le", "-ar", strconv.Itoa(sampleRate), "-ac", "1", "-i", "-",
		"-af", fmt.Sprintf("atempo=%.4f", tempo), "-f", "f32le", "-")
	if err != nil {
		return nil, err
	}
	return toSamples(raw), nil
}

type fitStats struct {
	Phrases    int     `json:"phrases"`
	TempoMean  float64 `json:"tempo_mean"`
	TempoAtCap int     `json:"tempo_at_cap"`
	LateMedian float64 `json:"late_median_s"`
	LateP90    float64 `json:"late_p90_s"`
	LateMax    float64 `json:"late_max_s"`
	EndS       float64 `json:"end_s"`
	GainDB     float64 `json:"gain_db"` // brings the track to the loudness of the original
	// Behind holds the clips that ended at the tempo cap or more than behindS late: dub hurry voices them faster.
	Behind []int `json:"-"`
}

const behindS = 2.0

// dueTimes says when Goenka says the words of each Russian phrase. Phrases that share one English phrase split its
// time by their length, so the second of two starts halfway through it, not at its start.
func dueTimes(ru []phrase, en map[int]phrase) ([]float64, error) {
	weight := func(p phrase) float64 { return float64(max(utf8.RuneCountInString(p.RuMarked), 1)) }
	total := map[int]float64{}
	for _, p := range ru {
		if p.EnID == nil {
			return nil, fmt.Errorf("phrase %d has no English counterpart", p.ID)
		}
		total[*p.EnID] += weight(p)
	}
	before := map[int]float64{}
	out := make([]float64, len(ru))
	for i, p := range ru {
		e := en[*p.EnID]
		out[i] = e.Start + (e.End-e.Start)*before[*p.EnID]/total[*p.EnID]
		before[*p.EnID] += weight(p)
	}
	return out, nil
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// layOut places each phrase at the moment Goenka says the same words. The tempo changes smoothly: it follows how much
// text the next minute holds and how late the track already is. clips[i] is the audio of phrase i, or nil.
func layOut(clips [][]float32, targets []float64, total float64) ([]float32, fitStats, error) {
	durs := make([]float64, len(clips))
	for i, c := range clips {
		durs[i] = float64(len(c)) / sampleRate
	}
	buf := make([]float32, int((total+120)*sampleRate))
	cursor := 0.0
	var late, tempos []float64
	var behind []int
	for i, c := range clips {
		if c == nil {
			continue
		}
		t := max(targets[i], cursor)
		lateness := t - targets[i]
		need := 0.0
		j := i
		for j < len(clips) && targets[j] < targets[i]+tempoSpan {
			need += durs[j] + phraseGap
			j++
		}
		end := total
		if j < len(clips) {
			end = targets[j]
		}
		ratio := need / max(end-targets[i]-lateness, 1.0)
		tempo := min(max(max(ratio, 1.0)+lateness/catchUp, 1.0), maxTempo)
		a, err := speedUp(c, tempo)
		if err != nil {
			return nil, fitStats{}, err
		}
		n := int(t * sampleRate)
		for k := 0; k < len(a) && n+k < len(buf); k++ {
			buf[n+k] += a[k]
		}
		cursor = t + float64(len(a))/sampleRate + phraseGap
		late = append(late, lateness)
		tempos = append(tempos, tempo)
		if tempo >= maxTempo-.001 || lateness > behindS {
			behind = append(behind, i)
		}
	}
	var sum float64
	speech := 0
	for _, v := range buf {
		if abs32(v) > 0.01 {
			sum += float64(v) * float64(v)
			speech++
		}
	}
	rms := 1.0
	if speech > 0 {
		rms = math.Sqrt(sum / float64(speech))
	}
	gain := float32(0.1 / max(rms, 1e-6)) // a working level; the final gain matches the original
	for i, v := range buf {
		buf[i] = min(max(v*gain, -0.98), 0.98)
	}
	sort.Float64s(late)
	var tempoSum float64
	atCap := 0
	for _, x := range tempos {
		tempoSum += x
		if x >= maxTempo-.001 {
			atCap++
		}
	}
	stats := fitStats{
		Phrases: len(late), TempoMean: math.Round(tempoSum/float64(len(tempos))*1000) / 1000, TempoAtCap: atCap,
		LateMedian: round1(late[len(late)/2]), LateP90: round1(late[int(float64(len(late))*.9)]), LateMax: round1(late[len(late)-1]),
		EndS: round1(cursor), Behind: behind,
	}
	return buf[:min(int(max(total, cursor+1)*sampleRate), len(buf))], stats, nil
}

// narratorClips cuts the human narration into the same phrases.
func narratorClips(l lesson, ru []phrase) ([][]float32, error) {
	whole, err := readPCM(l.At(l.RuMediaName()))
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(ru))
	for i, p := range ru {
		end := p.End + .3
		if i+1 < len(ru) {
			end = ru[i+1].Start
		}
		end = min(p.End+.12, end)
		from := min(int(max(p.Start-.05, 0)*sampleRate), len(whole))
		to := max(min(int(end*sampleRate), len(whole)), from)
		a := append([]float32(nil), whole[from:to]...)
		fade := min(int(.01*sampleRate), len(a)/2)
		for k := 0; k < fade; k++ {
			g := float32(k) / float32(max(fade-1, 1))
			a[k] *= g
			a[len(a)-1-k] *= g
		}
		out[i] = a
	}
	return out, nil
}

// engineClips reads a voice's phrases, several at a time: each one starts an ffmpeg.
func engineClips(l lesson, engine string, ru []phrase) ([][]float32, error) {
	out := make([][]float32, len(ru))
	errs := make([]error, len(ru))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for i, p := range ru {
		path := l.At(fmt.Sprintf("out-full/%s/%04d.wav", engine, p.ID))
		if _, err := os.Stat(path); err != nil {
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			a, err := readPCM(path)
			if err != nil {
				errs[i] = err
				return
			}
			out[i] = shortenPauses(trimSilence(a), longestPause)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// voices are the synthesized tracks, built when their folders exist.
var voices = []struct{ id, title string }{{"silero-eugene", "Silero eugene"}, {"xtts-clone", "XTTS, клон голоса"}}

var durationRe = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+(?:\.\d+)?)`)

func mediaSeconds(path string) (float64, error) {
	cmd := exec.Command(ffmpegPath(), "-hide_banner", "-i", path)
	output, _ := cmd.CombinedOutput() // ffmpeg exits with an error when given no output file
	m := durationRe.FindSubmatch(output)
	if m == nil {
		return 0, fmt.Errorf("no duration in ffmpeg's report on %s", path)
	}
	h, _ := strconv.ParseFloat(string(m[1]), 64)
	mi, _ := strconv.ParseFloat(string(m[2]), 64)
	s, _ := strconv.ParseFloat(string(m[3]), 64)
	return h*3600 + mi*60 + s, nil
}

type trackInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type tracksFile struct {
	Tracks      []trackInfo         `json:"tracks"`
	Report      map[string]fitStats `json:"report"`
	EnglishLUFS float64             `json:"english_lufs,omitempty"`
}

var integratedRe = regexp.MustCompile(`I:\s+(-?\d+(?:\.\d+)?) LUFS`)

// parseLoudness reads the integrated loudness from the summary ffmpeg's ebur128 filter prints at the end.
func parseLoudness(report string) (float64, error) {
	m := integratedRe.FindAllStringSubmatch(report, -1)
	if m == nil {
		return 0, fmt.Errorf("no integrated loudness in ffmpeg's report")
	}
	return strconv.ParseFloat(m[len(m)-1][1], 64)
}

// loudness measures integrated loudness by EBU R128, the way people hear it: quiet passages and pauses are gated out.
// A mono track is measured as two equal channels, like a stereo recording of one voice.
func loudness(input ...string) (float64, error) {
	args := append([]string{"-hide_banner", "-nostats"}, input...)
	args = append(args, "-af", "aformat=channel_layouts=stereo,ebur128=framelog=quiet", "-f", "null", "-")
	output, err := exec.Command(ffmpegPath(), args...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffmpeg loudness %v: %w: %s", input, err, output)
	}
	return parseLoudness(string(output))
}

// matchGain is the gain that brings a track to the loudness of the original, never more than +12 dB.
func matchGain(track, original float64) float64 {
	return math.Round(min(original-track, 12)*100) / 100
}

// gainFilter applies the gain; a limiter keeps a raised track from clipping.
func gainFilter(db float64) string {
	f := fmt.Sprintf("volume=%.2fdB", db)
	if db > 0 {
		f += ",alimiter=limit=0.95:level=false"
	}
	return f
}

func writeFloats(path string, a []float32) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	var b [4]byte
	for _, v := range a {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		if _, err := w.Write(b[:]); err != nil {
			return err
		}
	}
	return w.Flush()
}

// runFit lays the Russian tracks on the English timeline and builds the player's files and the .mkv.
// The picture is not re-encoded and the English sound is copied as it is; only the Russian tracks are encoded, to AAC.
// With statsOnly it only reports how the tracks fit.
// timeline reads a lesson's phrases, when each is due on the English video, and the video's length.
func timeline(l lesson) ([]phrase, []float64, float64, error) {
	var ru, enList []phrase
	if err := readJSON(l.At("full-ru.json"), &ru); err != nil {
		return nil, nil, 0, err
	}
	if err := readJSON(l.At("full-en.json"), &enList); err != nil {
		return nil, nil, 0, err
	}
	en := make(map[int]phrase, len(enList))
	for _, p := range enList {
		en[p.ID] = p
	}
	targets, err := dueTimes(ru, en)
	if err != nil {
		return nil, nil, 0, err
	}
	total, err := mediaSeconds(l.At("en-video-480.mp4"))
	return ru, targets, total, err
}

func runFit(l lesson, statsOnly bool) error {
	ru, targets, total, err := timeline(l)
	if err != nil {
		return err
	}
	video := l.At("en-video-480.mp4")
	out := l.Out()
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	type track struct {
		id, title string
		clips     func() ([][]float32, error)
	}
	tracks := []track{{"narrator", "Русский диктор", func() ([][]float32, error) { return narratorClips(l, ru) }}}
	for _, e := range voices {
		if st, err := os.Stat(l.At("out-full/" + e.id)); err == nil && st.IsDir() {
			tracks = append(tracks, track{e.id, e.title, func() ([][]float32, error) { return engineClips(l, e.id, ru) }})
		} else {
			fmt.Println("no voice yet:", e.id)
		}
	}
	report := map[string]fitStats{}
	englishAudio := data("audio", l.En+".m4a")
	englishLUFS := 0.0
	if !statsOnly {
		var err error
		if englishLUFS, err = loudness("-i", englishAudio); err != nil {
			return err
		}
		fmt.Printf("original: %.1f LUFS\n", englishLUFS)
	}
	list := []trackInfo{{"english", "Гоенка, оригинал"}}
	for _, t := range tracks {
		clips, err := t.clips()
		if err != nil {
			return err
		}
		var missing []int
		for i, c := range clips {
			if c == nil {
				missing = append(missing, ru[i].ID)
			}
		}
		if len(missing) == len(clips) {
			fmt.Println("no voice yet:", t.id)
			continue
		}
		if len(missing) > 0 {
			fmt.Println("WARNING:", t.id, "has no audio for phrases", missing, "and will be silent there")
		}
		buf, stats, err := layOut(clips, targets, total)
		if err != nil {
			return err
		}
		list = append(list, trackInfo{t.id, t.title})
		if statsOnly {
			report[t.id] = stats
			line, _ := json.Marshal(stats)
			fmt.Println(t.id, string(line))
			continue
		}
		tmp := filepath.Join(out, t.id+".f32")
		if err := writeFloats(tmp, buf); err != nil {
			return err
		}
		raw := []string{"-f", "f32le", "-ar", strconv.Itoa(sampleRate), "-ac", "1", "-i", tmp}
		measured, err := loudness(raw...)
		if err != nil {
			os.Remove(tmp)
			return err
		}
		stats.GainDB = matchGain(measured, englishLUFS)
		_, err = ffmpeg(nil, append(append([]string{"-y"}, raw...), "-af", gainFilter(stats.GainDB), "-c:a", "aac", "-b:a", "96k", filepath.Join(out, t.id+".m4a"))...)
		os.Remove(tmp)
		if err != nil {
			return err
		}
		report[t.id] = stats
		line, _ := json.Marshal(stats)
		fmt.Println(t.id, string(line))
	}
	if statsOnly {
		return nil
	}
	videoOut, englishOut := filepath.Join(out, "video.mp4"), filepath.Join(out, "english.m4a")
	if _, err := ffmpeg(nil, "-y", "-i", video, "-an", "-c:v", "copy", "-movflags", "+faststart", videoOut); err != nil {
		return err
	}
	if _, err := ffmpeg(nil, "-y", "-i", englishAudio, "-c:a", "copy", englishOut); err != nil {
		return err
	}
	args := []string{"-y", "-i", videoOut, "-i", englishOut}
	meta := []string{"-metadata:s:a:0", "title=Гоенка, оригинал", "-metadata:s:a:0", "language=eng"}
	maps := []string{"-map", "0:v", "-map", "1:a"}
	for k, t := range tracks {
		args = append(args, "-i", filepath.Join(out, t.id+".m4a"))
		meta = append(meta, fmt.Sprintf("-metadata:s:a:%d", k+1), "title="+t.title, fmt.Sprintf("-metadata:s:a:%d", k+1), "language=rus")
		maps = append(maps, "-map", strconv.Itoa(k+2)+":a")
	}
	args = append(args, maps...)
	args = append(args, "-c", "copy")
	args = append(args, meta...)
	args = append(args, "-disposition:a:0", "default", filepath.Join(out, fmt.Sprintf("lesson%d.mkv", l.N)))
	if _, err := ffmpeg(nil, args...); err != nil {
		return err
	}
	return writeJSON(filepath.Join(out, "tracks.json"), tracksFile{list, report, englishLUFS})
}
