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
- Go HTTP API and authoritative deterministic simulator with authored scenario rules
- Twelve synthetic, versioned scenarios spanning six tactical categories and three skill levels
- Fixture authoring validator with analyst rationale, tradeoffs, alternatives, and executable acceptance cases
- Offline Python telemetry windowing and highlight scoring using interpretable signals
- Analyst-labelled synthetic detector evaluation with precision, recall, category, and ranking gates
- Local normalized-telemetry collector, bounded live ingestion API, incremental detector, and identity-free visual timeline
- Restart-safe summary/draft storage with retention and deletion controls; raw telemetry and collector tokens remain memory-only
- Final-candidate to version 2.1 draft conversion with an enforced analyst publication gate
- Six unit classes with distinct health, movement, and attack profiles
- Click-to-inspect unit details and movement/attack range indicators
- `dodge` and `outplay` decisions alongside the original tactical actions
- Optional HTTP connector for model-suggested teammate and opponent positions
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
live non-player units—including the player's teammates and opponents—should try
to move in the next frame, but its output is only a target. The simulator keeps
the user-controlled unit outside model control, validates every proposal
atomically, applies class movement limits and map bounds, and remains solely
responsible for physics, damage, cooldowns, hidden state, and victory conditions.

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

## Optional position-model connector

No model, API key, or network service is required by default. Without a
connector, teammates and opponents use their seeded built-in policies.

To test an HTTP position model, configure its endpoint and stable identity for
the API process:

```bash
POSITION_MODEL_URL=http://127.0.0.1:9000/v1/positions \
POSITION_MODEL_NAME=trajectory-policy \
POSITION_MODEL_VERSION=2026.08.05 \
make dev-api
```

For Docker Compose, copy `.env.example` to `.env`, set the URL to an endpoint
reachable from the `api` container, set both identity values, and run
`docker compose up --build`. Once per turn the API posts the version `1.1`
session snapshot, accepts desired next-frame positions for live units other than
the player-controlled unit, and records accepted suggestions with that identity
in server-side session memory. Missing identity fails closed at startup; invalid
data, timeouts, and connection failures reject the full response and use the
deterministic fallback. Omitted teammates stay put, while omitted opponents use
the seeded chase policy. A model can never directly mutate simulator state.

Existing deployments may use the deprecated `OPPONENT_MODEL_URL`,
`OPPONENT_MODEL_NAME`, and `OPPONENT_MODEL_VERSION` aliases as one complete
group. These aliases preserve only the environment-variable names: the endpoint
must implement the same version `1.1` position-model protocol. Do not mix the
deprecated and preferred variable names. See
[`contracts/openapi.yaml`](contracts/openapi.yaml) for the exact webhook shapes.

## Simulator rules

Every fixture now declares its combat statistics, terrain, visibility, objective or escape state,
victory and defeat conditions, safe zone, unit policies, and an authored reference plan. Turns
produce causal events for movement, shielding, attacks, damage, eliminations, vision changes,
objective control, escape progress, and terminal outcomes.

The displayed **scenario advantage** is derived from remaining health, surviving units, objective
control, target pressure, and escape progress. It is deliberately not presented as a calibrated
win probability. At the end of a scenario, the API exposes deterministic rollouts for each legal
first action so the user can compare openings without claiming that any rollout is a historical
match result.

The terminal debrief can also reveal a calculated best allied line. Each turn is
selectable and shows the chosen command, its causal events, and how the strongest
continuation after every alternative command compared.

See [`docs/simulator-rules.md`](docs/simulator-rules.md) for the exact resolution order and limits.
See [`docs/scenario-authoring.md`](docs/scenario-authoring.md) to add or validate a scenario.
See [`docs/telemetry-scenario-drafts.md`](docs/telemetry-scenario-drafts.md) to
convert detector NDJSON into an analyst-reviewed scenario draft and preview it
without overwriting the authored library.
See [`docs/live-telemetry.md`](docs/live-telemetry.md) to replay a normalized
local match into the live detector and review its guarded drafts.

Validate the complete authored pack from `backend/`:

```bash
go run ./cmd/validate-fixtures -path ../fixtures/moments.json
```

## API

```text
GET  /healthz
GET  /api/v1/moments
POST /api/v1/sessions
GET  /api/v1/sessions/{id}
POST /api/v1/sessions/{id}/turns
POST /api/v1/sessions/{id}/reset
GET  /api/v1/telemetry/matches
POST /api/v1/telemetry/matches
DELETE /api/v1/telemetry/matches
GET  /api/v1/telemetry/matches/{id}
DELETE /api/v1/telemetry/matches/{id}
GET  /api/v1/telemetry/matches/{id}/timeline
POST /api/v1/telemetry/matches/{id}/frames
POST /api/v1/telemetry/matches/{id}/finish
GET  /api/v1/telemetry/matches/{id}/events
POST /api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft
GET  /api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft
PUT  /api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft
POST /api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft/validate
POST /api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft/preview
POST /api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft/review-pack
GET  /api/v1/local-storage
PUT  /api/v1/local-storage/retention
```

See [`contracts/openapi.yaml`](contracts/openapi.yaml) for request and response
shapes.

## Data and safety

All included data is synthetic and contains no player identity or proprietary
telemetry. The local API writes only finalized identity-free summaries and
analyst drafts to `.local-data/`, with a seven-day default retention policy.
Raw frames, source unit IDs, movement traces, and collector tokens are never
persisted. Production ingestion still requires explicit authorization, data
minimization, and game-publisher review before a source adapter is built. The
optional connector receives a server-side unit snapshot, so only an
operator-controlled URL should be configured; production model calls also
require encrypted transport, egress restrictions, and retention controls.

Normalized authorized or synthetic telemetry can be ranked offline with:

```bash
python3 -m ml.telemetry path/to/normalized-telemetry.json
```

See [`docs/telemetry-highlights.md`](docs/telemetry-highlights.md) for the strict
input contract, signal calculations, overlap suppression, and accuracy limits.

Run the checked-in detector regression pack and print its analyst-readable
report with:

```bash
python3 -m ml.evaluate.detector
```

See [`docs/detector-evaluation.md`](docs/detector-evaluation.md) for the labels,
matching rules, current baseline, and production-accuracy limits.

With the API and web app running, replay the identity-free demo locally:

```bash
cd backend
go run ./cmd/telemetry-collector --input ../fixtures/telemetry-demo.json --rate 4
```
