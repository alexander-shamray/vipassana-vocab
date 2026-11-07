"""Download the audio of every talk in the playlist (yt-dlp)."""

import json
import logging
from dataclasses import asdict

from . import paths
from .models import Talk

log = logging.getLogger(__name__)


def run(playlist_url: str = paths.PLAYLIST, only: set[int] | None = None) -> list[Talk]:
    from yt_dlp import YoutubeDL  # heavy optional dependency

    paths.AUDIO.mkdir(parents=True, exist_ok=True)
    with YoutubeDL({"extract_flat": "in_playlist", "quiet": True}) as ydl:
        info = ydl.extract_info(playlist_url, download=False)

    talks: list[Talk] = []
    for n, entry in enumerate(info.get("entries") or [], start=1):
        if only and n not in only:
            continue
        vid = entry["id"]
        target = paths.AUDIO / f"{vid}.m4a"
        if not target.exists():
            log.info("downloading talk %d: %s", n, entry.get("title"))
            opts = {
                "format": "bestaudio[ext=m4a]/bestaudio",
                "outtmpl": str(paths.AUDIO / "%(id)s.%(ext)s"),
                "quiet": True,
            }
            with YoutubeDL(opts) as ydl:
                ydl.download([f"https://www.youtube.com/watch?v={vid}"])
        audio = next(paths.AUDIO.glob(f"{vid}.*"))
        talks.append(
            Talk(talk=n, video_id=vid, title=entry.get("title", ""), audio=str(audio.relative_to(paths.ROOT)))
        )

    paths.TALKS.write_text(
        json.dumps([asdict(t) for t in talks], ensure_ascii=False, indent=2), encoding="utf-8"
    )
    log.info("%d talks listed in %s", len(talks), paths.TALKS)
    return talks


def load() -> list[Talk]:
    return [Talk(**t) for t in json.loads(paths.TALKS.read_text(encoding="utf-8"))]
