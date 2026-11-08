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


def test_quotes_use_the_glossary_spelling_of_pali():
    g = gl.index(
        [
            Term("sila", "sīla", [], "pali", ["нравственность"], False, misheard=["sheila"]),
            Term(
                "samma-vaca", "sammā-vācā", [], "pali", ["правильная речь"], False, misheard=["samma vacha"]
            ),
            Term("sensation", "sensation", ["sensations"], "en", ["ощущение"], False),
        ]
    )
    out = run(
        [transcript(seg(" Sheila is morality, samma vacha too. Observe sensations."))], g, fake_analyzer
    )
    assert out["sila"].occurrences[0].sentence == "Sīla is morality, sammā-vācā too."
    assert out["samma-vaca"].freq == 1
    # English terms keep the spoken form.
    assert out["sensation"].occurrences[0].sentence == "Observe sensations."


def test_respell_adds_diacritics_and_keeps_capitals():
    g = gl.index(
        [
            Term("samadhi", "samādhi", [], "pali", ["сосредоточение"], False),
            Term("dhamma", "Dhamma", [], "pali", ["Дхамма"], False),
            Term("samma-vaca", "sammā-vācā", [], "pali", ["правильная речь"], False),
        ]
    )
    assert g.respell("Samadhi and dhamma, samma vaca.") == "Samādhi and Dhamma, sammā-vācā."
    assert g.respell("Buddhas teach.") == "Buddhas teach."


RESPELL = gl.index(
    [
        Term("dhamma", "Dhamma", ["dharma"], "pali", ["Дхамма"], False),
        Term("sankhara", "saṅkhāra", ["samskara"], "pali", ["санкхара"], False, misheard=["sankara"]),
        Term("nama-rupa", "nāma-rūpa", [], "pali", ["ум и материя"], False),
        Term("parami", "pāramī", [], "pali", ["парами"], False, misheard=["parmi"]),
    ]
)


def test_respell_keeps_forms_said_on_purpose():
    assert (
        RESPELL.respell("In Sanskrit it is dharma, in Pali dhamma.")
        == "In Sanskrit it is dharma, in Pali Dhamma."
    )
    assert RESPELL.respell("samskara") == "samskara"


def test_respell_keeps_english_plurals():
    assert RESPELL.respell("These sankharas, those sankaras.") == "These saṅkhāras, those saṅkhāras."
    assert RESPELL.respell("The ten parmis.") == "The ten pāramīs."


def test_respell_never_swallows_a_comma():
    assert RESPELL.respell("Nama, rupa, vedana.") == "Nama, rupa, vedana."
    assert RESPELL.respell("nama-rupa and nama rupa") == "nāma-rūpa and nāma-rūpa"


def test_phrase_is_not_counted_across_a_comma():
    out = run([transcript(seg(" Nama, rupa: both change. Then nama rupa again."))], RESPELL, fake_analyzer)
    assert out["nama-rupa"].freq == 1


def test_run_together_mishearing_of_a_compound_is_respelled():
    g = gl.index([Term("u-ba-khin", "U Ba Khin", [], "pali", ["У Ба Кхин"], False, misheard=["ubakhin"])])
    assert g.respell("My teacher Ubakhin.") == "My teacher U Ba Khin."
    assert g.respell("Khin alone.") == "Khin alone."


def test_english_plural_of_a_misheard_pali_word_counts_for_the_term():
    g = gl.index([Term("sila", "sīla", [], "pali", ["нравственность"], False, misheard=["sheila"])])
    out = run([transcript(seg(" Of these five sheilas, keep each sheila."))], g, fake_analyzer)
    assert out["sila"].freq == 2
    assert "sheila" not in out


def test_sentence_split_across_segments_is_quoted_whole():
    first = seg(" Keep your sila and", start=10.0)
    second = seg(" observe the sensations.", start=14.0)
    out = run([transcript(first, second)], GLOSS, fake_analyzer)
    occ = out["observe"].occurrences[0]
    assert occ.sentence == "Keep your sila and observe the sensations."
    assert occ.t == 8  # the sentence starts in the first segment, minus the lead


def test_dropped_segment_ends_the_sentence():
    first = seg(" Keep your sila and", start=10.0)
    noise = seg(" la la la", start=14.0, avg_logprob=-1.5)
    second = seg(" observe the sensations.", start=18.0)
    out = run([transcript(first, noise, second)], GLOSS, fake_analyzer)
    assert out["observe"].occurrences[0].sentence == "observe the sensations."


def test_long_sentence_is_quoted_as_the_clauses_around_the_word():
    long = (
        " When you sit, you feel pain in the legs, you feel aches in the back,"
        " you start to observe the sensations with equanimity, without reacting,"
        " and slowly, slowly the old habit of the mind changes at the deepest level."
    )
    out = run([transcript(seg(long))], GLOSS, fake_analyzer)
    q = out["observe"].occurrences[0].sentence
    assert q.startswith("… ") and q.endswith(" …")
    assert "you start to observe the sensations with equanimity" in q
    assert len(q.split()) <= 25


def test_excerpt_of_one_endless_clause_is_centred():
    from vv_pipeline.extract import _excerpt

    words = [f"w{i}" for i in range(60)]
    assert _excerpt(words, 30) == (20, 40)
    assert _excerpt(words, 2) == (0, 20)
    assert _excerpt(words, 59) == (40, 60)


def test_excerpt_never_is_a_lone_short_clause():
    from vv_pipeline.extract import _excerpt

    words = [f"w{i}" for i in range(29)] + ["x,", "yes,"] + [f"v{i}" for i in range(30)]
    lo, hi = _excerpt(words, 30)
    assert hi - lo == 20 and lo <= 30 < hi


def test_excerpt_grows_on_the_side_with_less_context():
    from vv_pipeline.extract import _excerpt

    lo, hi = _excerpt(["a,"] * 30, 15)
    assert abs((15 - lo) - (hi - 1 - 15)) <= 1


def test_excerpt_does_not_cut_a_phrase_term():
    from vv_pipeline.extract import _excerpt

    words = [f"w{i}" for i in range(60)]
    assert _excerpt(words, 30, [(39, 42)]) == (20, 42)
    assert _excerpt(words, 30, [(18, 21)]) == (18, 40)


def test_word_said_twice_in_a_sentence_is_quoted_once():
    out = run([transcript(seg(" Observe, observe the sensations."))], GLOSS, fake_analyzer)
    assert out["observe"].freq == 2
    assert len(out["observe"].occurrences) == 1
