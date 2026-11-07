"""Pali and Dhamma glossaries: hand-checked spellings, meanings and pronunciations."""

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
