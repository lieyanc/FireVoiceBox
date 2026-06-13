# FireVoiceBox

A lightweight, **single-binary** web portal for collecting voice messages and audio blessings — built for things like graduation well-wishes, event greetings, or any "send us a voice note" campaign.

- 🎙️ **Public recording page** — open a link, enter your student ID + nickname, grant mic permission, record, re-listen, upload. No account needed.
- 🗂️ **Per-project settings** — title, description, max recording length, and per-IP upload caps.
- 🛠️ **Admin backend** — review every submission with IP, student ID, nickname, an inline **waveform player**, one-click **ZIP export** (audio + `metadata.csv`).
- 🔗 **Two-tier access** — a global admin owns/creates projects; each project also gets a shareable manage link (token) so collaborators can review one project without the master password.
- 📦 **One binary** — Go backend with the React UI embedded via `go:embed`. Pure-Go SQLite (no CGO), so it cross-compiles cleanly. Audio stored on the filesystem.
- 🎚️ **Optional transcoding** — store browser-native formats as-is, or normalize everything to MP3 using the system `ffmpeg`.

Built with **Go** (net/http + chi + modernc SQLite) and **React + TypeScript + Tailwind + shadcn-style UI + wavesurfer.js**.

---

## Quick start

```bash
# 1. Build the UI and the binary (requires Go 1.24+, Node 18+, pnpm)
make build

# 2. Run it
./bin/firevoicebox
```

On first run the binary **self-releases** `config.json` next to itself, generating a random admin password and cookie-signing secret. Watch the log:

```
config: created config.json with generated secrets
config: admin password = 18281d2e2336d51f69a0cf16  (change it in config.json if desired)
FireVoiceBox listening on :8080 (data: ./data)
```

Open <http://localhost:8080/admin>, log in with that password, and create your first project. The recording link looks like `http://localhost:8080/r/<slug>`.

> ⚠️ **Microphone access requires HTTPS** (browsers only allow `getUserMedia` on `https://` or `localhost`). For real use, put it behind a TLS-terminating reverse proxy — see [Deployment](#deployment).

---

## Build from source

Prerequisites: **Go 1.24+**, **Node 18+**, **pnpm**.

| Command | What it does |
| --- | --- |
| `make build` | Build the React app into `internal/web/dist`, then compile `bin/firevoicebox` with the UI embedded. |
| `make backend` | Compile the Go binary only (UI must already be built). |
| `make web` | Build the frontend only. |
| `make test` | Run all Go tests. |
| `make release` | Cross-compile a static `linux/amd64` binary (`bin/firevoicebox-linux-amd64`). |
| `make web-dev` | Start the Vite dev server on `:5173`, proxying `/api` → `:8080`. |

**Local development:** run the API with `go run ./cmd/firevoicebox` (serves `:8080`), and in another terminal `make web-dev` for hot-reloading UI on `:5173`.

---

## Configuration (`config.json`)

The config file is JSON. It is created automatically on first run; you can also point at a custom path with `-config /path/to/config.json`.

```jsonc
{
  "server": {
    "addr": ":8080",          // listen address
    "data_dir": "./data",     // SQLite db + audio files live here
    "trusted_proxy": true,    // trust X-Forwarded-For / X-Real-IP (enable behind a reverse proxy)
    "max_upload_mb": 25,      // reject uploads larger than this
    "secret": "<auto>"        // HMAC key for signing the admin session cookie
  },
  "admin": {
    "password": "<auto>"      // global owner password (login at /admin)
  },
  "transcode": {
    "enabled": false,         // false = store native webm/mp4; true = transcode to a single format
    "ffmpeg_path": "ffmpeg",  // path to the system ffmpeg binary
    "format": "mp3",
    "bitrate": "128k",
    "on_error": "keep_original" // on transcode failure: "keep_original" or "reject"
  },
  "update": {
    "enabled": false,         // enable background OTA checks
    "channel": "stable",      // "stable" = latest v* release; "dev" = dev prerelease
    "check_interval": 3600,   // seconds between background checks
    "tag": "",                // optional exact GitHub release tag; empty uses latest/dev
    "repo": "lieyanc/FireVoiceBox"
  }
}
```

### Audio: native vs. transcoded

- **Native (default):** uploads are stored exactly as the browser produced them (`audio/webm` on Chrome/Firefox, `audio/mp4` on Safari). Zero external dependencies. Review the admin page in a Chromium-based browser, which decodes both.
- **Transcoded:** set `transcode.enabled: true`. Each upload is converted to MP3 via the system `ffmpeg`, so it plays everywhere. If `ffmpeg` is missing at startup, the app logs a warning and falls back to native mode. Per-file failures follow `on_error`.

### `trusted_proxy`

Per-IP upload limits and the logged submitter IP rely on the client IP. When behind a reverse proxy, set `trusted_proxy: true` so the app reads `X-Forwarded-For`/`X-Real-IP`. When exposed directly to clients, set it to `false` — otherwise those headers are spoofable and could bypass the per-IP cap.

---

## Deployment

The binary speaks plain HTTP; terminate TLS in front of it. Because microphone capture needs a secure context, **a real certificate is required** for anything beyond `localhost`.

### Caddy (automatic HTTPS)

```caddy
voice.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name voice.example.com;

    ssl_certificate     /etc/letsencrypt/live/voice.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/voice.example.com/privkey.pem;

    client_max_body_size 30m;   # must be >= server.max_upload_mb

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

Keep `server.trusted_proxy: true` with either proxy. Run the binary under systemd (or any supervisor); it shuts down gracefully on SIGTERM.

### CDN cache policy

FireVoiceBox sets origin cache headers that are safe to pass through a CDN:

| Path | Cache policy |
| --- | --- |
| `/assets/*` | `public, max-age=31536000, immutable` because Vite emits content-hashed filenames. |
| `/`, `/index.html`, SPA routes | `no-store, max-age=0` so a deploy or OTA update can replace the client entry point immediately. |
| `/api/*` | `private, no-store, max-age=0` because responses can contain project data, sessions, exports, or token-protected audio. |

Configure the CDN to respect the origin `Cache-Control` header, and avoid a
blanket "cache everything" rule for HTML or `/api/*`. When the admin update
page detects that the backend has switched versions, it reloads the current URL
with `fvb_refresh=<version>`; that response also sends
`Clear-Site-Data: "cache"` to ask browsers to drop origin caches before loading
the fresh app. Other open client pages poll `/api/client/version` and use the
same refresh path when they see a new client cache key.

### systemd unit (example)

```ini
[Unit]
Description=FireVoiceBox
After=network.target

[Service]
WorkingDirectory=/opt/firevoicebox
ExecStart=/opt/firevoicebox/firevoicebox -config /opt/firevoicebox/config.json
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

**Backups:** everything lives under `data_dir` — `data/firevoicebox.db` plus `data/audio/`. Copy/rsync that directory.

### CI and OTA updates

GitHub Actions builds release binaries via `.github/workflows/cross-compile.yml`.
On pushes to `main`, it runs the frontend build and Go tests, cross-compiles
Linux (`amd64`, `arm64`) and macOS (`amd64`, `arm64`) binaries, uploads
`.sha256` checksums, and refreshes the fixed `dev` prerelease with
`version.json`. Tags matching `v*` publish stable releases and mark them as
latest.

The admin backend exposes a **系统更新** panel. It downloads directly from
GitHub Releases by tag, using the matching `firevoicebox-<target>` asset and
its `.sha256` checksum. Set `update.tag` to an exact release tag, or leave it
empty so stable follows GitHub's latest-release redirect and dev uses the fixed
`dev` tag. Stable updates apply immediately after download; dev updates wait
for admin confirmation.

For local smoke tests of published assets:

```bash
scripts/fetch-latest-build.sh --download-only
scripts/fetch-latest-build.sh --stable -- -config config.json
```

---

## How it works

```
cmd/firevoicebox/      entry point: config load, wiring, graceful shutdown
internal/config/       JSON config + embedded template + self-release
internal/store/        SQLite (modernc, CGO-free): projects & submissions
internal/audio/        filesystem storage + optional ffmpeg transcoding
internal/server/       chi router, auth, handlers, SPA serving
internal/web/          //go:embed of the built React app (dist)
web/                   React + TS + Tailwind + shadcn-style UI + wavesurfer.js
```

### API surface

| Method & path | Auth | Purpose |
| --- | --- | --- |
| `GET /api/p/{idOrSlug}` | public | Project info for the recording page |
| `POST /api/p/{idOrSlug}/submissions` | public | Upload a recording (multipart) |
| `POST /api/admin/login` · `/logout` · `GET /api/admin/me` | — | Owner session |
| `GET/POST/PATCH/DELETE /api/admin/projects[/{id}]` | owner | Project CRUD |
| `GET /api/manage/projects/{id}` | owner **or** token | Project + stats |
| `GET /api/manage/projects/{id}/submissions` | owner **or** token | List submissions |
| `GET /api/manage/submissions/{id}/audio` | owner **or** token | Stream audio (supports HTTP Range) |
| `DELETE /api/manage/submissions/{id}` | owner **or** token | Delete a submission |
| `GET /api/manage/projects/{id}/export` | owner **or** token | Download ZIP (audio + `metadata.csv`) |
| `GET /api/admin/version` | owner | Current binary version and OTA repo/channel. |
| `GET/POST /api/admin/update/*` | owner | Check, download, apply, or dismiss OTA updates. |

Manage-token requests carry the token via the `X-Manage-Token` header or `?key=` query parameter (the share link puts it in the URL hash, and the SPA forwards it).

---

## Notes & limitations

- The recorder uses the browser `MediaRecorder` API. It's widely supported (Chrome, Firefox, Edge, Safari 14.3+). The client-reported duration is validated server-side against the project limit, but it isn't a hard cryptographic guarantee — the `max_upload_mb` cap is the real backstop.
- Sessions are stateless signed cookies (HMAC over the `server.secret`). Rotating the secret invalidates existing logins.
- SQLite is opened in WAL mode with a single writer connection — plenty for an event-scale workload (hundreds to low-thousands of submissions).

## License

See [LICENSE](LICENSE).
