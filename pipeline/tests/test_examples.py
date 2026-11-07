from vv_pipeline.examples import choose
from vv_pipeline.models import Lemma, Occ


def lemma(key, freq=1, occs=(), is_term=False):
    return Lemma(
        key=key, lemma=key, lang="en", pos="NOUN", freq=freq, occurrences=list(occs), is_term=is_term
    )


def occ(talk, text, t=0):
    return Occ(talk=talk, video_id=f"v{talk}", t=t, sentence=text)


def filler(n, freq=50):
    """Other words, more frequent than the one under test, so it is not in the top 2%."""
    return {f"w{i}": lemma(f"w{i}", freq=freq) for i in range(n)}


def test_three_from_different_talks_shortest_first():
    occs = [
        occ(1, "a b c d e f"),
        occ(1, "a b c"),
        occ(2, "a b c d"),
        occ(3, "a b c d e"),
        occ(4, "a b c d e f g"),
        occ(2, "x y z"),
        occ(3, "q r s t u v w"),
        occ(4, "m n o"),
        occ(1, "long " * 10),
        occ(2, "p q r s"),
    ]
    lemmas = {"word": lemma("word", freq=10, occs=occs), **filler(100)}
    picked = choose(lemmas, set())["word"]
    assert len(picked) == 3
    assert len({e.talk for e in picked}) == 3
    assert [len(e.text.split()) for e in picked] == [3, 3, 3]


def test_long_sentence_never_chosen():
    long = " ".join(["w"] * 26)
    lemmas = {"word": lemma("word", occs=[occ(1, long)]), **filler(100)}
    assert choose(lemmas, set())["word"] == []


def test_stop_word_gets_none():
    lemmas = {"the": lemma("the", occs=[occ(1, "the cat sat")]), **filler(100)}
    assert choose(lemmas, {"the"})["the"] == []


def test_top_two_percent_get_none_but_terms_keep_theirs():
    lemmas = {
        **filler(98, freq=5),
        "mind": lemma("mind", freq=900, occs=[occ(1, "the mind is restless")]),
        "sensation": lemma("sensation", freq=800, occs=[occ(1, "feel the sensation now")], is_term=True),
    }
    out = choose(lemmas, set())
    assert out["mind"] == []
    assert len(out["sensation"]) == 1


def test_single_occurrence():
    lemmas = {"rare": lemma("rare", occs=[occ(5, "a rare word here", t=77)]), **filler(100)}
    [e] = choose(lemmas, set())["rare"]
    assert (e.talk, e.t, e.video_id) == (5, 77, "v5")
