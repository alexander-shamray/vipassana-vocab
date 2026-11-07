# Пайплайн словаря

Запускается вручную на машине с видеокартой NVIDIA. Каждый шаг кэширует свой
результат в `data/` (не в git), поэтому шаги можно перезапускать по одному.

```bash
cd pipeline
py -3.12 -m pip install -e ".[run,dev]"
py -3.12 -m spacy download en_core_web_sm
# для GPU: CUDA 12 и cuDNN 9 — см. https://github.com/SYSTRAN/faster-whisper#gpu
```

| Шаг | Команда | Нужно | Результат |
|---|---|---|---|
| Скачать аудио | `python -m vv_pipeline fetch [--only 1]` | — | `data/audio/`, `data/talks.json` |
| Расшифровать | `python -m vv_pipeline transcribe [--only 1]` | GPU, ~1 ч на все беседы | `data/transcripts/*.json` |
| Леммы | `python -m vv_pipeline extract` | — | `data/lemmas.json` |
| Перевод + сборка | `python -m vv_pipeline translate` | `ANTHROPIC_API_KEY` | `data/translations.json`, `dictionary/words.jsonl`, `data/examples.jsonl` |
| Только сборка | `python -m vv_pipeline build` | — | то же из кэша переводов |
| Озвучка | `python -m vv_pipeline tts` | `AZURE_SPEECH_KEY`, `AZURE_SPEECH_REGION` | `data/audio_tts/*.mp3` |
| Выгрузка | `python -m vv_pipeline upload` | `R2_*` (см. [docs/deploy.md](../docs/deploy.md)) | mp3 в публичный бакет, примеры — в приватный |
| Всё сразу | `python -m vv_pipeline all` | всё выше | |

Начните с одной беседы (`--only 1`) и посмотрите `data/transcripts/` глазами:
как записались слова пали и отброшены ли дохи на хинди (поле `language`).

## Ручная работа владельца

- `glossary/pali.yaml` и `glossary/dhamma.yaml` — переводы терминов. Проверили —
  поставьте `reviewed: true`, и пометка «черновик перевода» на сайте исчезнет.
  Новые написания, которые путает Whisper, добавляйте в `variants`.
- `overrides/words.yaml` — убрать мусорное слово, склеить формы, исправить
  перевод любого слова. Затем `python -m vv_pipeline build` и PR с новым
  `dictionary/words.jsonl`.

## Правила, зашитые в код

- Примеры: не больше 3 на слово, фраза 3–25 слов, по возможности из разных
  бесед; у стоп-слов и 2% самых частых слов примеров нет (у терминов — есть).
- Отрезки не на английском, с `avg_logprob < -1.0` или `no_speech_prob > 0.6`
  отбрасываются.
- Перевод: `claude-opus-5-5`, `effort: low`, ответ по JSON-схеме, порции по 40 слов,
  кэш после каждой порции. Термины глоссария в модель не отправляются.
- Озвучка: `en-GB-RyanNeural` для английского, `en-IN-PrabhatNeural` + IPA для пали.
