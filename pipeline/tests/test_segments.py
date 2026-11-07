from vv_pipeline.segments import keep

GOOD = {
    "text": " Observe the sensation.",
    "language": "en",
    "language_prob": 0.97,
    "avg_logprob": -0.2,
    "no_speech_prob": 0.01,
}


def test_keeps_confident_english():
    assert keep(GOOD)


def test_drops_hindi_doha():
    assert not keep({**GOOD, "language": "hi"})


def test_drops_unsure_language():
    assert not keep({**GOOD, "language_prob": 0.3})


def test_drops_low_logprob():
    assert not keep({**GOOD, "avg_logprob": -1.3})


def test_drops_silence():
    assert not keep({**GOOD, "no_speech_prob": 0.7})


def test_drops_empty_text():
    assert not keep({**GOOD, "text": "   "})
