# VoxIntel

Records a live panel/talk from the browser, transcribes it, and generates a
"Top Takeaways" summary on demand, built for single-operator use (one
person records; others can view and export past sessions).

## How it works

1. Open the app, pick an audio input (laptop mic, USB interface, or a
   line-in cable from a mixer/PA), and click **Start Recording**.
2. Click **Stop** when the session ends. The recording uploads and is
   transcribed automatically in the background.
3. Once transcribed, click **Generate Summary** to produce a "Top
   Takeaways" board from the transcript.
4. **Export** downloads the transcript + takeaways as a Markdown file.
   **Delete** permanently removes a session and its audio.

```
Browser (mic capture, MediaRecorder)
        │  upload audio
        ▼
Go backend ──▶ Whisper API (transcription)
        │
        ├──▶ LLM API (takeaways generation)
        │
        ▼
Local disk (session JSON + audio files)
```

Session data is stored as one JSON file per session under `data/sessions/`,
with audio files under `data/audio/`. There is no external database —
appropriate at this scale (one operator, low session volume), and it makes
backups and manual inspection trivial.

## Running it

A `Makefile` wraps the common commands:

| Command             | Does                                                                |
| ------------------- | ------------------------------------------------------------------- |
| `make build`        | Compiles the binary (`./voxintel`)                                  |
| `make run`          | Runs locally via `go run .`                                         |
| `make test`         | Runs the test suite                                                 |
| `make test-race`    | Runs tests with the race detector (checks concurrent access safety) |
| `make vet`          | Runs `go vet`                                                       |
| `make fmt`          | Lists any files not matching `gofmt`                                |
| `make docker-build` | Builds the Docker image                                             |
| `make docker-run`   | Runs the container, with a named volume for persistent session data |
| `make clean`        | Removes the compiled binary                                         |

### Local development

```bash
make build
make run
```

Reads config from a `.env` file in the working directory (see below).
Visit `http://localhost:8080`.

### Docker (recommended for deployment)

```bash
make docker-build
make docker-run
```

Equivalent to, if you need the raw commands (e.g. to adjust flags):

```bash
docker build -t voxintel .
docker run -p 8080:8080 --env-file .env -v voxintel-data:/app/data voxintel
```

The container is a multi-stage build — the final image contains only the
compiled binary and `ca-certificates` (needed to verify HTTPS to the
transcription/LLM APIs), not the Go toolchain. The frontend
(`static/index.html`, `app.js`, `style.css`) is compiled directly into the
binary via `go:embed`, so no separate files need to be mounted or copied.

Session data is written to `/app/data` inside the container. **Mount a
volume there if you need it to survive container restarts/redeploys:**

```bash
docker run -p 8080:8080 --env-file .env -v voxintel-data:/app/data voxintel
```

## Continuous integration

Every push/PR to `main` runs `go build`, `go vet`, `go test -race`, and a
`gofmt` check via GitHub Actions (`.github/workflows/ci.yml`). Check the
**Actions** tab on the repo for current status.

## Configuration

`.env.example` to `.env` and fill in real values. All variables are
read from the environment (or from `.env` in the working directory, if
present).

| Variable              | Required | Default                          | Purpose                                                                                                                                       |
| --------------------- | -------- | -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `AUTH_USERS`          | Yes      | —                                | Comma-separated `user:password` pairs for HTTP Basic Auth, e.g. `anant:pass1,varun:pass2`. Give each person their own login.                  |
| `TRANSCRIBE_API_KEY`  | Yes      | —                                | API key for the Whisper-compatible transcription endpoint (Groq or OpenAI).                                                                   |
| `ANALYZE_API_KEY`     | Yes      | —                                | API key for the chat-completions endpoint used to generate takeaways (Groq or OpenAI).                                                        |
| `TRANSCRIBE_BASE_URL` | No       | `https://api.groq.com/openai/v1` | Base URL for the transcription API.                                                                                                           |
| `TRANSCRIBE_MODEL`    | No       | `whisper-large-v3`               | Transcription model name. Verify against the provider's current `/models` endpoint before deploying — these are renamed/retired periodically. |
| `TRANSCRIBE_PROMPT`   | No       | _(empty)_                        | Optional vocabulary hint (panelist names, company names, jargon) to improve transcription accuracy for a specific event.                      |
| `ANALYZE_BASE_URL`    | No       | `https://api.groq.com/openai/v1` | Base URL for the analysis/LLM API.                                                                                                            |
| `ANALYZE_MODEL`       | No       | `openai/gpt-oss-120b`            | Chat-completion model used to generate takeaways. Same note as above re: verifying current model names.                                       |
| `PORT`                | No       | `8080`                           | HTTP listen port.                                                                                                                             |
| `DATA_DIR`            | No       | `data`                           | Directory for session/audio storage.                                                                                                          |

## API reference

All routes except `GET /healthz` require HTTP Basic Auth.

| Method   | Path                         | Purpose                                                            |
| -------- | ---------------------------- | ------------------------------------------------------------------ |
| `GET`    | `/healthz`                   | Liveness check — returns `{"status":"ok"}`. No auth required.      |
| `POST`   | `/api/sessions`              | Create a new session (called when recording starts).               |
| `GET`    | `/api/sessions`              | List all sessions.                                                 |
| `GET`    | `/api/sessions/{id}`         | Get one session's current status/transcript/takeaways.             |
| `POST`   | `/api/sessions/{id}/audio`   | Upload the recorded audio; triggers transcription.                 |
| `POST`   | `/api/sessions/{id}/analyze` | Generate takeaways from the transcript. Body: `{"prompt": "..."}`. |
| `GET`    | `/api/sessions/{id}/export`  | Download transcript + takeaways as Markdown.                       |
| `DELETE` | `/api/sessions/{id}`         | Permanently delete a session and its audio file.                   |

## Known limitations

- **Transcription accuracy depends heavily on audio quality.** Clean,
  close-mic, single-speaker audio typically transcribes with roughly
  10-20% word-level error. Audio from a distant laptop mic with multiple
  speakers talking over each other (common in panel discussions) can see
  30-45%+ error — this is a limitation of speech recognition generally,
  not specific to this app. **A direct line-in feed (cable from a
  mixer/PA) into the selected audio input significantly improves
  accuracy** over relying on the laptop's built-in microphone, and is
  recommended whenever available.
- **Single-operator recording model.** Only one person records a session
  at a time. Other users can view, export, and delete past sessions but
  do not have independent recording sessions of their own.
- **Authentication is intentionally simple.** HTTP Basic Auth (the
  browser's native login prompt) rather than a custom login page or
  session-based auth system — appropriate for a small, known group of
  users, not intended to scale to public/self-serve access.
- **No mobile-optimized UI.** Built and tested for desktop browser use.
- **Ephemeral audio input selection.** The list of available audio
  devices (including a plugged-in cable/USB interface) is read once when
  the page loads; if a device is plugged in after the page is already
  open, refresh the page to see it in the dropdown.
