"""Command line: python -m vv_pipeline <step> [--only 1,2]."""

import argparse
import json
import logging
from dataclasses import asdict
from pathlib import Path

from . import build, examples, extract, fetch, glossary, paths, transcribe, translate, tts, upload
from .models import Lemma, Occ

STEPS = ["fetch", "transcribe", "extract", "translate", "tts", "upload"]


def main(argv: list[str] | None = None) -> None:
    p = argparse.ArgumentParser(prog="vv_pipeline", description=__doc__)
    p.add_argument("steps", nargs="+", choices=[*STEPS, "build", "all"])
    p.add_argument("--only", help="comma-separated talk numbers, e.g. 1,2 (fetch/transcribe)")
    p.add_argument(
        "--tts",
        choices=tts.ENGINES,
        help="pronunciation engine: kokoro (local GPU) or azure (needs AZURE_SPEECH_*); "
        "default: the engine of the last build, else kokoro",
    )
    args = p.parse_args(argv)
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")

    steps = STEPS if "all" in args.steps else args.steps
    only = {int(x) for x in args.only.split(",")} if args.only else None
    gloss = glossary.load()

    if "fetch" in steps:
        fetch.run(only=only)
    if "transcribe" in steps:
        talks = [t for t in fetch.load() if not only or t.talk in only]
        transcribe.run(talks, list(gloss))
    if "extract" in steps:
        lemmas = extract.run(transcribe.load_all(), gloss, extract.spacy_analyzer())
        _save_lemmas(lemmas)
    # Translation, TTS and the build always work from the saved lemmas.
    if {"translate", "build", "tts", "upload"} & set(steps) or "all" in args.steps:
        _build(gloss, steps, resolve_engine(args.tts))


def resolve_engine(flag: str | None, marker: Path = paths.TTS_ENGINE) -> str:
    """An explicit --tts wins; otherwise keep the engine of the last build, so
    `upload` alone never renames the mp3s in words.jsonl to another engine's."""
    if flag:
        return flag
    if marker.exists() and (saved := marker.read_text(encoding="utf-8").strip()) in tts.ENGINES:
        return saved
    return tts.DEFAULT_ENGINE


def _build(gloss, steps, engine: str) -> None:
    from spacy.lang.en.stop_words import STOP_WORDS

    overrides = build.load_overrides()
    lemmas = build.apply_lemma_overrides(_load_lemmas(), overrides)
    client = None
    if "translate" in steps:
        import anthropic

        client = anthropic.Anthropic()
    translations = (
        translate.run(lemmas, gloss, client)
        if client
        else {k: v for k, v in translate._load(paths.TRANSLATIONS).items()}
    )
    rows = build.entries(lemmas, translations, gloss, overrides, engine)
    if "tts" in steps:
        tts.run(rows, {t.key: t.ipa for t in gloss}, engine)
    # words.jsonl names only recordings that exist; the rest use browser speech.
    rows = build.with_existing_audio(rows, paths.TTS)
    build.write(rows, examples.choose(lemmas, set(STOP_WORDS)))
    paths.TTS_ENGINE.parent.mkdir(parents=True, exist_ok=True)
    paths.TTS_ENGINE.write_text(engine, encoding="utf-8")
    voiced = sum(1 for r in rows if r["audio"])
    logging.info("wrote %d words (%d with %s audio) to %s", len(rows), voiced, engine, paths.WORDS)
    if "upload" in steps:
        upload.run()


def _save_lemmas(lemmas: dict[str, Lemma]) -> None:
    data = {k: {**asdict(v), "forms": sorted(v.forms)} for k, v in sorted(lemmas.items())}
    paths.LEMMAS.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")


def _load_lemmas() -> dict[str, Lemma]:
    raw = json.loads(paths.LEMMAS.read_text(encoding="utf-8"))
    out = {}
    for k, v in raw.items():
        v["forms"] = set(v["forms"])
        v["occurrences"] = [Occ(**o) for o in v["occurrences"]]
        out[k] = Lemma(**v)
    return out


if __name__ == "__main__":
    main()
