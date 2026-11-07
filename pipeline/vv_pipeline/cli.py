"""Command line: python -m vv_pipeline <step> [--only 1,2]."""

import argparse
import json
import logging
from dataclasses import asdict

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
        default=tts.DEFAULT_ENGINE,
        help="pronunciation engine: kokoro (local GPU, default) or azure (needs AZURE_SPEECH_*)",
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
        _build(gloss, steps, args.tts)


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
    build.write(rows, examples.choose(lemmas, set(STOP_WORDS)))
    logging.info("wrote %d words to %s", len(rows), paths.WORDS)
    if "tts" in steps:
        tts.run(rows, {t.key: t.ipa for t in gloss}, engine)
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
