# retriv-back

Privacy-preserving lost-and-found matching API. Go (stdlib-first), PostgreSQL +
PostGIS + pgvector, Ollama for embeddings and verification grading.

## Quick start

```bash
cp .env.example .env          # then edit secrets (ENC_KEY, SESSION_SECRET)
make up                       # Postgres+PostGIS+pgvector, Ollama, Mailpit
make models                   # pull nomic-embed-text + llama3.1 (once)
make tidy                     # resolve go.sum
make run                      # migrations apply on boot; API on :8080
```

Check it: `curl localhost:8080/healthz`. Dev email lands in Mailpit at
http://localhost:8025.

## Layout

- `cmd/retriv` — entrypoint and wiring.
- `internal/config` — env → `Config`.
- `internal/db` — pgx pool + embedded goose migrations.
- `internal/{auth,reports,matching,verification,channel,notify}` — one domain
  each: `service.go` (logic) + `repo.go` (hand-written SQL).
- `internal/ollama` — embeddings + chat client.
- `internal/crypto` — AES-GCM for finder verification details.
- `internal/api` — HTTP transport (thin handlers, `net/http` 1.22 routing).

## API

| Method + path                         | Auth        | Purpose                                     |
| ------------------------------------- | ----------- | ------------------------------------------- |
| `POST /api/auth/register`             | —           | create account, email sign-in link          |
| `POST /api/auth/login`                | —           | email sign-in link                          |
| `GET /api/auth/verify?token=`         | —           | consume link, set session, redirect         |
| `POST /api/auth/logout`               | —           | clear session                               |
| `GET /api/me`                         | yes         | current user                                |
| `POST /api/reports`                   | yes         | submit report; returns binary match status  |
| `GET /api/reports`                    | yes         | my reports                                  |
| `GET /api/matches/{id}/questions`     | yes (loser) | verification prompts                        |
| `POST /api/matches/{id}/answers`      | yes (loser) | submit answers -> verified/failed/discarded |
| `POST /api/matches/{id}/resolve`      | yes         | mark returned                               |
| `GET/POST /api/matches/{id}/messages` | yes (party) | masked channel                              |
| `GET /api/notifications`              | yes         | in-app notifications                        |

The frontend proxies `/api` to this server so the session cookie and magic-link
redirect share one origin.
