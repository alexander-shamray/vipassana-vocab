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


def test_three_from_different_talks_whole_sentences_first():
    twelve = "Observe the breath as it comes in and goes out naturally, just observe."
    occs = [
        occ(1, "one of your word."),  # fragment: no capital
        occ(1, twelve),
        occ(2, "The word is short."),
        occ(2, "and then the word continues without any end"),  # fragment
        occ(3, "This word appears in a sentence of about the right length."),
        occ(4, "Another word sentence that is whole and fine to show here."),
    ]
    lemmas = {"word": lemma("word", freq=10, occs=occs), **filler(100)}
    picked = choose(lemmas, set())["word"]
    assert len(picked) == 3
    assert len({e.talk for e in picked}) == 3
    assert all(e.text[0].isupper() and e.text.endswith(".") for e in picked)
    assert picked[0].text == twelve  # closest to IDEAL_WORDS


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


def test_whole_sentence_rules():
    from vv_pipeline.examples import _whole

    assert _whole("10 days of Vipassana.")
    assert _whole('He said, "Be happy."')
    assert not _whole("And then he went...")
    assert not _whole("one of your sīla.")
    assert not _whole("This goes on and on")
