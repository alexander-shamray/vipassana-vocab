"""Plain data passed between pipeline steps."""

from dataclasses import dataclass, field
from typing import Literal

Lang = Literal["en", "pali"]


@dataclass
class Talk:
    talk: int  # 1-based position in the playlist
    video_id: str
    title: str
    audio: str  # path relative to the repository root


@dataclass
class Occ:
    """One occurrence of a word: the sentence around it and when it was said."""

    talk: int
    video_id: str
    t: int  # seconds from the start of the video
    sentence: str


@dataclass
class Lemma:
    key: str
    lemma: str
    lang: Lang
    pos: str
    forms: set[str] = field(default_factory=set)
    freq: int = 0
    occurrences: list[Occ] = field(default_factory=list)
    is_term: bool = False


@dataclass
class Term:
    """A glossary entry: a Pali word or an English word used as a Dhamma term."""

    key: str
    display: str
    variants: list[str]
    lang: Lang
    ru: list[str]
    reviewed: bool
    ipa: str | None = None
    note: str | None = None
    # Spellings Whisper wrote for this Pali word; quotes are respelled.
    # `variants` are forms said on purpose (Sanskrit "dharma") and stay.
    misheard: list[str] = field(default_factory=list)


@dataclass
class Translation:
    ru: list[str]
    pos: str = ""
    note: str | None = None


@dataclass
class Example:
    key: str
    talk: int
    t: int
    video_id: str
    text: str
