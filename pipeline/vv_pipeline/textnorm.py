"""Text folding and stable word keys.

Must match app/internal/textnorm/fold.go; both are tested against
testdata/fold_cases.json.
"""

import unicodedata


def fold(s: str) -> str:
    """Lower-case, strip diacritics (paññā -> panna, ё -> е), collapse whitespace."""
    out: list[str] = []
    pending_space = False
    for ch in unicodedata.normalize("NFD", s):
        if unicodedata.category(ch) == "Mn":
            continue
        if ch.isspace():
            pending_space = bool(out)
            continue
        if pending_space:
            out.append(" ")
            pending_space = False
        out.append(ch.lower())
    return "".join(out)


def key(s: str) -> str:
    """Stable identifier of an entry: fold with spaces as hyphens."""
    return fold(s).replace(" ", "-")
