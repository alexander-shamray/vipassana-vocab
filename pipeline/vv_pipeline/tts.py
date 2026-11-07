"""Pronunciation mp3s: Kokoro on the local GPU (default) or Azure neural TTS.

Each engine has a `spec`: a string that fully determines the recording
(engine, voice, text or phonemes). The build names every mp3 after a hash
of its spec, so a corrected IPA, another voice or another engine gets a new
URL instead of a stale cached recording.
"""

import logging
import os
import time
from pathlib import Path
from xml.sax.saxutils import escape, quoteattr

from . import paths

log = logging.getLogger(__name__)

ENGINES = ("kokoro", "azure")
DEFAULT_ENGINE = "kokoro"

# Azure
VOICE_EN = "en-GB-RyanNeural"
VOICE_PALI = "en-IN-PrabhatNeural"
OUTPUT_FORMAT = "audio-24khz-48kbitrate-mono-mp3"
REQUESTS_PER_SECOND = 5

# Kokoro-82M: British voice for English; a Hindi voice reads Pali from IPA.
KOKORO_REPO = "hexgrad/Kokoro-82M"
KOKORO_VOICE_EN = "bm_george"
KOKORO_VOICE_PALI = "hm_omega"
KOKORO_RATE = 24000


def ssml(text: str, lang: str, ipa: str | None) -> str:
    voice, xml_lang = (VOICE_PALI, "en-IN") if lang == "pali" else (VOICE_EN, "en-GB")
    body = escape(text)
    if ipa:
        body = f'<phoneme alphabet="ipa" ph={quoteattr(ipa)}>{body}</phoneme>'
    return (
        f'<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="{xml_lang}">'
        f'<voice name="{voice}">{body}</voice></speak>'
    )


def kokoro_phonemes(ipa: str) -> str:
    """Our Pali IPA in Kokoro's phoneme set: it has ʰ but not breathy ʱ
    (unknown symbols are silently dropped), and wants a stress mark."""
    ps = ipa.replace("ʱ", "ʰ")
    return ps if ps.startswith(("ˈ", "ˌ")) else "ˈ" + ps


def spec(text: str, lang: str, ipa: str | None, engine: str = DEFAULT_ENGINE) -> str:
    if engine == "azure":
        return ssml(text, lang, ipa)
    if engine == "kokoro":
        if ipa:
            return f"kokoro|{KOKORO_VOICE_PALI}|ps|{kokoro_phonemes(ipa)}"
        return f"kokoro|{KOKORO_VOICE_EN}|text|{text}"
    raise ValueError(f"unknown TTS engine {engine!r}; choose one of {ENGINES}")


def run(
    rows: list[dict],
    ipa_by_key: dict[str, str | None],
    engine: str = DEFAULT_ENGINE,
    out_dir: Path = paths.TTS,
) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    todo = [r for r in rows if not (out_dir / r["audio"]).exists()]
    log.info("tts (%s): %d of %d recordings to make", engine, len(todo), len(rows))
    if not todo:
        return
    if engine == "azure":
        _run_azure(todo, ipa_by_key, out_dir)
    elif engine == "kokoro":
        _run_kokoro(todo, ipa_by_key, out_dir)
    else:
        raise ValueError(f"unknown TTS engine {engine!r}; choose one of {ENGINES}")


def _run_azure(rows: list[dict], ipa_by_key: dict[str, str | None], out_dir: Path) -> None:
    import httpx

    key, region = os.environ["AZURE_SPEECH_KEY"], os.environ["AZURE_SPEECH_REGION"]
    url = f"https://{region}.tts.speech.microsoft.com/cognitiveservices/v1"
    headers = {
        "Ocp-Apim-Subscription-Key": key,
        "Content-Type": "application/ssml+xml",
        "X-Microsoft-OutputFormat": OUTPUT_FORMAT,
        "User-Agent": "vipassana-vocab",
    }
    with httpx.Client(timeout=30) as client:
        for r in rows:
            resp = client.post(
                url,
                headers=headers,
                content=ssml(r["lemma"], r["lang"], ipa_by_key.get(r["key"])).encode("utf-8"),
            )
            if resp.status_code != 200:
                log.error("tts %s: HTTP %d %s", r["key"], resp.status_code, resp.text[:200])
                continue
            (out_dir / r["audio"]).write_bytes(resp.content)
            time.sleep(1 / REQUESTS_PER_SECOND)


def _run_kokoro(rows: list[dict], ipa_by_key: dict[str, str | None], out_dir: Path) -> None:
    import numpy as np
    import soundfile as sf
    import torch
    from kokoro import KModel, KPipeline

    device = "cuda" if torch.cuda.is_available() else "cpu"
    model = KModel(repo_id=KOKORO_REPO).to(device).eval()
    english = KPipeline(lang_code="b", repo_id=KOKORO_REPO, model=model)
    hindi = KPipeline(lang_code="h", repo_id=KOKORO_REPO, model=model)
    for n, r in enumerate(rows, start=1):
        ipa = ipa_by_key.get(r["key"])
        try:
            if ipa:
                ps = kokoro_phonemes(ipa)
                if missing := {c for c in ps if c not in model.vocab}:
                    log.warning("tts %s: Kokoro has no phoneme for %s", r["key"], "".join(sorted(missing)))
                chunks = [res.audio for res in hindi.generate_from_tokens(ps, voice=KOKORO_VOICE_PALI)]
            else:
                chunks = [res.audio for res in english(r["lemma"], voice=KOKORO_VOICE_EN)]
            audio = np.concatenate([c.cpu().numpy() for c in chunks if c is not None])
        except Exception as e:  # one bad word must not stop thousands
            log.error("tts %s: %s", r["key"], e)
            continue
        sf.write(out_dir / r["audio"], audio, KOKORO_RATE, format="MP3")
        if n % 500 == 0:
            log.info("tts: %d of %d", n, len(rows))
