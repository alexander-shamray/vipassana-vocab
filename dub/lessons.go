package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// lesson ties one day's English video to the human Russian narration of the same talk.
type lesson struct {
	N     int
	En    string // YouTube id of the English video
	Ru    string // YouTube id of the Russian narration
	Title string
}

var lessons = []lesson{
	{1, "cz7QHNvNFfA", "YCkqVeQuCvM", "Урок 1: первый день"},
	{2, "cYG5VvHry7c", "_ffDuUtpfR4", "Урок 2: второй день"},
	{3, "rXXnSK2a47w", "NGcguPHTSgA", "Урок 3: третий день"},
	{4, "UvKl0Wpwbn0", "VQCfCIyVXTE", "Урок 4: четвёртый день"},
	{5, "dB0TB7tQoYY", "oqLTmVmQzuI", "Урок 5: пятый день"},
	{6, "Yxp0mZeK2zk", "ueK7FQi9HB0", "Урок 6: шестой день"},
	{7, "u4twJT1RfiM", "B6GraH0gBH4", "Урок 7: седьмой день"},
	{8, "Us5Iq302eNU", "gWBswPjUhXQ", "Урок 8: восьмой день"},
	{9, "OeCO_EQ0vN8", "KgFzfupMHUU", "Урок 9: девятый день"},
	{10, "NzrQ2HMFOuo", "XGOFC2DJCWI", "Урок 10: десятый день"},
	{11, "aeqCeU8Abng", "c4l8MZ_Fe5Y", "Урок 11: последний день"},
}

func findLesson(arg string) (lesson, error) {
	n, err := strconv.Atoi(arg)
	if err != nil {
		return lesson{}, fmt.Errorf("lesson number expected, got %q", arg)
	}
	for _, l := range lessons {
		if l.N == n {
			return l, nil
		}
	}
	return lesson{}, fmt.Errorf("no lesson %d: the playlist has 1 to %d", n, len(lessons))
}

// Work is the folder of the lesson's working files. Lesson 1 predates the numbering and sits in the top folder.
func (l lesson) Work() string {
	if l.N == 1 {
		return "."
	}
	return "work" + strconv.Itoa(l.N)
}

// At is the path of one of the lesson's working files.
func (l lesson) At(name string) string { return filepath.Join(l.Work(), name) }

// Out is the folder of the finished player.
func (l lesson) Out() string { return "lesson" + strconv.Itoa(l.N) }

// RuMediaName is the file holding the Russian narration.
func (l lesson) RuMediaName() string {
	if l.N == 1 {
		return "ru-video.mp4"
	}
	return "ru-audio.m4a"
}

var lessonDir = regexp.MustCompile(`^lesson(\d+)$`)

// builtLessons lists the numbers of the lesson folders that hold a player, in order.
func builtLessons() []int {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		m := lessonDir.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Name(), "index.html")); err != nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}
