"""Upload pronunciations (public bucket) and examples (private bucket) to Cloudflare R2."""

import logging
import os

from . import paths

log = logging.getLogger(__name__)


def run() -> None:
    import boto3  # heavy optional dependency

    s3 = boto3.client(
        "s3",
        endpoint_url=os.environ["R2_ENDPOINT"],
        aws_access_key_id=os.environ["R2_ACCESS_KEY_ID"],
        aws_secret_access_key=os.environ["R2_SECRET_ACCESS_KEY"],
        region_name="auto",
    )
    public, private = os.environ["R2_PUBLIC_BUCKET"], os.environ["R2_PRIVATE_BUCKET"]

    # Copyrighted quotes: private bucket only, fetched by the deploy job.
    s3.upload_file(
        str(paths.EXAMPLES), private, "examples.jsonl", ExtraArgs={"ContentType": "application/x-ndjson"}
    )
    log.info("examples.jsonl -> %s", private)

    n = 0
    for mp3 in sorted(paths.TTS.glob("*.mp3")):
        s3.upload_file(
            str(mp3),
            public,
            f"audio/{mp3.name}",
            ExtraArgs={"ContentType": "audio/mpeg", "CacheControl": "public, max-age=31536000"},
        )
        n += 1
    log.info("%d mp3 files -> %s/audio/", n, public)
