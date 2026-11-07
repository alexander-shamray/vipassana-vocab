"""Where the pipeline reads and writes. `data/` is gitignored and local only."""

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT / "data"
AUDIO = DATA / "audio"
TRANSCRIPTS = DATA / "transcripts"
TTS = DATA / "audio_tts"
TTS_ENGINE = DATA / "tts_engine"  # the engine the current words.jsonl names its mp3s for
TALKS = DATA / "talks.json"
LEMMAS = DATA / "lemmas.json"
TRANSLATIONS = DATA / "translations.json"
EXAMPLES = DATA / "examples.jsonl"

DICTIONARY = ROOT / "dictionary"
WORDS = DICTIONARY / "words.jsonl"

PIPELINE = ROOT / "pipeline"
GLOSSARY = PIPELINE / "glossary"
OVERRIDES = PIPELINE / "overrides"

PLAYLIST = "https://www.youtube.com/playlist?list=PLPJVlVRVmhc4Z01fD57jbzycm9I6W054x"
