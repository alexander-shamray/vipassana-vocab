"""Which transcript segments are English speech worth mining for words.

Per-segment language detection was tried and dropped: on Goenka's accent it
took plain English for Bengali, Sanskrit or Tibetan, and the old rule
dropped 8.6% of segments (talks 1-2, 2026-11-07). Sung Hindi couplets are mostly cut by the
VAD filter; what reaches the forced-English decoder comes out unsure, so
the log-probability check catches it. Text in a non-Latin script is dropped
as a last guard.
"""

MIN_AVG_LOGPROB = -1.0
# Whisper's own silence rule pairs a high no-speech probability with an
# unsure decode; a confident decode is speech even when no_speech_prob is high.
MAX_NO_SPEECH_PROB = 0.6
SILENT_MAX_LOGPROB = -0.5
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
    if seg.get("no_speech_prob", 0.0) > MAX_NO_SPEECH_PROB and logprob < SILENT_MAX_LOGPROB:
        return False
    return latin_share(text) >= MIN_LATIN_SHARE
