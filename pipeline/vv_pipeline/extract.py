"""Turn transcripts into lemmas: forms, frequencies and the sentences they occur in."""

from collections.abc import Callable, Iterable
from dataclasses import dataclass

from . import segments
from .glossary import Glossary
from .models import Lemma, Occ
from .textnorm import fold, key

SKIP_POS = {"NUM", "PUNCT", "SPACE", "SYM", "X"}
VOWELS = set("aeiouy")
LEAD_SECONDS = 2  # start a link a little before the word


@dataclass
class Tok:
    text: str
    lemma: str
    pos: str
    idx: int  # character offset in the analysed text


@dataclass
class Doc:
    toks: list[Tok]
    sents: list[tuple[int, int]]  # character spans


Analyzer = Callable[[str], Doc]


def spacy_analyzer(model: str = "en_core_web_sm") -> Analyzer:
    import spacy  # heavy optional dependency

    nlp = spacy.load(model, disable=["ner"])

    def analyze(text: str) -> Doc:
        d = nlp(text)
        return Doc(
            toks=[Tok(t.text, t.lemma_, t.pos_, t.idx) for t in d],
            sents=[(s.start_char, s.end_char) for s in d.sents],
        )

    return analyze


def clean(word: str) -> str:
    """Strip quotes and punctuation around a token: “Bhavatu -> Bhavatu."""
    return word.strip("\"'“”‘’«»()[]{}.,;:!?—–-…")


def plausible(k: str) -> bool:
    if len(k) < 2:
        return k in {"a", "i"}
    return any(c in VOWELS for c in k.replace("-", ""))


def run(transcripts: Iterable[dict], glossary: Glossary, analyze: Analyzer) -> dict[str, Lemma]:
    lemmas: dict[str, Lemma] = {}
    for tr in transcripts:
        for seg in tr["segments"]:
            if segments.keep(seg):
                _segment(seg, tr["talk"], tr["video_id"], glossary, analyze, lemmas)
    return {k: v for k, v in lemmas.items() if v.is_term or _keep_lemma(v)}


def _keep_lemma(lm: Lemma) -> bool:
    if lm.pos == "PROPN" and lm.freq < 2:
        return False
    return plausible(lm.key)


def _segment(
    seg: dict, talk: int, video_id: str, glossary: Glossary, analyze: Analyzer, lemmas: dict[str, Lemma]
) -> None:
    words = seg.get("words") or []
    text = "".join(w["word"] for w in words) if words else seg["text"]
    starts = _char_times(words, seg["start"])
    doc = analyze(text)

    def when(idx: int) -> float:
        best = seg["start"]
        for offset, t in starts:
            if offset > idx:
                break
            best = t
        return best

    def sentence_at(idx: int) -> tuple[str, float]:
        for a, b in doc.sents:
            if a <= idx < b:
                a += len(text[a:b]) - len(text[a:b].lstrip())  # time of the first word, not the gap
                return glossary.respell(" ".join(text[a:b].split())), when(a)
        return glossary.respell(" ".join(text.split())), seg["start"]

    def add(k: str, lemma: str, lang: str, pos: str, form: str, idx: int, is_term: bool) -> None:
        lm = lemmas.get(k)
        if lm is None:
            lm = lemmas[k] = Lemma(key=k, lemma=lemma, lang=lang, pos=pos, is_term=is_term)
        lm.freq += 1
        lm.forms.add(form)
        sent, t = sentence_at(idx)
        lm.occurrences.append(
            Occ(talk=talk, video_id=video_id, t=max(0, int(t) - LEAD_SECONDS), sentence=sent)
        )

    # Word tokens only, so phrases match across hyphens and stray punctuation.
    toks = [t for t in doc.toks if clean(t.text) and clean(t.text)[0].isalpha()]
    folded = [fold(clean(t.text)) for t in toks]
    used = [False] * len(toks)

    for words_, term in glossary.phrases:
        n = len(words_)
        for i in range(len(toks) - n + 1):
            if not any(used[i : i + n]) and tuple(folded[i : i + n]) == words_:
                form = " ".join(clean(t.text) for t in toks[i : i + n]).lower()
                add(term.key, term.display, term.lang, "NOUN", form, toks[i].idx, True)
                for j in range(i, i + n):
                    used[j] = True

    for i, t in enumerate(toks):
        if used[i]:
            continue
        word = clean(t.text)
        term = glossary.single.get(folded[i])
        if term is not None:
            add(term.key, term.display, term.lang, "NOUN", word.lower(), t.idx, True)
            continue
        if t.pos in SKIP_POS or not word.replace("'", "").isalpha():
            continue
        base = clean(t.lemma) or word
        if t.pos == "PROPN":
            lemma, form = base, word
        else:
            lemma, form = base.lower(), word.lower()
        k = key(lemma)
        if not k:
            continue
        # A form of a glossary term (e.g. lemma "sensation" from "sensations").
        term = glossary.terms.get(k)
        if term is not None:
            add(term.key, term.display, term.lang, "NOUN", form, t.idx, True)
            continue
        add(k, lemma, "en", t.pos, form, t.idx, False)


def _char_times(words: list[dict], fallback: float) -> list[tuple[int, float]]:
    """Character offset at which each Whisper word starts, with its time."""
    out, offset = [], 0
    for w in words:
        lead = len(w["word"]) - len(w["word"].lstrip())
        out.append((offset + lead, w.get("start", fallback)))
        offset += len(w["word"])
    return out
