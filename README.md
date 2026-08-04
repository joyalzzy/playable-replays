# Playable Replays

A deterministic, web-based esports replay prototype. It turns synthetic MOBA
telemetry into short tactical scenarios where a user can replay a pivotal
decision, compare their choice with a reference policy, and share a stable
scenario URL.

The prototype deliberately does **not** claim to recreate a proprietary game
engine or predict what would certainly have happened in a real match. Outcomes
come from a small, deterministic simulator and are labelled as counterfactual
estimates.

## What is included

- React + strict TypeScript tactical board
- Go HTTP API and authoritative deterministic simulator
- Synthetic, versioned telemetry fixtures
- Offline Python highlight scorer using interpretable signals
- OpenAPI contract and JSON schemas
- Unit, API, frontend, and preprocessing tests
- Docker Compose and GitHub Actions

## Quick start

Requirements: Go 1.26.5+, Node.js 22+, and Python 3.11+.

```bash
make test
make dev-api
```

In another terminal:

```bash
make dev-web
```

Open <http://localhost:5173>. The Vite development server proxies `/api` to
the Go API at `http://localhost:8080`.

Alternatively:

```bash
docker compose up --build
```

The web app is then available at <http://localhost:5173>.

## Repository layout

| Path | Purpose |
| --- | --- |
| `backend/` | Go API, simulator, fixture loading, and tests |
| `frontend/` | React tactical board and component tests |
| `contracts/` | OpenAPI and JSON schema contracts |
| `fixtures/` | Synthetic replay moments |
| `ml/` | Offline highlight scoring and tests |
| `docs/` | Architecture, limitations, and roadmap |

## Core design rule

The browser requests a legal high-level action. The Go simulator validates and
resolves that action using a scenario seed. An LLM is never trusted to invent
physics, hidden state, damage, or victory conditions.

## API

```text
GET  /healthz
GET  /api/v1/moments
POST /api/v1/sessions
GET  /api/v1/sessions/{id}
POST /api/v1/sessions/{id}/turns
POST /api/v1/sessions/{id}/reset
```

See [`contracts/openapi.yaml`](contracts/openapi.yaml) for request and response
shapes.

## Data and safety

All included data is synthetic and contains no player identity or proprietary
telemetry. Production ingestion must require authorization, data minimization,
retention controls, and game-publisher review.
