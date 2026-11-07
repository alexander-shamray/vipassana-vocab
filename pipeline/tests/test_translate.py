import json
from types import SimpleNamespace

from vv_pipeline import glossary as gl
from vv_pipeline import translate
from vv_pipeline.models import Lemma, Occ, Term


class FakeClient:
    """Answers with canned translations; can drop keys or fail on a call."""

    def __init__(self, drop=(), fail_on_call=None, wrap=False):
        self.calls = []
        self.drop, self.fail_on_call, self.wrap = set(drop), fail_on_call, wrap
        self.beta = SimpleNamespace(messages=SimpleNamespace(create=self.create))

    def create(self, **kw):
        self.calls.append(kw)
        if self.fail_on_call == len(self.calls):
            raise RuntimeError("network down")
        items = json.loads(kw["messages"][0]["content"].split("\n", 1)[1])
        out = [
            {"key": i["key"], "ru": [f"ру-{i['key']}"], "pos": i["pos"], "note": None}
            for i in items
            if i["key"] not in self.drop
        ]
        text = json.dumps({"translations": out}, ensure_ascii=False)
        if self.wrap:
            text = "Here you go:\n" + text + "\nDone."
        return SimpleNamespace(stop_reason="end_turn", content=[SimpleNamespace(type="text", text=text)])


def lemmas(*keys):
    return {
        k: Lemma(key=k, lemma=k, lang="en", pos="NOUN", occurrences=[Occ(1, "v", 0, f"a {k} here")])
        for k in keys
    }


GLOSS = gl.index([Term("sensation", "sensation", [], "en", ["ощущение"], True)])


def test_glossary_terms_never_sent(tmp_path):
    client = FakeClient()
    out = translate.run(lemmas("sensation", "chair"), GLOSS, client, tmp_path / "c.json")
    assert out["sensation"].ru == ["ощущение"]
    assert len(client.calls) == 1 and "sensation" not in client.calls[0]["messages"][0]["content"]
    assert client.calls[0]["model"] == "claude-opus-5-5"


def test_cache_means_no_second_call(tmp_path):
    cache = tmp_path / "c.json"
    translate.run(lemmas("chair", "table"), GLOSS, FakeClient(), cache)
    again = FakeClient()
    out = translate.run(lemmas("chair", "table"), GLOSS, again, cache)
    assert again.calls == [] and out["table"].ru == ["ру-table"]


def test_text_around_json_is_tolerated(tmp_path):
    out = translate.run(lemmas("chair"), GLOSS, FakeClient(wrap=True), tmp_path / "c.json")
    assert out["chair"].ru == ["ру-chair"]


def test_missing_key_retried_once_then_left_empty(tmp_path):
    client = FakeClient(drop={"ghost"})
    out = translate.run(lemmas("chair", "ghost"), GLOSS, client, tmp_path / "c.json")
    assert len(client.calls) == 2
    assert out["ghost"].ru == [] and out["chair"].ru == ["ру-chair"]


def test_failed_batch_is_retried_on_next_run(tmp_path):
    cache = tmp_path / "c.json"
    out = translate.run(lemmas("chair"), GLOSS, FakeClient(drop={"chair"}), cache)
    assert out["chair"].ru == []
    again = FakeClient()
    out = translate.run(lemmas("chair"), GLOSS, again, cache)
    assert len(again.calls) == 1 and out["chair"].ru == ["ру-chair"]


def test_crash_on_second_batch_keeps_first_in_cache(tmp_path):
    cache = tmp_path / "c.json"
    client = FakeClient(fail_on_call=2)
    translate.run(lemmas("a1", "a2", "b1", "b2"), GLOSS, client, cache, batch=2)
    saved = json.loads(cache.read_text(encoding="utf-8"))
    assert saved["a1"]["ru"] == ["ру-a1"]


def test_parse_rejects_garbage():
    assert translate.parse("no json", {"x"}) == {}
    assert translate.parse('{"translations": [{"key": "y", "ru": ["a"]}]}', {"x"}) == {}
