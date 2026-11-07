"""Which transcript segments are English speech worth mining for words.

The talks contain sung Hindi couplets (dohas) and Pali chanting. Whisper
forced to English turns them into confident-looking nonsense, so segments
are dropped when language detection disagrees or recognition is unsure.
"""

MIN_AVG_LOGPROB = -1.0
MAX_NO_SPEECH_PROB = 0.6
MIN_EN_PROB = 0.5


def keep(seg: dict) -> bool:
    """True when seg looks like confidently recognised English speech."""
    if seg.get("language", "en") != "en":
        return False
    if seg.get("language_prob", 1.0) < MIN_EN_PROB:
        return False
    if seg.get("avg_logprob", 0.0) < MIN_AVG_LOGPROB:
        return False
    if seg.get("no_speech_prob", 0.0) > MAX_NO_SPEECH_PROB:
        return False
    return bool(seg.get("text", "").strip())
