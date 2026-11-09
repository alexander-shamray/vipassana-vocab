"""Silero eugene reads the whole lesson with the stress marks from the script."""
import json, os, sys, time
import soundfile as sf, torch
from lessons import at
from marks import fix_marks

model, _ = torch.hub.load('models/torchhub/snakers4_silero-models_master', 'silero_tts', source='local', language='ru', speaker='v5_ru')
model.to(torch.device('cuda'))
out = at('out-full/silero-eugene')
os.makedirs(out, exist_ok=True)
started = time.time()
failed = []
for p in json.load(open(at('full-ru.json'), encoding='utf-8')):
    path = f'{out}/{p["id"]:04d}.wav'
    if os.path.exists(path):
        continue
    try:
        a = model.apply_tts(text=fix_marks(p['ru_marked']), speaker='eugene', sample_rate=48000, put_accent=True, put_yo=False, put_stress_homo=False, put_yo_homo=False)
        sf.write(path, a.cpu().numpy(), 48000)
    except Exception as e:
        failed.append(p['id'])
        print('failed', p['id'], repr(e)[:160], flush=True)
print(f'silero done in {time.time() - started:.0f} s, failed: {failed}', flush=True)
if failed:
    sys.exit('Some phrases failed: run again, finished ones are kept.')
