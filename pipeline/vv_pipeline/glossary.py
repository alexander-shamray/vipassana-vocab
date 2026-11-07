"""Pali and Dhamma glossaries: hand-checked spellings, meanings and pronunciations."""

import re
import unicodedata
from dataclasses import dataclass
from pathlib import Path

import yaml

from . import paths
from .models import Lang, Term
from .textnorm import fold, key


@dataclass
class Glossary:
    terms: dict[str, Term]  # by key
    single: dict[str, Term]  # folded one-word spelling -> term
    phrases: list[tuple[tuple[str, ...], Term]]  # folded word tuples, longest first

    def __iter__(self):
        return iter(self.terms.values())

    def respell(self, text: str) -> str:
        """Write Pali terms in a quote with their glossary spelling.

        Whisper writes what it hears ("Sheila", "parmi", "samma vacha"); the
        site quotes the talks, so a misheard Pali word is shown as "sīla".
        English (dhamma.yaml) terms are left as spoken.
        """
        for words, term in self.phrases:
            if term.lang != "pali":
                continue
            pattern = r"\b" + r"[\s,-]+".join(re.escape(w) for w in words) + r"\b"
            text = re.sub(pattern, lambda m, t=term: _cased(t.display, m.group(0)), text, flags=re.IGNORECASE)

        def one(m: re.Match) -> str:
            word = m.group(0)
            term = self.single.get(fold(word))
            if term is None or term.lang != "pali" or " " in term.display or "-" in term.display:
                return word  # a lone word of a compound stays as spoken
            return _cased(term.display, word)

        return _WORD.sub(one, text)


_WORD = re.compile(r"[^\W\d_]+")


def _cased(display: str, spoken: str) -> str:
    """Keep a capital the speaker's sentence had: "Sheila" -> "Sīla"."""
    if spoken[:1].isupper() and display[:1].islower():
        return display[:1].upper() + display[1:]
    return display


def load(directory: Path = paths.GLOSSARY) -> Glossary:
    terms: dict[str, Term] = {}
    for name, lang in (("pali.yaml", "pali"), ("dhamma.yaml", "en")):
        for raw in yaml.safe_load((directory / name).read_text(encoding="utf-8")) or []:
            t = _term(raw, lang)
            if t.key in terms:
                raise ValueError(f"{name}: duplicate glossary key {t.key!r}")
            terms[t.key] = t
    return index(list(terms.values()))


def index(terms: list[Term]) -> Glossary:
    single: dict[str, Term] = {}
    phrases: list[tuple[tuple[str, ...], Term]] = []
    for t in terms:
        for spelling in {t.display, *t.variants}:
            words = tuple(fold(spelling).replace("-", " ").split())
            if len(words) == 1:
                single[words[0]] = t
            elif words:
                phrases.append((words, t))
    phrases.sort(key=lambda p: -len(p[0]))
    return Glossary({t.key: t for t in terms}, single, phrases)


def _term(raw: dict, lang: Lang) -> Term:
    display = raw["display"]
    return Term(
        key=key(display),
        display=display,
        variants=list(raw.get("variants") or []),
        lang=lang,
        ru=list(raw.get("ru") or []),
        reviewed=bool(raw.get("reviewed", False)),
        ipa=raw.get("ipa") or (pali_ipa(display) if lang == "pali" else None),
        note=raw.get("note"),
    )


# Pali (IAST) to IPA. Aspirated pairs first so "kh" is not read as "k"+"h".
_PALI_IPA = [
    ("kh", "kʰ"),
    ("gh", "ɡʱ"),
    ("ch", "tʃʰ"),
    ("jh", "dʒʱ"),
    ("ṭh", "ʈʰ"),
    ("ḍh", "ɖʱ"),
    ("th", "tʰ"),
    ("dh", "dʱ"),
    ("ph", "pʰ"),
    ("bh", "bʱ"),
    ("ā", "aː"),
    ("ī", "iː"),
    ("ū", "uː"),
    ("a", "ɐ"),
    ("i", "ɪ"),
    ("u", "ʊ"),
    ("e", "eː"),
    ("o", "oː"),
    ("k", "k"),
    ("g", "ɡ"),
    ("ṅ", "ŋ"),
    ("c", "tʃ"),
    ("j", "dʒ"),
    ("ñ", "ɲ"),
    ("ṭ", "ʈ"),
    ("ḍ", "ɖ"),
    ("ṇ", "ɳ"),
    ("t", "t"),
    ("d", "d"),
    ("n", "n"),
    ("p", "p"),
    ("b", "b"),
    ("m", "m"),
    ("y", "j"),
    ("r", "r"),
    ("l", "l"),
    ("ḷ", "ɭ"),
    ("v", "ʋ"),
    ("s", "s"),
    ("h", "h"),
    ("ṃ", "ŋ"),
    ("ṁ", "ŋ"),
    ("-", " "),
    (" ", " "),
]


def pali_ipa(text: str) -> str:
    """Approximate IPA for a Pali word in IAST spelling, for the TTS phoneme tag."""
    s = unicodedata.normalize("NFC", text.lower())
    out: list[str] = []
    i = 0
    while i < len(s):
        for src, dst in _PALI_IPA:
            if s.startswith(src, i):
                out.append(dst)
                i += len(src)
                break
        else:
            i += 1  # unknown character: skip
    return "".join(out).strip()
