"""Turn transcripts into lemmas: forms, frequencies and the sentences they occur in."""

import re
from bisect import bisect_right
from collections.abc import Callable, Iterable, Iterator
from dataclasses import dataclass

from . import segments
from .glossary import PHRASE_GAP, Glossary
from .models import Lemma, Occ
from .textnorm import fold, key

SKIP_POS = {"NUM", "PUNCT", "SPACE", "SYM", "X"}
VOWELS = set("aeiouy")
LEAD_SECONDS = 2  # start a link a little before the word
# Goenka speaks in long chains of clauses (median sentence: 34 words). A
# sentence too long to quote whole (examples.MAX_WORDS) is cut to the clauses
# around the word, at most this many words, marked with "…".
EXCERPT_WORDS = 20
QUOTE_LIMIT = 25
_CLAUSE_END = re.compile(r"[,;:]$")
_GAP = re.compile(PHRASE_GAP)


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
        for part in _runs(tr["segments"]):
            _segment(part, tr["talk"], tr["video_id"], glossary, analyze, lemmas)
    return {k: v for k, v in lemmas.items() if v.is_term or _keep_lemma(v)}


def _runs(segs: Iterable[dict]) -> Iterator[dict]:
    """Consecutive kept segments joined into one text.

    Whisper ends a segment wherever it likes, often mid-sentence; analysing
    each segment alone cut the quotes into fragments. A dropped segment
    (silence, chanting, noise) ends a run, so no sentence bridges it.
    """
    run: list[dict] = []
    for seg in segs:
        if segments.keep(seg):
            run.append(seg)
        elif run:
            yield _join(run)
            run = []
    if run:
        yield _join(run)


def _join(segs: list[dict]) -> dict:
    words: list[dict] = []
    for s in segs:
        for i, w in enumerate(s.get("words") or [{"word": s["text"], "start": s["start"]}]):
            # A word without a time takes its own segment's, not the run's.
            w = {**w, "start": w.get("start", s["start"])}
            if i == 0 and words and not words[-1]["word"][-1:].isspace() and not w["word"][:1].isspace():
                w["word"] = " " + w["word"]  # keep the last and first words apart
            words.append(w)
    return {"start": segs[0]["start"], "text": "".join(w["word"] for w in words), "words": words}


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
    offsets = [o for o, _ in starts]
    doc = analyze(text)
    sent_starts = [a for a, _ in doc.sents]

    def when(idx: int) -> float:
        i = bisect_right(offsets, idx) - 1  # a run is a whole talk: no linear scans
        return starts[i][1] if i >= 0 else seg["start"]

    quotes: dict[tuple[int, int], str] = {}

    def quote(a: int, b: int) -> str:
        if (a, b) not in quotes:  # respell each sentence once, not once per word
            quotes[(a, b)] = glossary.respell(" ".join(text[a:b].split()))
        return quotes[(a, b)]

    long_sents: dict[tuple[int, int], tuple[list[tuple[int, int]], list[str], list[tuple[int, int]]]] = {}

    quoted: dict[str, tuple[int, int]] = {}  # the sentence each word was last quoted from

    def span_of(idx: int) -> tuple[int, int]:
        i = bisect_right(sent_starts, idx) - 1
        return doc.sents[i] if i >= 0 and idx < doc.sents[i][1] else (0, len(text))

    def sentence_at(idx: int) -> tuple[str, float]:
        a, b = span_of(idx)
        spans = [(m.start() + a, m.end() + a) for m in re.finditer(r"\S+", text[a:b])]
        if not spans:
            return quote(a, b), seg["start"]
        whole = quote(spans[0][0], spans[-1][1])
        if len(whole.split()) <= QUOTE_LIMIT:  # counted after respelling: "ubakhin" -> "U Ba Khin"
            return whole, when(spans[0][0])
        if (a, b) not in long_sents:  # phrase terms are found once per sentence
            ws = [text[x:y] for x, y in spans]
            long_sents[(a, b)] = (spans, ws, _phrase_spans(ws, glossary))
        spans, ws, keep = long_sents[(a, b)]
        lo, hi = _excerpt(ws, bisect_right([x for x, _ in spans], idx) - 1, keep)
        q = quote(spans[lo][0], spans[hi - 1][1])
        q = ("… " if lo > 0 else "") + q.rstrip(",;:") + (" …" if hi < len(spans) else "")
        return q, when(spans[lo][0])

    def add(k: str, lemma: str, lang: str, pos: str, form: str, idx: int, is_term: bool) -> None:
        lm = lemmas.get(k)
        if lm is None:
            lm = lemmas[k] = Lemma(key=k, lemma=lemma, lang=lang, pos=pos, is_term=is_term)
        lm.freq += 1
        lm.forms.add(form)
        span = span_of(idx)
        if quoted.get(k) == span:
            return  # said twice in one sentence: one quote, not two overlapping excerpts
        quoted[k] = span
        sent, t = sentence_at(idx)
        lm.occurrences.append(
            Occ(talk=talk, video_id=video_id, t=max(0, int(t) - LEAD_SECONDS), sentence=sent)
        )

    def joined(a: Tok, b: Tok) -> bool:
        """Only spaces or hyphens between two words of a phrase, as in respell."""
        end = a.idx + a.text.find(clean(a.text)) + len(clean(a.text))
        start = b.idx + b.text.find(clean(b.text))
        return _GAP.fullmatch(text[end:start]) is not None

    # Word tokens only; phrases may span hyphens but not commas or stops.
    toks = [t for t in doc.toks if clean(t.text) and clean(t.text)[0].isalpha()]
    folded = [fold(clean(t.text)) for t in toks]
    used = [False] * len(toks)

    for words_, term in glossary.phrases:
        n = len(words_)
        for i in range(len(toks) - n + 1):
            if (
                not any(used[i : i + n])
                and tuple(folded[i : i + n]) == words_
                and all(joined(toks[j], toks[j + 1]) for j in range(i, i + n - 1))
            ):
                form = " ".join(clean(t.text) for t in toks[i : i + n]).lower()
                add(term.key, term.display, term.lang, "NOUN", form, toks[i].idx, True)
                for j in range(i, i + n):
                    used[j] = True

    for i, t in enumerate(toks):
        if used[i]:
            continue
        word = clean(t.text)
        term = glossary.single.get(folded[i])
        if term is None and folded[i].endswith("s"):  # English plural of a Pali word: "sheilas"
            plural_of = glossary.single.get(folded[i][:-1])
            term = plural_of if plural_of is not None and plural_of.lang == "pali" else None
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


def _phrase_spans(words: list[str], glossary: Glossary) -> list[tuple[int, int]]:
    """Word ranges of phrase terms ("U Ba Khin"), which an excerpt must not cut."""
    folded = [fold(clean(w)) for w in words]
    out = []
    for phrase, _ in glossary.phrases:
        n = len(phrase)
        for i in range(len(words) - n + 1):
            if tuple(folded[i : i + n]) == phrase and not any(
                _CLAUSE_END.search(w) for w in words[i : i + n - 1]
            ):
                out.append((i, i + n))
    return out


def _excerpt(words: list[str], at: int, keep: list[tuple[int, int]] = ()) -> tuple[int, int]:
    """Word range [lo, hi) of about EXCERPT_WORDS around words[at]: whole
    clauses where they make a quote of a useful size, else a window centred
    on the word. Never cuts inside a `keep` range (a phrase term)."""
    at = max(0, min(at, len(words) - 1))
    clauses, start = [], 0
    for j, w in enumerate(words):
        if _CLAUSE_END.search(w) or j == len(words) - 1:
            clauses.append((start, j + 1))
            start = j + 1
    c = next(k for k, (x, y) in enumerate(clauses) if x <= at < y)
    lo, hi = clauses[c]
    left, right = c - 1, c + 1
    while hi - lo <= EXCERPT_WORDS:  # grow by whole clauses on the side with less context
        order = (left, right) if at - lo < hi - 1 - at else (right, left)
        k = next(
            (
                k
                for k in order
                if 0 <= k < len(clauses) and max(hi, clauses[k][1]) - min(lo, clauses[k][0]) <= EXCERPT_WORDS
            ),
            None,
        )
        if k is None:
            break
        lo, hi = min(lo, clauses[k][0]), max(hi, clauses[k][1])
        left, right = (left - 1, right) if k == left else (left, right + 1)
    if hi - lo > EXCERPT_WORDS or hi - lo < EXCERPT_WORDS // 2:
        # One endless clause, or a clause too short to say anything ("… yes …").
        lo = max(0, min(at - EXCERPT_WORDS // 2, len(words) - EXCERPT_WORDS))
        hi = min(len(words), lo + EXCERPT_WORDS)
    for x, y in keep:  # widen rather than cut a phrase term in two
        if x < lo < y:
            lo = x
        if x < hi < y:
            hi = y
    return lo, hi


def _char_times(words: list[dict], fallback: float) -> list[tuple[int, float]]:
    """Character offset at which each Whisper word starts, with its time."""
    out, offset = [], 0
    for w in words:
        lead = len(w["word"]) - len(w["word"].lstrip())
        out.append((offset + lead, w.get("start", fallback)))
        offset += len(w["word"])
    return out
