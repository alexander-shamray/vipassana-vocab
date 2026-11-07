import json

from vv_pipeline import paths
from vv_pipeline.textnorm import fold, key


def test_fold_cases_shared_with_go():
    cases = json.loads((paths.ROOT / "testdata" / "fold_cases.json").read_text(encoding="utf-8"))
    for c in cases:
        assert fold(c["in"]) == c["fold"], c["in"]
        assert key(c["in"]) == c["key"], c["in"]
