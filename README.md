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
minimization, and game-publisher review before a source adapter is built.

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
