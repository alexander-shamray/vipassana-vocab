"""Whisper hears the Russian narration, with the time of every word."""
import json, sys, os
sys.path.insert(0, '../../pipeline')
from lessons import RU_MEDIA, at
from vv_pipeline import transcribe as tr
tr.use_pip_cuda()
from faster_whisper import WhisperModel
m = WhisperModel('large-v3', device='cuda', compute_type='float16')
segs, info = m.transcribe(RU_MEDIA, language='ru', word_timestamps=True, vad_filter=False, condition_on_previous_text=False)
out = []
for s in segs:
    out.append({'start': s.start, 'end': s.end, 'text': s.text, 'words': [{'word': w.word, 'start': w.start, 'end': w.end} for w in s.words], 'avg_logprob': s.avg_logprob, 'no_speech_prob': s.no_speech_prob})
json.dump(out, open(at('ru-transcript-full.json'), 'w', encoding='utf-8'), ensure_ascii=False, indent=0)
print(len(out))
