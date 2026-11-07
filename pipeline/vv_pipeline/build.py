"""Apply the owner's overrides and write dictionary/words.jsonl and data/examples.jsonl."""

import json
from dataclasses import asdict
from pathlib import Path

import yaml

from . import paths
from .glossary import Glossary
from .models import Example, Lemma, Translation


def load_overrides(path: Path = paths.OVERRIDES / "words.yaml") -> dict[str, dict]:
    if not path.exists():
        return {}
    return yaml.safe_load(path.read_text(encoding="utf-8")) or {}


def apply_lemma_overrides(lemmas: dict[str, Lemma], overrides: dict[str, dict]) -> dict[str, Lemma]:
    """drop and merge_into act on lemmas, before examples, translation and TTS."""
    out = dict(lemmas)
    for k, o in overrides.items():
        if k not in out:
            continue
        if o.get("drop"):
            del out[k]
        elif (target := o.get("merge_into")) and target in out and target != k:
            src, dst = out.pop(k), out[target]
            dst.forms |= src.forms
            dst.freq += src.freq
            dst.occurrences += src.occurrences
    return out


def entries(
    lemmas: dict[str, Lemma],
    translations: dict[str, Translation],
    glossary: Glossary,
    overrides: dict[str, dict],
) -> list[dict]:
    rows = []
    for k in sorted(lemmas):
        lm = lemmas[k]
        tr = translations.get(k, Translation(ru=[]))
        term = glossary.terms.get(k)
        if term is not None:  # the glossary is the only source for terms
            tr = Translation(ru=term.ru, pos=tr.pos or "NOUN", note=term.note)
        ru, reviewed = tr.ru, bool(term and term.reviewed)
        if (o := overrides.get(k)) and o.get("ru"):
            ru, reviewed = list(o["ru"]), True
        rows.append(
            {
                "key": k,
                "lemma": term.display if term else lm.lemma,
                "lang": term.lang if term else lm.lang,
                "pos": tr.pos or lm.pos,
                "forms": sorted(lm.forms),
                "freq": lm.freq,
                "ru": ru,
                "note": tr.note,
                "audio": f"{k}.mp3",
                "reviewed": reviewed,
                "is_term": lm.is_term,
            }
        )
    return rows


def write(
    rows: list[dict],
    examples: dict[str, list[Example]],
    words_path: Path = paths.WORDS,
    examples_path: Path = paths.EXAMPLES,
) -> None:
    keys = {r["key"] for r in rows}
    words_path.parent.mkdir(parents=True, exist_ok=True)
    with words_path.open("w", encoding="utf-8", newline="\n") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False, sort_keys=False) + "\n")
    examples_path.parent.mkdir(parents=True, exist_ok=True)
    with examples_path.open("w", encoding="utf-8", newline="\n") as f:
        for k in sorted(examples):
            if k in keys:
                for e in examples[k]:
                    f.write(json.dumps(asdict(e), ensure_ascii=False) + "\n")
