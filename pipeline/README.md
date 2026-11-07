# Пайплайн словаря

Запускается вручную на машине с видеокартой NVIDIA. Каждый шаг кэширует свой
результат в `data/` (не в git), поэтому шаги можно перезапускать по одному.

```bash
cd pipeline
py -3.12 -m venv .venv                      # .venv в .gitignore
.venv/Scripts/python -m pip install -e ".[run,dev]"
.venv/Scripts/python -m spacy download en_core_web_sm
# озвучка Kokoro: сначала torch с CUDA, потом сам Kokoro
.venv/Scripts/python -m pip install torch --index-url https://download.pytorch.org/whl/cu128
.venv/Scripts/python -m pip install -e ".[kokoro]"
```

CUDA 12 (cuBLAS) и cuDNN 9 ставятся из pip вместе с `[run]`; ставить CUDA в
систему не нужно — `transcribe` сам добавляет их в `PATH`. PyAV закреплён ниже
15: faster-whisper 1.2 вызывает параметр, которого в PyAV 15+ нет. На Linux и
macOS вместо `.venv/Scripts/python` — `.venv/bin/python`.

| Шаг | Команда | Нужно | Результат |
|---|---|---|---|
| Скачать аудио | `python -m vv_pipeline fetch [--only 1]` | — | `data/audio/`, `data/talks.json` |
| Расшифровать | `python -m vv_pipeline transcribe [--only 1]` | GPU, ~1 ч на все беседы | `data/transcripts/*.json` |
| Леммы | `python -m vv_pipeline extract` | — | `data/lemmas.json` |
| Перевод + сборка | `python -m vv_pipeline translate` | `ANTHROPIC_API_KEY` | `data/translations.json`, `dictionary/words.jsonl`, `data/examples.jsonl` |
| Только сборка | `python -m vv_pipeline build` | — | то же из кэша переводов |
| Озвучка | `python -m vv_pipeline tts` | GPU (Kokoro, по умолчанию) | `data/audio_tts/*.mp3` |
| Озвучка Azure | `python -m vv_pipeline build tts --tts azure` | `AZURE_SPEECH_KEY`, `AZURE_SPEECH_REGION` | то же, другие имена файлов |
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
- Отбрасываются отрезки с `avg_logprob < -1.0`, тишина (`no_speech_prob > 0.6`
  вместе с `avg_logprob < -0.5`) и текст не латиницей. Язык по отрезкам не
  определяется: на беседах 1–2 детектор принял за бенгальский, санскрит и
  тибетский 8,6% английских отрезков; пение на хинди вырезает VAD.
- Перевод: `claude-opus-5-5`, `effort: low`, ответ по JSON-схеме, порции по 40 слов,
  кэш после каждой порции. Термины глоссария в модель не отправляются.
- Озвучка (`--tts`): по умолчанию Kokoro-82M на своей видеокарте — британский
  голос `bm_george` для английского, голос хинди `hm_omega` читает пали по IPA
  (`ʱ` заменяется на `ʰ`, которого Kokoro знает). `--tts azure`:
  `en-GB-RyanNeural` и `en-IN-PrabhatNeural` + IPA. Имя mp3 — хэш движка,
  голоса и произносимого, поэтому смена движка даёт новые файлы, а не старый
  кэш. Сборка (`build`) должна идти с тем же `--tts`, что и озвучка.
- `server dev` сам раздаёт `data/audio_tts/` по адресу `/audio/`, так что
  локальный сайт играет mp3 без облака.
