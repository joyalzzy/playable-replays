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
- Six unit classes with distinct health, movement, and attack profiles
- Click-to-inspect unit details and movement/attack range indicators
- `dodge` and `outplay` decisions alongside the original tactical actions
- Optional HTTP connector for model-suggested opponent positions
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
resolves that action using a scenario seed. An optional model may suggest where
opponents should try to move in the next frame, but its output is only a target:
the simulator validates unit ownership and coordinates, applies class movement
limits and map bounds, and remains solely responsible for physics, damage,
cooldowns, hidden state, and victory conditions.

## Unit classes

Every fixture unit has an explicit class. The class controls maximum health,
per-frame movement, and attack radius; the API exposes `moveRange` and
`attackRange` so the board can explain those limits rather than hiding them.
Tanks are the toughest and slowest class, while marksmen trade health for range.

| Class | Maximum health | Move range | Attack range |
| --- | ---: | ---: | ---: |
| Tank | 160 | 7 | 10 |
| Fighter | 125 | 10 | 14 |
| Marksman | 90 | 11 | 28 |
| Mage | 95 | 9 | 24 |
| Support | 110 | 8 | 20 |
| Assassin | 100 | 13 | 12 |

Ranges are map units per frame.

## Optional opponent-model connector

No model, API key, or network service is required by default. Without a
connector, opponents use the seeded built-in policy.

To test an HTTP position model, configure its endpoint and stable identity for
the API process:

```bash
OPPONENT_MODEL_URL=http://127.0.0.1:9000/v1/positions \
OPPONENT_MODEL_NAME=trajectory-policy \
OPPONENT_MODEL_VERSION=2026.08.04 \
make dev-api
```

For Docker Compose, copy `.env.example` to `.env`, set the URL to an endpoint
reachable from the `api` container, set both identity values, and run
`docker compose up --build`. Once per turn the API posts the session snapshot,
accepts desired next-frame opponent positions, and records accepted suggestions
with that identity in server-side session memory. Missing identity fails closed
at startup; invalid data, timeouts, and connection failures use the deterministic
fallback. A model can never directly mutate simulator state. See
[`contracts/openapi.yaml`](contracts/openapi.yaml) for the exact webhook shapes.

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
telemetry. The optional connector receives a server-side unit snapshot, so only
an operator-controlled URL should be configured. Production ingestion and model
calls must require authorization, encrypted transport, data minimization,
retention controls, egress restrictions, and game-publisher review.
