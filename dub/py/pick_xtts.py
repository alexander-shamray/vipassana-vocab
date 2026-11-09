"""Whisper hears each fresh take; the best take replaces the phrase. pick_xtts.py init lists suspects in retry.json,
pick_xtts.py <round> scores that round and lists what is still wrong. Originals are kept in out-full/xtts-original."""
import glob, json, os, re, shutil, sys
sys.path.insert(0, '../../pipeline')
sys.stdout.reconfigure(encoding='utf-8')
import soundfile as sf
from lessons import at
from marks import fix_marks
from check_phrases_lib import distance, norm

CHECK = at('check-xtts-clone.json')
data = json.load(open(CHECK, encoding='utf-8'))
rows = {r['id']: r for r in data['rows']}
refs = {p['id']: norm(fix_marks(p['ru_marked'])) for p in json.load(open(at('full-ru.json'), encoding='utf-8'))}
raw = {p['id']: norm(p['ru_marked']) for p in json.load(open(at('full-ru.json'), encoding='utf-8'))}


def errs(i, heard):
    return distance(refs[i], norm(heard))


def wrong(i, r):
    return errs(i, r['heard']) >= 2 or len(norm(r['heard'])) > len(refs[i])


arg = sys.argv[1]
if arg == 'init':
    forced = [i for i in refs if refs[i] != raw[i]]  # the text itself changed, the old audio is wrong whatever it scores
    ids = sorted({i for i, r in rows.items() if wrong(i, r)} | set(forced))
    json.dump(forced, open(at('retry-forced.json'), 'w'))
    json.dump(ids, open(at('retry.json'), 'w'))
    print('suspects', len(ids), 'forced', forced)
    sys.exit()

from vv_pipeline import transcribe as tr
tr.use_pip_cuda()
from faster_whisper import WhisperModel

take = sys.argv[1]
forced = set(json.load(open(at('retry-forced.json'))))
m = WhisperModel('large-v3', device='cuda', compute_type='float16')
os.makedirs(at('out-full/xtts-original'), exist_ok=True)
replaced = 0
for path in sorted(glob.glob(at(f'out-full/xtts-takes/*-{take}.wav'))):
    i = int(os.path.basename(path).split('-')[0])
    segs, _ = m.transcribe(path, language='ru', condition_on_previous_text=False, beam_size=5, vad_filter=False)
    heard = ' '.join(s.text.strip() for s in segs)
    seconds = sf.info(path).duration
    old = rows[i]
    first = i in forced and not old.get('taken')
    if first or (errs(i, heard), seconds) < (errs(i, old['heard']), old['seconds']):
        current = at(f'out-full/xtts-clone/{i:04d}.wav')
        keep = at(f'out-full/xtts-original/{i:04d}.wav')
        if not os.path.exists(keep):
            shutil.copy(current, keep)
        shutil.copy(path, current)
        old.update(heard=heard, seconds=round(seconds, 2), taken=take, errors=errs(i, heard), ref_words=len(refs[i]),
                   chars_per_s=round(len(old['text']) / max(seconds, .01), 1))
        replaced += 1
still = sorted(i for i, r in rows.items() if wrong(i, r))
json.dump(still, open(at('retry.json'), 'w'))
for i, r in rows.items():
    r['errors'] = errs(i, r['heard'])
    r['ref_words'] = len(refs[i])
words = sum(r['ref_words'] for r in rows.values())
errors = sum(r['errors'] for r in rows.values())
data.update(wer_pct=round(100 * errors / words, 1), words=words, seconds=round(sum(r['seconds'] for r in rows.values())), still=still)
json.dump(data, open(CHECK, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
print(f'round {take}: replaced {replaced}, still suspect {len(still)}, WER {data["wer_pct"]}%', flush=True)
