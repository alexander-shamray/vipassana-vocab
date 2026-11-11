"""Fresh takes of the phrases listed in retry.json: python retry_xtts.py <round>. XTTS samples, so each seed differs."""
import json, os, sys
os.environ['TTS_HOME'] = os.path.abspath(os.path.join('models', 'tts'))
import soundfile as sf, torch, torchaudio
from lessons import at


def load(path, *a, **k):  # torchaudio's torchcodec needs FFmpeg DLLs that are not on this machine
    d, r = sf.read(path, dtype='float32', always_2d=True)
    return torch.from_numpy(d.T), r


torchaudio.load = load
from TTS.api import TTS
from marks import xtts_text

take = int(sys.argv[1])
speed = float(os.environ.get('DUB_XTTS_SPEED', '1'))  # dub hurry asks for faster speech
out = at('out-full/xtts-takes')
os.makedirs(out, exist_ok=True)
ids = set(json.load(open(at('retry.json'))))
tts = TTS('tts_models/multilingual/multi-dataset/xtts_v2').to('cuda')
for p in json.load(open(at('full-ru.json'), encoding='utf-8')):
    if p['id'] in ids:
        torch.manual_seed(1000 * take + p['id'])
        tts.tts_to_file(text=xtts_text(p['ru_marked']), speaker_wav='ref/ref25.wav', language='ru',
                        file_path=f'{out}/{p["id"]:04d}-{take}.wav', split_sentences=False, speed=speed)
print('takes done', take, len(ids), 'speed', speed, flush=True)
