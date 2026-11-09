"""XTTS with the speaker's voice reads the whole lesson.
Safe to stop and start again: finished phrases are kept."""
import json, os, sys, time
os.environ['TTS_HOME'] = os.path.abspath(os.path.join('models', 'tts'))
import soundfile as sf, torch, torchaudio
from lessons import at


def load(path, *a, **k):  # torchaudio's torchcodec needs FFmpeg DLLs that are not on this machine
    d, r = sf.read(path, dtype='float32', always_2d=True)
    return torch.from_numpy(d.T), r


torchaudio.load = load
from TTS.api import TTS
from marks import xtts_text

out = at('out-full/xtts-clone')
os.makedirs(out, exist_ok=True)
tts = TTS('tts_models/multilingual/multi-dataset/xtts_v2').to('cuda')
phrases = json.load(open(at('full-ru.json'), encoding='utf-8'))
started = time.time()
failed = []
for n, p in enumerate(phrases, 1):
    path = f'{out}/{p["id"]:04d}.wav'
    if os.path.exists(path):
        continue
    try:
        tts.tts_to_file(text=xtts_text(p['ru_marked']), speaker_wav='ref/ref25.wav', language='ru', file_path=path, split_sentences=False)
    except Exception as e:
        failed.append(p['id'])
        print('failed', p['id'], repr(e)[:160], flush=True)
    if n % 25 == 0:
        print(f'{n} of {len(phrases)}, {time.time() - started:.0f} s', flush=True)
print(f'xtts done in {time.time() - started:.0f} s, failed: {failed}', flush=True)
if failed:
    sys.exit('Some phrases failed: run again, finished ones are kept.')
