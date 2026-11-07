from vv_pipeline.cli import resolve_engine


def test_engine_flag_wins(tmp_path):
    marker = tmp_path / "tts_engine"
    marker.write_text("azure", encoding="utf-8")
    assert resolve_engine("kokoro", marker) == "kokoro"


def test_engine_of_the_last_build_is_kept(tmp_path):
    marker = tmp_path / "tts_engine"
    marker.write_text("azure\n", encoding="utf-8")
    assert resolve_engine(None, marker) == "azure"


def test_default_engine_without_marker(tmp_path):
    assert resolve_engine(None, tmp_path / "missing") == "kokoro"
    (tmp_path / "bad").write_text("espeak", encoding="utf-8")
    assert resolve_engine(None, tmp_path / "bad") == "kokoro"
