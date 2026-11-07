"""Translate lemmas into Russian with Claude, in cached batches.

Glossary terms are never sent to the model: their meanings come from the
owner-reviewed glossaries.
"""

import json
import logging
from pathlib import Path

from . import paths
from .glossary import Glossary
from .models import Lemma, Translation

log = logging.getLogger(__name__)

MODEL = "claude-opus-5-5"
BATCH = 40
CONTEXTS = 2

SYSTEM = (
    "You translate English words from S. N. Goenka's 10-day Vipassana meditation discourses into "
    "Russian for a Russian-speaking student. For each word you get its part of speech and sentences "
    "from the talks. Give 1-3 short Russian meanings, the first being the sense used in the talks; "
    "use the Russian base form (infinitive for verbs, nominative singular for nouns). Add a short "
    "Russian note only when the word is used in a special sense in the talks; otherwise null."
)

SCHEMA = {
    "type": "object",
    "properties": {
        "translations": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "key": {"type": "string"},
                    "ru": {"type": "array", "items": {"type": "string"}},
                    "pos": {"type": "string"},
                    "note": {"type": ["string", "null"]},
                },
                "required": ["key", "ru", "pos", "note"],
                "additionalProperties": False,
            },
        }
    },
    "required": ["translations"],
    "additionalProperties": False,
}


def run(
    lemmas: dict[str, Lemma],
    glossary: Glossary,
    client,
    cache_path: Path = paths.TRANSLATIONS,
    batch: int = BATCH,
) -> dict[str, Translation]:
    cache = _load(cache_path)
    out: dict[str, Translation] = {}
    todo: list[Lemma] = []
    for k, lm in sorted(lemmas.items()):
        if (term := glossary.terms.get(k)) is not None:
            out[k] = Translation(ru=term.ru, pos="NOUN", note=term.note)
        elif k in cache:
            out[k] = cache[k]
        else:
            todo.append(lm)

    for i in range(0, len(todo), batch):
        chunk = todo[i : i + batch]
        got = _ask(client, chunk)
        missing = [lm for lm in chunk if lm.key not in got]
        if missing:
            log.warning("retrying %d words missing from the reply", len(missing))
            got.update(_ask(client, missing))
        for lm in chunk:
            if lm.key not in got:
                log.error("no translation for %r; left empty", lm.key)
                got[lm.key] = Translation(ru=[], pos=lm.pos)
            cache[lm.key] = out[lm.key] = got[lm.key]
        _save(cache_path, cache)  # after every batch, so a crash loses at most one
        log.info("translated %d/%d", min(i + batch, len(todo)), len(todo))
    return out


def prompt(chunk: list[Lemma]) -> str:
    items = []
    for lm in chunk:
        contexts = sorted({o.sentence for o in lm.occurrences}, key=len)[:CONTEXTS]
        items.append({"key": lm.key, "word": lm.lemma, "pos": lm.pos, "contexts": contexts})
    return "Translate these words:\n" + json.dumps(items, ensure_ascii=False, indent=1)


def _ask(client, chunk: list[Lemma]) -> dict[str, Translation]:
    try:
        resp = client.beta.messages.create(
            model=MODEL,
            max_tokens=16000,
            system=SYSTEM,
            messages=[{"role": "user", "content": prompt(chunk)}],
            output_config={"effort": "low", "format": {"type": "json_schema", "schema": SCHEMA}},
            betas=["server-side-fallback-2026-07-01"],
            fallbacks="default",
        )
    except Exception as e:  # noqa: BLE001 - one failed batch must not stop the run
        log.error("batch failed: %s", e)
        return {}
    if getattr(resp, "stop_reason", None) == "refusal":
        log.error("batch refused")
        return {}
    text = next((b.text for b in resp.content if getattr(b, "type", "") == "text"), "")
    return parse(text, {lm.key for lm in chunk})


def parse(text: str, wanted: set[str]) -> dict[str, Translation]:
    """Reads the JSON reply, tolerating stray text around the object."""
    a, b = text.find("{"), text.rfind("}")
    if a < 0 or b < a:
        return {}
    try:
        data = json.loads(text[a : b + 1])
    except json.JSONDecodeError:
        return {}
    out = {}
    for item in data.get("translations", []):
        k = item.get("key")
        ru = [r.strip() for r in item.get("ru") or [] if isinstance(r, str) and r.strip()]
        if k in wanted and ru:
            out[k] = Translation(ru=ru[:3], pos=item.get("pos") or "", note=item.get("note"))
    return out


def _load(path: Path) -> dict[str, Translation]:
    if not path.exists():
        return {}
    raw = json.loads(path.read_text(encoding="utf-8"))
    return {k: Translation(**v) for k, v in raw.items()}


def _save(path: Path, cache: dict[str, Translation]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".tmp")
    tmp.write_text(
        json.dumps({k: vars(v) for k, v in sorted(cache.items())}, ensure_ascii=False, indent=1),
        encoding="utf-8",
    )
    tmp.replace(path)
