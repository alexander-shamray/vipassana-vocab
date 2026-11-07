"""Transcribe each talk with word timestamps (faster-whisper on the local GPU)."""

import importlib.util
import json
import logging
import os
from pathlib import Path

from . import paths
from .models import Talk, Term

log = logging.getLogger(__name__)
SAMPLE_RATE = 16000


def initial_prompt(terms: list[Term]) -> str:
    """Primes Whisper with the spellings of Pali and Dhamma terms."""
    words = sorted({v for t in terms for v in [t.display, *t.variants] if v.isascii()})
    return "Vipassana meditation discourse by S. N. Goenka. Terms: " + ", ".join(words) + "."


def use_pip_cuda() -> None:
    """Put the CUDA DLLs of the nvidia-* pip wheels (cuBLAS, cuDNN) on PATH.

    On Windows CTranslate2 finds them only through PATH; without this the
    first GPU call fails unless CUDA 12 and cuDNN 9 are installed system-wide.
    """
    spec = importlib.util.find_spec("nvidia")
    if os.name != "nt" or spec is None or not spec.submodule_search_locations:
        return
    for base in spec.submodule_search_locations:
        for bin_dir in sorted(Path(base).glob("*/bin")):
            os.environ["PATH"] = str(bin_dir) + os.pathsep + os.environ.get("PATH", "")


def run(talks: list[Talk], terms: list[Term], model_name: str = "large-v3") -> None:
    use_pip_cuda()
    from faster_whisper import WhisperModel, decode_audio  # heavy optional dependency

    paths.TRANSCRIPTS.mkdir(parents=True, exist_ok=True)
    model = WhisperModel(model_name, device="cuda", compute_type="float16")
    prompt = initial_prompt(terms)
    for talk in talks:
        out = paths.TRANSCRIPTS / f"{talk.video_id}.json"
        if out.exists():
            continue
        log.info("transcribing talk %d (%s)", talk.talk, talk.video_id)
        audio = decode_audio(str(paths.ROOT / talk.audio), sampling_rate=SAMPLE_RATE)
        segments, _ = model.transcribe(
            audio,
            language="en",
            word_timestamps=True,
            vad_filter=True,
            initial_prompt=prompt,
            condition_on_previous_text=False,
        )
        result = []
        for s in segments:
            result.append(
                {
                    "start": s.start,
                    "end": s.end,
                    "text": s.text,
                    "avg_logprob": s.avg_logprob,
                    "no_speech_prob": s.no_speech_prob,
                    "words": [
                        {"word": w.word, "start": w.start, "end": w.end, "probability": w.probability}
                        for w in (s.words or [])
                    ],
                }
            )
        out.write_text(
            json.dumps(
                {"talk": talk.talk, "video_id": talk.video_id, "segments": result}, ensure_ascii=False
            ),
            encoding="utf-8",
        )


def load_all() -> list[dict]:
    return [json.loads(p.read_text(encoding="utf-8")) for p in sorted(paths.TRANSCRIPTS.glob("*.json"))]
