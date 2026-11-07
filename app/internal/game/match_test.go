package game

import (
	"testing"

	"github.com/alexander-shamray/vipassana-vocab/app/internal/dict"
)

var (
	sensation = dict.Entry{Key: "sensation", Lemma: "sensation", Forms: []string{"sensations"},
		RU: []string{"ощущение (телесное, ведана)"}}
	sankhara = dict.Entry{Key: "sankhara", Lemma: "saṅkhāra", Forms: []string{"sankhara"}, Lang: "pali",
		RU: []string{"санкхара — обусловленная реакция ума"}}
	craving = dict.Entry{Key: "craving", Lemma: "craving", RU: []string{"страстное желание", "жажда"}}
	dhamma  = dict.Entry{Key: "dhamma", Lemma: "Dhamma", Aliases: []string{"dharma"},
		RU: []string{"Дхамма — закон природы", "учение Будды"}}
	mindMatter = dict.Entry{Key: "mind-and-matter", Lemma: "mind and matter", RU: []string{"ум и материя"}}
)

func TestCheckRussian(t *testing.T) {
	cases := []struct {
		e      dict.Entry
		answer string
		want   bool
	}{
		{sensation, "ощущение", true},
		{sensation, "  Ощущение ", true},
		{sensation, "ощушение", true}, // one typo in a long word
		{sensation, "ведана", false},  // only in brackets
		{sankhara, "обусловленная реакция ума", true},
		{sankhara, "реакция", true},
		{sankhara, "санкхара", true},
		{craving, "желание", true},
		{craving, "жажда", true},
		{craving, "жажа", true},   // five letters: one typo allowed
		{mindMatter, "уи", false}, // short: no typo budget
		{craving, "это жажда", true},
		{dhamma, "закон природы", true},
		{mindMatter, "ум", false},
		{mindMatter, "ум и материя", true},
		{sensation, "", false},
		{sensation, "чувство", false},
	}
	for _, c := range cases {
		if got := CheckRussian(c.e, c.answer); got != c.want {
			t.Errorf("CheckRussian(%s, %q) = %v, want %v", c.e.Key, c.answer, got, c.want)
		}
	}
}

func TestCheckRussian_AnyAlternative(t *testing.T) {
	if !CheckRussian(craving, "важная", "жажда") {
		t.Error("the second recognition alternative should count")
	}
}

func TestCheckEnglish(t *testing.T) {
	cases := []struct {
		e      dict.Entry
		answer string
		want   bool
	}{
		{sensation, "sensation", true},
		{sensation, "Sensations", true},
		{sensation, "the sensation", true},
		{sensation, "sensasion", true},
		{sankhara, "sankhara", true},
		{dhamma, "dharma", true},
		{mindMatter, "Mind and matter", true},
		{mindMatter, "mind", false},
		{craving, "cravin", true},   // one letter dropped by the recogniser
		{craving, "carving", false}, // a different word
		{craving, "crave", false},
	}
	for _, c := range cases {
		if got := CheckEnglish(c.e, c.answer); got != c.want {
			t.Errorf("CheckEnglish(%s, %q) = %v, want %v", c.e.Key, c.answer, got, c.want)
		}
	}
}

func TestRussianPrompt(t *testing.T) {
	for e, want := range map[*dict.Entry]string{
		&sensation:  "ощущение",
		&sankhara:   "обусловленная реакция ума",
		&craving:    "страстное желание",
		&mindMatter: "ум и материя",
	} {
		if got := RussianPrompt(*e); got != want {
			t.Errorf("RussianPrompt(%s) = %q, want %q", e.Key, got, want)
		}
	}
	if RussianPrompt(dict.Entry{}) != "" {
		t.Error("no meanings → empty prompt")
	}
}
