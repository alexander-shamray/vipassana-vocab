"""Which lesson a step works on: the dub command sets DUB_WORK and DUB_RU_MEDIA. Steps run from the working folder."""
import os

WORK = os.environ.get('DUB_WORK', '.')


def at(name):
    """Path of a per-lesson working file."""
    return os.path.join(WORK, name)


RU_MEDIA = at(os.environ.get('DUB_RU_MEDIA', 'ru-video.mp4'))
