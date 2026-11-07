# Деплой: пошагово для владельца

Сайт живёт на Fly.io (Сингапур), база — SQLite на томе Fly, резервная копия
непрерывно уходит в Cloudflare R2 через Litestream. Озвучка и приватные
примеры тоже лежат в R2. Ориентировочная цена: Fly ~$2–5/мес, R2 — в
бесплатном лимите.

## 1. Cloudflare R2

1. В панели Cloudflare → R2 создайте три бакета:
   - `vv-audio` — **публичный** (Settings → Public access → r2.dev или свой домен);
   - `vv-private` — приватный, для `examples.jsonl`;
   - `vv-backup` — приватный, для Litestream.
2. R2 → Manage API tokens → создайте токен «Object Read & Write» на эти бакеты.
   Запишите `Access Key ID`, `Secret Access Key` и endpoint
   `https://<account-id>.r2.cloudflarestorage.com`.

## 2. OAuth

- **Google**: Google Cloud Console → APIs & Services → Credentials → OAuth client ID
  (Web application). Redirect URI: `https://<app>.fly.dev/auth/google/callback`.
- **GitHub**: Settings → Developer settings → OAuth Apps → New. Callback URL:
  `https://<app>.fly.dev/auth/github/callback`.

Можно настроить только одного провайдера — на странице входа будут лишь настроенные.

## 3. Fly.io

```bash
fly auth login
fly launch --no-deploy --copy-config --name <app>   # имя из fly.toml или своё
fly volumes create data --region sin --size 1
fly secrets set \
  BASE_URL=https://<app>.fly.dev \
  AUDIO_BASE_URL=https://<публичный-домен-vv-audio>/audio \
  ALLOWED_EMAILS=you@example.com,friend@example.com \
  SESSION_KEY=$(openssl rand -hex 32) \
  GOOGLE_CLIENT_ID=... GOOGLE_CLIENT_SECRET=... \
  GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... \
  LITESTREAM_BUCKET=vv-backup \
  LITESTREAM_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com \
  LITESTREAM_ACCESS_KEY_ID=... LITESTREAM_SECRET_ACCESS_KEY=...
```

Пригласить ещё кого-то — `fly secrets set ALLOWED_EMAILS=...` с полным списком.

## 4. GitHub Actions

В репозитории → Settings → Secrets and variables → Actions:

- Secrets: `FLY_API_TOKEN` (`fly tokens create deploy`), `R2_ENDPOINT`,
  `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_PRIVATE_BUCKET=vv-private`.
- Variables: `DEPLOY_ENABLED=true` — до этого workflow деплоя пропускается.

Деплой запускается сам на каждый merge в `main`, который трогает `app/`,
`dictionary/` или `fly.toml`; вручную — Actions → deploy → Run workflow.

## 5. Первый словарь

Деплой требует `dictionary/words.jsonl`. Соберите его пайплайном
([pipeline/README.md](../pipeline/README.md)), выгрузите (`python -m vv_pipeline upload`
с переменными `R2_*` и `R2_PUBLIC_BUCKET=vv-audio`, `R2_PRIVATE_BUCKET=vv-private`)
и закоммитьте `dictionary/words.jsonl` через PR.

## Восстановление

Новый том или новая машина: при старте `entrypoint.sh` сам восстанавливает базу
из `vv-backup` (`litestream restore -if-db-not-exists`), затем обновляет словарь.
Пометки пользователей ссылаются на ключ слова, поэтому пересборка словаря их
не теряет.
