# Playable Replays

Playable Replays is a Garena AI Build Challenge 2026 prototype: a shareable
web tactical board built around three teaching scenarios from the T1–Bilibili
Gaming 2024 Worlds Final. Choose a high-level command, inspect the authoritative
simulation, react to incoming marksman projectiles with a separate two-charge
Dodge control, and compare the result with an authored decision tree.

This is a compact counterfactual teaching model—not a proprietary game-engine
recreation, a replay-telemetry viewer, or proof of what a professional player
intended. Outcomes are deterministic simulator results. The full-map unit,
terrain, objective, and turret coordinates are **authored normalized
approximations informed by reviewed minimap frames; they are not replay
telemetry and are not replay-exact historical positions**.

## What is included

- Three versioned, replay-derived scenarios: one beginner, one intermediate,
  and one advanced.
- A React and strict TypeScript full-map board with movable player inspection,
  class/range indicators, six server-supplied turrets, projectile paths, a
  timeline, causal logs, and an outcome decision tree.
- Four tactical turn actions only: `move`, `hold`, `contest`, and `retreat`.
- A separate Dodge reaction with two charges. Dodge removes an eligible pending
  projectile and repositions the controlled unit without advancing the turn.
- A Go HTTP API and authoritative simulator for movement, combat, fog, class
  limits, projectile resolution, objectives, outcomes, and reference search.
- A Python model daemon that makes a real OpenAI Responses API call for every
  live non-controlled unit's advisory action. Invalid or unavailable model
  output falls back to deterministic Go policies.
- OpenAPI and JSON Schema contracts, executable fixture acceptance cases,
  frontend/backend/model-daemon tests, Docker Compose, and GitHub Actions.

There is no live telemetry ingestion, telemetry dashboard, local telemetry
storage, or runtime scenario-generation workflow.

## Scenarios

| Scenario | Source moment | Teaching focus |
| --- | --- | --- |
| Bank the Cross-map Trade | Game 1, 15:32 | Preserve a completed objective trade instead of donating the follow-up fight. |
| Wait for the Damage Dealer | Game 3, 21:35 | Delay the engage until the marksman can contribute damage. |
| Re-engage the Split 3v4 | Game 5, 28:47 | Judge local target access and effective participants instead of headline player count. |

For Game 3, the bundle's QA recommendation intentionally rewinds the teaching
frame to 21:35, ten seconds before source moment 05's 21:45 core window, so the
setup is visible before the learner commits.

Difficulty labels describe how many tactical signals the learner combines;
they are not ratings of the named players.

## Quick start

Requirements: Go 1.26.5+, Node.js 22+, and Python 3.12+.

Run all local checks:

```bash
make test
```

Run the deterministic fallback experience in two terminals:

```bash
make dev-api
```

```bash
make dev-web
```

Open <http://localhost:5173>. Vite proxies `/api` and `/healthz` to the Go API
at `http://127.0.0.1:8080`.

### Run with the real model bridge

Start the model daemon with an operator-owned OpenAI API key:

```bash
OPENAI_API_KEY='your-key' make dev-model
```

Then configure the Go API to call it:

```bash
BOT_MODEL_URL=http://127.0.0.1:9000/v1/actions \
BOT_MODEL_NAME=openai-npc-actions \
BOT_MODEL_VERSION=gpt-5.6 \
make dev-api
```

Start the web app with `make dev-web`. The API key belongs only in the daemon
environment; never put it in the browser, fixtures, source, or a committed
`.env` file. If you override `OPENAI_MODEL`, update `BOT_MODEL_VERSION` to the
same deployed model identifier or another accurate rollout identifier.

Docker Compose starts all three services and configures the server-to-server
bridge:

```bash
cp .env.example .env
# Set OPENAI_API_KEY in .env for real model-backed bot turns.
docker compose up --build
```

Without a key, the daemon deliberately returns a non-success response and the
Go engine uses its deterministic fallback, so the local stack remains playable.

## Runtime architecture

The browser sends one validated tactical action to Go. At each accepted turn,
Go may send a schema `2.0` privileged snapshot to the model daemon. The daemon
uses strict Structured Outputs to request one legal action for every live unit
except the user-controlled unit. Go rejects the entire response if any unit or
action is missing, duplicated, unknown, malformed, illegal, or out of bounds.
Accepted Move targets are still constrained by server-owned class movement
limits, and Go alone resolves damage, visibility, projectiles, objectives, and
terminal state.

`Session.botControl.source` explains the active path:

- `pending`: a model is configured but no turn has completed yet;
- `external-model`: a complete model response was accepted, with model name and
  version; or
- `deterministic-fallback`: no model is configured or the configured call was
  unavailable or unusable.

See [docs/architecture.md](docs/architecture.md) and
[docs/model-plan.md](docs/model-plan.md) for the trust and failure boundaries.

## Gameplay contract

All coordinates use a normalized inclusive `0..100` full map. Every session
contains five blue and five red units, exactly one marksman per team, and
exactly six canonical turrets—one per team in each lane. Turrets are currently
map landmarks; the server supplies their health/state, but they do not execute
combat logic.

Marksman attacks create a visible pending projectile aimed at a unit and worth
half that target's maximum health, rounded up. It remains pending until the
next tactical turn starts. If the controlled unit is the target, the separate
Dodge control may consume one of two charges to evade it immediately without
advancing the tactical turn. Otherwise the projectile resolves before the next
user command.

The four tactical commands are:

- `move`: travel toward a chosen full-map point, capped by class range;
- `hold`: gain a small shield and guard for the current turn;
- `contest`: focus the visible authored target, otherwise close on the nearest
  visible enemy, or advance on an objective when no target is visible; and
- `retreat`: move toward the authored safe zone with defensive guard.

The ending decision tree and best-case line search only these four commands.
Reference simulation automatically uses the same two-charge Dodge reaction
when a projectile is eligible; Dodge is never a fifth tactical action.

See [docs/simulator-rules.md](docs/simulator-rules.md) for resolution order and
[docs/scenario-authoring.md](docs/scenario-authoring.md) for fixture rules.

## API

```text
GET  /healthz
GET  /api/v1/moments
POST /api/v1/sessions
GET  /api/v1/sessions/{id}
POST /api/v1/sessions/{id}/turns
POST /api/v1/sessions/{id}/dodge
POST /api/v1/sessions/{id}/reset
```

The model daemon exposes `GET /healthz` and `POST /v1/actions`. The browser must
never call the model daemon directly. See [contracts/openapi.yaml](contracts/openapi.yaml)
and [model-daemon/README.md](model-daemon/README.md) for exact payloads.

## Repository layout

| Path | Purpose |
| --- | --- |
| `backend/` | Go API, authoritative simulator, fixture loading, and tests |
| `frontend/` | React full-map tactical board and component/API tests |
| `model-daemon/` | Bounded Python bridge from schema `2.0` bot snapshots to the OpenAI Responses API |
| `contracts/` | Public OpenAPI contract and fixture JSON Schema |
| `fixtures/moments.json` | Version `3.0` pack containing the three authored scenarios |
| `docs/` | Architecture, model, simulator, and authoring decisions |

Validate the authored pack from `backend/`:

```bash
go run ./cmd/validate-fixtures -path ../fixtures/moments.json
```

## Source attribution and disclosure

This prototype was prepared for the **Garena AI Build Challenge 2026 — Case
Brief**. Its three scenarios derive from the supplied evidence bundle
`playable-replays-t1-blg-2024`, SHA-256
`07a8b2732dfb62e4d416e011bd4f5e0317a4bf38e84963d0030c14104cc07d1f`.
The bundle identifies its media source as Caedrel's edited upload of the T1 vs
Bilibili Gaming 2024 Worlds Final, not a 2026 match:
[YouTube VOD](https://www.youtube.com/watch?v=xiKg7qfaPAI). It cross-references
Games of Legends match timelines `62816`–`62820`, the
[Riot Games Worlds 2024 Primer](https://lolesports.com/en-PH/news/worlds-2024-primer),
and secondary analysis from Sheep Esports and Team Liquid/SAP.

Fixtures retain the bundle ID/hash, source-moment ID, game and decision time,
caption evidence IDs, external evidence IDs, analyst assessment, and coaching
correction. Those anchors support the teaching premise; they do not turn the
simulator layout into measured replay state. **Every map coordinate and spatial
arrangement in this project is an authored normalized approximation, not
extracted replay telemetry.** Names and match references are contextual source
attribution and do not imply endorsement by the teams, players, publishers,
event organizers, or source authors.
