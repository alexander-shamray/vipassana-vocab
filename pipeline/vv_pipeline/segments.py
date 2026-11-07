"""Which transcript segments are English speech worth mining for words.

Per-segment language detection was tried and dropped: on Goenka's accent it
took plain English for Bengali, Sanskrit or Tibetan, and the old rule
dropped 8.6% of segments (talks 1-2, 2026-11-07). Sung Hindi couplets are mostly cut by the
VAD filter; what reaches the forced-English decoder comes out unsure, so
the log-probability check catches it. Text in a non-Latin script is dropped
as a last guard.
"""

MIN_AVG_LOGPROB = -1.0
# faster-whisper already drops no_speech_prob > 0.6 with avg_logprob < -1.0
# while decoding. keep() widens that band to -0.5 but leaves a confident
# decode alone (talk 1 had clear English at no_speech_prob 0.69), except
# near-certain silence, where a confident "Thank you." is a hallucination.
MAX_NO_SPEECH_PROB = 0.6
SILENT_MAX_LOGPROB = -0.5
SURE_SILENCE = 0.9
MIN_LATIN_SHARE = 0.7


def latin_share(text: str) -> float:
    letters = [c for c in text if c.isalpha()]
    if not letters:
        return 0.0
    return sum(1 for c in letters if c.isascii() or "À" <= c <= "ɏ" or "Ḁ" <= c <= "ỿ") / len(letters)


def keep(seg: dict) -> bool:
    """True when seg looks like confidently recognised English speech."""
    text = seg.get("text", "").strip()
    if not text:
        return False
    logprob = seg.get("avg_logprob", 0.0)
    if logprob < MIN_AVG_LOGPROB:
        return False
    no_speech = seg.get("no_speech_prob", 0.0)
    if no_speech > SURE_SILENCE or (no_speech > MAX_NO_SPEECH_PROB and logprob < SILENT_MAX_LOGPROB):
        return False
    return latin_share(text) >= MIN_LATIN_SHARE
