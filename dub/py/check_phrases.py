"""Per-phrase check of a synthesized track: python check_phrases.py <engine>. Whisper hears each phrase on its own,
so a garbled, cut or babbling phrase shows up by id instead of disappearing in a whole-lesson average."""
import json, os, re, sys
import soundfile as sf
sys.path.insert(0, os.path.join(os.environ.get('DUB_REPO', '../..'), 'pipeline'))
from check_phrases_lib import distance, norm
from lessons import at
sys.stdout.reconfigure(encoding='utf-8')
from vv_pipeline import transcribe as tr
tr.use_pip_cuda()
from faster_whisper import WhisperModel

engine = sys.argv[1]




m = WhisperModel('large-v3', device='cuda', compute_type='float16')
rows = []
for n, p in enumerate(json.load(open(at('full-ru.json'), encoding='utf-8')), 1):
    path = at(f'out-full/{engine}/{p["id"]:04d}.wav')
    info = sf.info(path)
    segs, _ = m.transcribe(path, language='ru', condition_on_previous_text=False, beam_size=5, vad_filter=False)
    heard = ' '.join(s.text.strip() for s in segs)
    ref, got = norm(p['ru_marked']), norm(heard)
    rows.append({'id': p['id'], 'text': p['text'], 'heard': heard, 'ref_words': len(ref), 'errors': distance(ref, got),
                 'seconds': round(info.duration, 2), 'chars_per_s': round(len(p['text']) / max(info.duration, .01), 1)})
    if n % 100 == 0:
        print(n, flush=True)
words = sum(r['ref_words'] for r in rows)
errors = sum(r['errors'] for r in rows)
bad = sorted((r for r in rows if r['errors'] / max(r['ref_words'], 1) >= .25 or r['ref_words'] and not 7 <= r['chars_per_s'] <= 25), key=lambda r: -r['errors'])
speeds = sorted(r['chars_per_s'] for r in rows)
json.dump({'wer_pct': round(100 * errors / words, 1), 'words': words, 'seconds': round(sum(r['seconds'] for r in rows)), 'bad': bad, 'rows': rows},
          open(at(f'check-{engine}.json'), 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
print(f'{engine}: WER {100 * errors / words:.1f}% of {words} words, {sum(r["seconds"] for r in rows) / 60:.1f} min of speech, chars/s {speeds[0]}..{speeds[-1]}, median {speeds[len(speeds) // 2]}, flagged {len(bad)}')
