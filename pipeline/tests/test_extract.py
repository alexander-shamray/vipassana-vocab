import re

from vv_pipeline import glossary as gl
from vv_pipeline.extract import Doc, Tok, plausible, run
from vv_pipeline.models import Term

LEMMAS = {
    "suffering": "suffer",
    "suffered": "suffer",
    "suffers": "suffer",
    "sensations": "sensation",
    "observing": "observe",
    "do": "do",
    "n't": "not",
}
PROPN = {"Goenka", "Bhavatu"}


def fake_analyzer(text: str) -> Doc:
    toks = []
    for m in re.finditer(r"n't|[\w’'-]+|[^\w\s]", text):
        w = m.group()
        pos = "PROPN" if w in PROPN else ("PUNCT" if not w[0].isalnum() else "NOUN")
        if w.isdigit():
            pos = "NUM"
        toks.append(Tok(w, LEMMAS.get(w.lower(), w), pos, m.start()))
    sents, start = [], 0
    for m in re.finditer(r"[.?!]", text):
        sents.append((start, m.end()))
        start = m.end()
    if start < len(text):
        sents.append((start, len(text)))
    return Doc(toks, sents)


def words(text: str, start: float = 0.0, step: float = 1.0) -> list[dict]:
    out, t = [], start
    for w in re.findall(r"\s*\S+", text):
        out.append({"word": w, "start": t, "end": t + step, "probability": 0.9})
        t += step
    return out


def seg(text: str, start: float = 10.0, **kw) -> dict:
    base = {
        "start": start,
        "end": start + 30,
        "text": text,
        "avg_logprob": -0.2,
        "no_speech_prob": 0.01,
        "language": "en",
        "language_prob": 0.99,
        "words": words(text, start),
    }
    return {**base, **kw}


GLOSS = gl.index(
    [
        Term("anicca", "anicca", ["anichcha"], "pali", ["непостоянство"], False),
        Term("mind-and-matter", "mind and matter", [], "en", ["ум и материя"], False),
        Term("sensation", "sensation", ["sensations"], "en", ["ощущение"], False),
    ]
)


def transcript(*segments, talk=1, vid="vid01"):
    return {"talk": talk, "video_id": vid, "segments": list(segments)}


def test_pali_variants_merge_into_one_lemma():
    out = run([transcript(seg(" Anichcha, anicca: everything changes."))], GLOSS, fake_analyzer)
    assert out["anicca"].freq == 2
    assert out["anicca"].lang == "pali" and out["anicca"].is_term
    assert "anichcha" not in out


def test_forms_reduce_to_lemma():
    out = run([transcript(seg(" You suffered. He suffers. Suffering ends."))], GLOSS, fake_analyzer)
    assert out["suffer"].forms == {"suffered", "suffers", "suffering"}
    assert out["suffer"].freq == 3


def test_multiword_term():
    out = run([transcript(seg(" Observe mind and matter now."))], GLOSS, fake_analyzer)
    assert out["mind-and-matter"].freq == 1
    assert "matter" not in out


def test_term_reached_through_lemma():
    out = run([transcript(seg(" Feel the sensations."))], GLOSS, fake_analyzer)
    assert out["sensation"].is_term and "sensations" in out["sensation"].forms


def test_punctuation_and_quotes_are_cleaned():
    out = run([transcript(seg(" “Bhavatu — don't stop 42 times."))], GLOSS, fake_analyzer)
    assert all(k and k.isascii() for k in out)
    assert "42" not in out and "—" not in out
    assert "bhavatu" not in out  # proper noun seen once


def test_unsure_segment_is_ignored():
    # Sung Hindi forced through the English decoder comes out unsure.
    out = run([transcript(seg(" Jaise jaise samjhega", avg_logprob=-1.2))], GLOSS, fake_analyzer)
    assert out == {}


def test_occurrence_time_and_sentence():
    out = run(
        [transcript(seg(" First sentence here. Then anicca again.", start=100.0))], GLOSS, fake_analyzer
    )
    occ = out["anicca"].occurrences[0]
    assert occ.sentence == "Then anicca again."
    assert occ.t == 103 - 2  # "Then" is the 4th word, one second each, minus the lead
    assert occ.video_id == "vid01" and occ.talk == 1


def test_plausible():
    assert plausible("a") and plausible("mind")
    assert not plausible("x") and not plausible("hmm") and not plausible("")


def test_pali_ipa():
    assert gl.pali_ipa("paññā") == "pɐɲɲaː"
    assert gl.pali_ipa("dukkha") == "dʊkkʰɐ"
