import json

from vv_pipeline import build
from vv_pipeline import glossary as gl
from vv_pipeline.models import Example, Lemma, Occ, Term, Translation
from vv_pipeline.tts import ssml

GLOSS = gl.index([Term("panna", "paññā", ["pannya"], "pali", ["мудрость"], False, ipa="pɐɲɲaː")])


def lemmas():
    return {
        "panna": Lemma("panna", "paññā", "pali", "NOUN", {"panna"}, 3, [Occ(1, "v1", 5, "x panna y")], True),
        "go": Lemma("go", "go", "en", "VERB", {"go", "went"}, 10, [Occ(1, "v1", 9, "we go on")]),
        "gonna": Lemma("gonna", "gonna", "en", "VERB", {"gonna"}, 2, [Occ(2, "v2", 1, "gonna go")]),
        "junk": Lemma("junk", "junk", "en", "NOUN", {"junk"}, 1, []),
    }


OVERRIDES = {"junk": {"drop": True}, "gonna": {"merge_into": "go"}, "go": {"ru": ["идти", "ехать"]}}


def test_overrides_drop_merge_and_translate():
    lm = build.apply_lemma_overrides(lemmas(), OVERRIDES)
    assert "junk" not in lm and "gonna" not in lm
    assert lm["go"].freq == 12 and lm["go"].forms == {"go", "went", "gonna"}
    rows = build.entries(lm, {"go": Translation(["идти"], "VERB")}, GLOSS, OVERRIDES)
    go = next(r for r in rows if r["key"] == "go")
    assert go["ru"] == ["идти", "ехать"] and go["reviewed"] is True


def test_pali_lemma_keeps_diacritics_key_does_not():
    rows = build.entries(lemmas(), {"panna": Translation(["мудрость"])}, GLOSS, {})
    p = next(r for r in rows if r["key"] == "panna")
    assert p["lemma"] == "paññā" and p["lang"] == "pali" and p["is_term"] and not p["reviewed"]


def test_aliases_and_audio_name_follow_the_glossary():
    rows = build.entries(lemmas(), {}, GLOSS, {})
    p = next(r for r in rows if r["key"] == "panna")
    assert p["aliases"] == ["pannya"]  # "panna" was heard, so it is a form, not an alias
    fixed = gl.index([Term("panna", "paññā", ["pannya"], "pali", ["мудрость"], False, ipa="pɐnnaː")])
    p2 = next(r for r in build.entries(lemmas(), {}, fixed, {}) if r["key"] == "panna")
    assert p["audio"].startswith("panna-") and p2["audio"] != p["audio"]


def test_output_is_sorted_and_stable(tmp_path):
    lm = build.apply_lemma_overrides(lemmas(), OVERRIDES)
    rows = build.entries(lm, {}, GLOSS, OVERRIDES)
    exs = {"go": [Example("go", 1, 9, "v1", "we go on")], "junk": [Example("junk", 1, 1, "v", "junk x y")]}
    outs = []
    for i in range(2):
        w, e = tmp_path / f"w{i}.jsonl", tmp_path / f"e{i}.jsonl"
        build.write(rows, exs, w, e)
        outs.append((w.read_bytes(), e.read_bytes()))
    assert outs[0] == outs[1]
    keys = [json.loads(line)["key"] for line in outs[0][0].decode().splitlines()]
    assert keys == sorted(keys)
    assert b"junk" not in outs[0][1]  # examples of dropped words are not written
    assert "мудрость".encode() in outs[0][0]  # ensure_ascii=False


def test_ssml_voices_ipa_and_escaping():
    en = ssml("suffer & <go>", "en", None)
    assert "en-GB-RyanNeural" in en and "suffer &amp; &lt;go&gt;" in en
    pali = ssml("paññā", "pali", "pɐɲɲaː")
    assert "en-IN-PrabhatNeural" in pali and '<phoneme alphabet="ipa" ph="pɐɲɲaː">paññā</phoneme>' in pali
