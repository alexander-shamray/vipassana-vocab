package main

import (
	_ "embed"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

//go:embed player.html
var playerTemplate string

// Sound of each track when the page first opens; 0 means off. The visitor's own settings are remembered in the browser.
var trackDefaults = map[string]int{"english": 100, "narrator": 0, "silero-eugene": 100, "xtts-clone": 0}

// runPage writes the player: a video and independent audio tracks, each with its own volume and off switch.
func runPage(l lesson) error {
	var info tracksFile
	if err := readJSON(filepath.Join(l.Out(), "tracks.json"), &info); err != nil {
		return err
	}
	var rows strings.Builder
	for _, t := range info.Tracks {
		checked, volume := "", 100
		if d := trackDefaults[t.ID]; d != 0 {
			checked, volume = "checked", d
		}
		fmt.Fprintf(&rows, "<div class=track><label class=on><input type=checkbox data-on='%[1]s' %[2]s> %[3]s</label>"+
			"<input type=range min=0 max=100 value='%[4]d' data-vol='%[1]s' aria-label='Громкость: %[3]s'>"+
			"<output data-out='%[1]s'></output><audio preload=auto src='%[1]s.m4a' data-track='%[1]s'></audio></div>",
			t.ID, checked, html.EscapeString(t.Title), volume)
	}
	page := strings.NewReplacer("@ROWS@", rows.String(), "@N@", strconv.Itoa(l.N), "@TITLE@", l.Title).Replace(playerTemplate)
	path := filepath.Join(l.Out(), "index.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(path)
	fmt.Println(abs)
	return nil
}

var (
	playerURL  = regexp.MustCompile(`^/(lesson\d+)/([\w.-]*)$`)
	mediaTypes = map[string]string{".html": "text/html; charset=utf-8", ".json": "application/json", ".mp4": "video/mp4", ".m4a": "audio/mp4"}
)

// servePlayers answers only for the lesson<N>/ folders, with the Range requests that seeking in a video needs.
func servePlayers(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		var links strings.Builder
		for _, n := range builtLessons() {
			fmt.Fprintf(&links, "<li><a href='/lesson%d/'>Урок %d</a></li>", n, n)
		}
		w.Header().Set("Content-Type", mediaTypes[".html"])
		fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'><title>Уроки</title><ul style='font:20px/2 system-ui'>%s</ul>", links.String())
		return
	}
	m := playerURL.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	name := m[2]
	if name == "" {
		name = "index.html"
	}
	kind, ok := mediaTypes[filepath.Ext(name)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(m[1], name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", kind)
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// localAddress picks the address of the network interface; dialing UDP sends no packet.
func localAddress() string {
	c, err := net.Dial("udp", "10.255.255.255:1")
	if err != nil {
		return "localhost"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// runServe serves the players to this network. Anyone on it can open them while this runs: there is no password.
func runServe(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	fmt.Printf("Open on the phone: http://%s:%s/   (Ctrl+C stops it)\n", localAddress(), port)
	return http.ListenAndServe(addr, http.HandlerFunc(servePlayers))
}
