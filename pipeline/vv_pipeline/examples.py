"""Pick a few short quotes per word, keeping the total quoted text small.

Copyright limit from the design: at most PER_WORD quotes of at most
MAX_WORDS words, none for stop words or the most frequent words.
Glossary terms keep their quotes whatever their frequency: they are the
words a learner most needs to hear in context.
"""

import math

from .models import Example, Lemma

PER_WORD = 3
MAX_WORDS = 25
MIN_WORDS = 3
TOP_SHARE = 0.02


def choose(lemmas: dict[str, Lemma], stop_words: set[str]) -> dict[str, list[Example]]:
    ranked = sorted(lemmas.values(), key=lambda lm: (-lm.freq, lm.key))
    # A word said once is never "very frequent", whatever the ranking says.
    top = {lm.key for lm in ranked[: math.ceil(len(ranked) * TOP_SHARE)] if lm.freq > 1}
    out: dict[str, list[Example]] = {}
    for lm in lemmas.values():
        if not lm.is_term and (lm.key in top or lm.lemma.lower() in stop_words):
            out[lm.key] = []
            continue
        out[lm.key] = _pick(lm)
    return out


def _pick(lm: Lemma) -> list[Example]:
    seen: set[str] = set()
    candidates = []
    for occ in lm.occurrences:
        n = len(occ.sentence.split())
        if MIN_WORDS <= n <= MAX_WORDS and occ.sentence not in seen:
            seen.add(occ.sentence)
            candidates.append((n, occ.talk, occ.t, occ))
    candidates.sort(key=lambda c: c[:3])

    picked, talks = [], set()
    for *_, occ in candidates:  # first pass: one per talk, shortest first
        if occ.talk not in talks:
            picked.append(occ)
            talks.add(occ.talk)
        if len(picked) == PER_WORD:
            break
    for *_, occ in candidates:  # top up from talks already used
        if len(picked) == PER_WORD:
            break
        if occ not in picked:
            picked.append(occ)
    return [Example(lm.key, o.talk, o.t, o.video_id, o.sentence) for o in picked]
