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


def test_keeps_english_whatever_the_detected_language():
    # Real misdetections from talks 1-2: English sentences labelled bn/sa/bo.
    for lang in ("bn", "sa", "bo", "hi"):
        assert keep({**GOOD, "language": lang, "language_prob": 0.9})


def test_keeps_confident_speech_with_high_no_speech_prob():
    # Talk 1, 21:45: "don't mix up with any belief or dogma…" no_speech 0.69.
    assert keep({**GOOD, "no_speech_prob": 0.69, "avg_logprob": -0.15})


def test_drops_unsure_silence():
    assert not keep({**GOOD, "no_speech_prob": 0.7, "avg_logprob": -0.7})


def test_drops_low_logprob():
    assert not keep({**GOOD, "avg_logprob": -1.3})


def test_keeps_pali_diacritics():
    assert keep({**GOOD, "text": " Bhavatu sabba maṅgalaṃ, saṅkhāra."})


def test_drops_devanagari():
    assert not keep({**GOOD, "text": " धरम न जाने कोई"})


def test_drops_empty_text():
    assert not keep({**GOOD, "text": "   "})


def test_drops_confident_hallucination_in_silence():
    assert not keep({**GOOD, "text": " Thank you.", "avg_logprob": -0.3, "no_speech_prob": 0.95})
