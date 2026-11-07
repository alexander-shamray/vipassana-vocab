"""Pronunciation mp3s from Azure neural TTS (REST, SSML)."""

import logging
import os
import time
from pathlib import Path
from xml.sax.saxutils import escape, quoteattr

from . import paths

log = logging.getLogger(__name__)

VOICE_EN = "en-GB-RyanNeural"
VOICE_PALI = "en-IN-PrabhatNeural"
OUTPUT_FORMAT = "audio-24khz-48kbitrate-mono-mp3"
REQUESTS_PER_SECOND = 5


def ssml(text: str, lang: str, ipa: str | None) -> str:
    voice, xml_lang = (VOICE_PALI, "en-IN") if lang == "pali" else (VOICE_EN, "en-GB")
    body = escape(text)
    if ipa:
        body = f'<phoneme alphabet="ipa" ph={quoteattr(ipa)}>{body}</phoneme>'
    return (
        f'<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="{xml_lang}">'
        f'<voice name="{voice}">{body}</voice></speak>'
    )


def run(rows: list[dict], ipa_by_key: dict[str, str | None], out_dir: Path = paths.TTS) -> None:
    import httpx

    key, region = os.environ["AZURE_SPEECH_KEY"], os.environ["AZURE_SPEECH_REGION"]
    url = f"https://{region}.tts.speech.microsoft.com/cognitiveservices/v1"
    headers = {
        "Ocp-Apim-Subscription-Key": key,
        "Content-Type": "application/ssml+xml",
        "X-Microsoft-OutputFormat": OUTPUT_FORMAT,
        "User-Agent": "vipassana-vocab",
    }
    out_dir.mkdir(parents=True, exist_ok=True)
    with httpx.Client(timeout=30) as client:
        for r in rows:
            target = out_dir / r["audio"]
            if target.exists():
                continue
            resp = client.post(
                url,
                headers=headers,
                content=ssml(r["lemma"], r["lang"], ipa_by_key.get(r["key"])).encode("utf-8"),
            )
            if resp.status_code != 200:
                log.error("tts %s: HTTP %d %s", r["key"], resp.status_code, resp.text[:200])
                continue
            target.write_bytes(resp.content)
            time.sleep(1 / REQUESTS_PER_SECOND)
