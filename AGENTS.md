<!-- markdownlint-disable MD013 -->

# Playable Replays agent guide

This file applies to the whole repository. A nested `AGENTS.md` may add local
rules but must not weaken the authority, contract, source-disclosure,
determinism, security, or validation rules below.

## Mission and non-negotiable boundaries

Playable Replays is a shareable full-map MOBA tactical board with three
replay-derived teaching scenarios from the T1–Bilibili Gaming 2024 Worlds
Final.

- The Go simulator is authoritative. Browsers and the bot model submit
  high-level intent; only Go resolves movement, combat, visibility,
  projectiles, Dodge, objectives, advantage, and terminal state.
- Tactical `ActionType` is exactly `move`, `hold`, `contest`, or `retreat`.
  Dodge is a separate two-charge reaction endpoint and must never become a
  tactical action or decision-tree branch. No fifth tactical command is
  supported.
- The browser must never call the model service or receive its endpoint.
- External model output is advisory and atomically validated. Any unavailable
  or unusable response activates Go fallback for the full turn.
- In no-model/fallback mode, the same fixture seed and tactical action sequence
  produces the same state apart from the generated session ID. Invalid input
  must leave state unchanged.
- The product has no runtime telemetry ingestion, telemetry UI/API, local
  telemetry persistence, or automatic scenario-publication path. Do not
  reintroduce one as incidental scope.
- The three fixtures are evidence-backed teaching adaptations, not synthetic
  telemetry and not replay-exact reconstructions. Their full-map unit, terrain,
  objective, turret, safe-zone, and target coordinates are **authored normalized
  approximations informed by reviewed minimap frames, not replay telemetry**.
- Describe simulator outcomes as deterministic counterfactual estimates, not
  factual match outcomes, calibrated win probabilities, optimal real-world
  decisions, or proof of a professional player's intent/style.
- Never add secrets, raw/proprietary match data, personal data, unlicensed game
  assets, or hidden model snapshots to browser-visible/logged state.

## Source and challenge attribution

Preserve the attribution in `README.md` and each fixture's `replayEvidence`.
The exact challenge title is **Garena AI Build Challenge 2026 — Case Brief**.
The supplied source bundle is `playable-replays-t1-blg-2024`, SHA-256
`07a8b2732dfb62e4d416e011bd4f5e0317a4bf38e84963d0030c14104cc07d1f`.
It identifies the media as Caedrel's edited upload of the 2024 Worlds Final and
cross-references Games of Legends IDs `62816..62820`, Riot's Worlds 2024 Primer,
and secondary analysis.

Do not call the source a Worlds 2026 match. Preserve the separation between
caption/event evidence, analyst assessment, coaching correction, and simulator
output. Names are contextual attribution and do not imply endorsement.

## Repository map and ownership

| Path | Responsibility |
| --- | --- |
| `backend/cmd/server/` | API startup, `BOT_MODEL_*` configuration, logging, and graceful shutdown |
| `backend/cmd/validate-fixtures/` | Complete fixture and executable acceptance validation |
| `backend/internal/api/` | Strict HTTP routing, JSON/error shapes, public filtering, session locks, and mutation rate limits |
| `backend/internal/engine/` | Authoritative deterministic rules, geometry, full-map turrets, projectiles, Dodge, bot validation/fallback, and reference search |
| `backend/internal/positionmodel/` | Historical package path for the schema `2.0` bot-action HTTP client; do not describe it as position-only |
| `backend/internal/model/` | Shared Go domain/public wire structs |
| `backend/internal/fixtures/` | Version `3.0`, `1..3` pack loading and semantic validation |
| `frontend/src/api.ts` | Browser's only HTTP client boundary |
| `frontend/src/types.ts` | Strict TypeScript mirror of the public API |
| `frontend/src/components/` | Accessible interaction/rendering components and colocated tests |
| `contracts/openapi.yaml` | Canonical public Go API and bot-model webhook contract |
| `contracts/moment.schema.json` | Draft 2020-12 schema for fixture version `3.0` |
| `fixtures/moments.json` | The three authored source-attributed scenarios |
| `docs/` | Current architecture, model, authoring, simulator, limitations, and decisions |
| `.github/workflows/ci.yml` | Go, frontend, and fixture validation |

The repository is split by responsibility: `main` is the stable app, `dev` is
the gameplay/interface development line, `ml` contains offline preparation and
training/fine-tuning, and `ml-inference` contains portable model artifacts and the
dependency-free inference service. Training and inference files are
intentionally absent from `dev`.

## Runtime flow

1. The Go API loads one to three version `3.0` moments.
2. React lists moments and creates an in-memory session.
3. React renders the entire normalized `0..100` map using the server's units,
   terrain, objective, six turrets, and projectiles.
4. React submits one of the four tactical actions to the turn endpoint.
5. Go validates the action, resolves pending projectiles, user intent, bot
   behavior, fog, combat, objective/escape state, and outcome.
6. When configured, Go posts a privileged schema `2.0` snapshot to the
   separately deployed `ml-inference` service before bot resolution. The
   service evaluates the exported linear policy and returns every live
   non-controlled unit's advisory action.
7. Go accepts the complete response atomically or applies fallback, then
   exposes `botControl` status in the session.
8. When a red projectile targets the controlled unit, React may call the
   separate Dodge endpoint. Go removes the eligible projectile and sidesteps
   without advancing the tactical turn.
9. Terminal sessions expose reference outcomes, the best-case four-command
   decision tree, causal timeline/logs, and debrief.

## Toolchain and commands

### Go backend

- Go `1.26.5`; module path
  `github.com/joyalzzy/playable-replays/backend`.
- Prefer the standard library. Run `go mod tidy` after dependency changes.
- Production builds use the hardening in `backend/Dockerfile`.
- Format with `gofmt`; validate with `go vet ./...` and `go test -race ./...`.

### TypeScript frontend

- Node 22, React 19, TypeScript 5, Vite 6, ECMAScript modules.
- Keep `strict`, `isolatedModules`, `noEmit`, and
  `noUncheckedIndexedAccess`; do not add `any` escapes.
- Use `npm ci` for reproducible checks. Commit `package-lock.json` with
  dependency changes; never commit `node_modules`, `dist`, coverage, or
  TypeScript build artifacts.

### Python inference service

- Python 3.12, standard library only at runtime, no upstream model call, and no
  API key.
- The portable model, `serve.py`, and export validation live on
  `ml-inference`; offline pipelines and notebooks live on `ml`.
- Use type hints and strict boundary validation. Never duplicate simulator
  rules in the inference service.

Run from the repository root unless noted:

| Intent | Command |
| --- | --- |
| All required checks | `make test` |
| Go tests | `make test-go` |
| Frontend type-check/tests/build | `make test-web` |
| Validate fixture pack and acceptance cases | `make validate-fixtures` |
| Start fallback/configured API | `make dev-api` |
| Start frontend | `make dev-web` |
| Build backend/frontend | `make build` |
| CI-equivalent Go checks | `cd backend && test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...` |
| Whitespace check | `git diff --check` |

## Environment variables

| Variable | Owner | Default/purpose |
| --- | --- | --- |
| `LISTEN_ADDR` | Go API | API defaults to `127.0.0.1:8080`; Compose sets it explicitly |
| `FIXTURE_PATH` | Go API | `../fixtures/moments.json` from `backend/`; Compose uses `/app/fixtures/moments.json` |
| `VITE_API_TARGET` | Vite | `http://127.0.0.1:8080`; Compose uses `http://api:8080` |
| `BOT_MODEL_URL` | Go bot client | Optional absolute HTTP(S) action endpoint, conventionally `http://127.0.0.1:9000/v1/actions` |
| `BOT_MODEL_NAME` | Go bot client | Required with URL; stable operator-owned policy name |
| `BOT_MODEL_VERSION` | Go bot client | Required with URL; rollout version shown on accepted turns |
`BOT_MODEL_URL`, `BOT_MODEL_NAME`, and `BOT_MODEL_VERSION` are all-or-nothing.
`BOT_MODEL_VERSION` is displayed provenance and must match the deployed export
or another accurate rollout ID.
Do not add compatibility aliases, local-data settings, or telemetry environment
variables. Never accept the model URL from the browser or a session request;
doing so would create an SSRF boundary.

## Public API contract

The canonical public description is `contracts/openapi.yaml`. Go structs in
`backend/internal/model/types.go` and TypeScript interfaces in
`frontend/src/types.ts` manually mirror it. A shape change is incomplete until
the contract, both mirrors, API client, UI, and focused tests agree.

All coordinates use normalized inclusive `0..100`. JSON names are `camelCase`.

| Method and path | Success | Purpose and principal errors |
| --- | --- | --- |
| `GET /healthz` | `200` | API liveness |
| `GET /api/v1/moments` | `200` | List the one-to-three moment summaries |
| `POST /api/v1/sessions` | `201 Session` | Create by `momentId`; `400 invalid_request`, `404 moment_not_found` |
| `GET /api/v1/sessions/{id}` | `200 Session` | Read public session state; `404 session_not_found` |
| `POST /api/v1/sessions/{id}/turns` | `200 Session` | Resolve one Move/Hold/Contest/Retreat; `400`, `404`, `422 illegal_action`, `429`, bounded `500` |
| `POST /api/v1/sessions/{id}/dodge` | `200 Session` | Evade one eligible projectile without advancing a turn; `404`, `422 dodge_unavailable`, `429`, bounded `500` |
| `POST /api/v1/sessions/{id}/reset` | `200 Session` | Reset seed/state under the same ID; `404`, `429` |

No capture or data-persistence routes are part of the product. Unknown routes
and unsupported methods use the same structured error schema:

```json
{"error":{"code":"stable_machine_code","message":"human-readable message"}}
```

Request bodies are capped at 64 KiB, reject unknown fields, require exactly one
JSON value, and require EOF. Preserve JSON content type, `nosniff`,
`no-referrer`, localhost-only development CORS, structured router 404/405, and
the `Allow` header on 405.

## Session and simulator invariants

`Session` includes full public state plus:

- `turrets`: exactly six canonical blue/red lane turrets;
- `projectiles`: marksman shots pending until the next tactical turn;
- `dodgeCharges`: starts at two;
- `dodgeAvailable`: server-computed eligibility; and
- `botControl`: `pending`, `external-model`, or `fallback`, with
  name/version only for accepted external results.

Only visible/alive red units may render. Hidden red units are omitted from
public `units`; hidden red projectile source IDs are redacted. React must not
reconstruct hidden state.

Turn order is: validate; reset defense/tick cooldowns; increment; resolve old
projectiles; resolve user; fog; request/validate bot actions; resolve allies;
fog; resolve enemies; record model/fallback; fog; objective/escape; advantage;
outcome/reference; Dodge availability/debrief. Reordering is a behavior change
that requires focused tests and doc/contract review.

Marksman attacks launch a one-turn projectile for half the target's maximum HP,
rounded up. Dodge removes an eligible projectile and performs a class-limited
automatic sidestep without turn/cooldown/objective/model progression. The
best-case/reference search branches over four tactical actions and invokes the
same Dodge reaction automatically when eligible.

Geometry and canonical turrets live in `backend/internal/engine/geometry.go`.
Do not make React or the model service a competing authority. Turrets are currently
visual landmarks only.

## Bot model contract

The inference service on `ml-inference` exposes:

| Method/path | Contract |
| --- | --- |
| `GET /healthz` | Liveness and deployed model version |
| `POST /v1/actions` | Strict schema `2.0` authoritative snapshot to complete advisory action array |

The snapshot has `schemaVersion: "2.0"`,
`stateScope: "authoritative_server_state"`, session/moment/turn keys, map
bounds, `controlledUnitId`, the exact four legal actions, optional objective,
pending projectiles, and authoritative units. It is privileged server state.

The response is:

```json
{
  "actions": [
    {"unitId":"blue-support","action":{"type":"hold"}},
    {"unitId":"red-marksman","action":{"type":"move","target":{"x":54,"y":43}}}
  ]
}
```

Integration rules:

- Require exactly one action for every live snapshot unit except
  `controlledUnitId`; reject duplicate, missing, unknown, controlled, or dead
  units atomically.
- Only Move has a complete finite in-bounds target; other actions have no
  target. Dodge is never legal model output.
- Cap backend request/response and service request/response bodies at their
  source-defined limits; preserve bounded timeouts and no retry.
- Apply class movement limits and every gameplay consequence only in Go.
- Timeout, non-200, malformed/oversized/incomplete output, or any validation
  failure uses fallback once.
- Record accepted actions with schema/model identity in session memory; clear
  records on reset. Do not claim durable replay without exporting them.
- Do not log API keys, privileged snapshots, or model outputs.
- Version schema changes and update the `ml-inference` service, connector,
  OpenAPI, tests, Compose, env example, README, and this guide together.

## Fixture invariants

- Top-level fixture version is `3.0`; the pack contains `1..3` moments.
- Current stable IDs are `resource-trade-932`, `positioning-1295`, and
  `teamfight-reversal-1727`.
- Every moment contains complete `replayEvidence`; `coordinateMethod` must
  explicitly include “approx”.
- IDs/slugs are unique lowercase hyphenated identifiers. `maxTurns` is `1..20`,
  signals are `0..1`, and points are inside `0..100`.
- Every moment has exactly five blue and five red units, exactly one marksman
  per team, and a live blue `controlled` unit.
- Classes must match canonical health/move/attack profiles; policies are
  controlled/support/protector/aggressive/skirmisher.
- Reference plan/reasons cover every turn. Defaults and full continuations
  exist for all four actions only.
- Authoring includes category, skill level, rationale, two tradeoffs, two
  distinct alternatives, and executable win/loss acceptance cases.
- Acceptance `dodgeBeforeTurns` has at most two unique valid one-based tactical
  turn numbers and calls the separate reaction before that turn.
- Scenario-specific mechanics such as `baron-pit` require a linked pre-play
  briefing.
- Keep JSON Schema, strict Go decoding/semantic validation, fixtures, and tests
  aligned. Schema validation does not replace engine-backed acceptance tests.

The historical highlight score may remain in summary metadata, but it is not a
calibrated confidence and no runtime detector is supported.

## Coding conventions

### Go

- Keep transport in `internal/api`, rules in `internal/engine`, wire/domain
  structs in `internal/model`, and fixture validation in `internal/fixtures`.
- Constructors establish valid state; return defensive copies of slices and
  pointer fields.
- Seed a private `rand.Rand`; never use global or wall-clock randomness for
  simulation.
- Fully validate before mutation. Preserve sentinel errors for HTTP mapping and
  do not expose internal error detail in `500` responses.
- Protect registry access and per-session mutations; never hold the global map
  mutex across network/model I/O.
- Use structured `slog`; do not log secrets or privileged snapshots.

### TypeScript and React

- Use function components/hooks, `import type`, shared types, and the central
  API client. Avoid ad hoc duplicate response interfaces.
- The backend is authoritative; frontend prevalidation is only user feedback.
- Preserve share URLs using `?moment=<slug>` and always render the server's full
  map/turrets/projectiles.
- Keep loading, busy, terminal, unavailable-Dodge, and error states explicit.
- Preserve semantic controls, keyboard/focus behavior, live status messaging,
  labelled ranges/tooltips, sufficient contrast, and reduced-motion behavior.
- Test with accessible roles/names and user behavior rather than CSS snapshots.

### Python

- Use `snake_case`, type hints, bounded parsing, deterministic validation, and
  no hidden retries.
- Keep inference-service tests network-free.
- Do not add a second engine, state store, or local fake model success.

## Change and validation matrix

| Change | Required checks and companion work |
| --- | --- |
| Go API/engine/model | `gofmt`, `go vet ./...`, `go test -race ./...`; update OpenAPI, TypeScript, client/UI, and tests for public changes |
| Frontend | `npm ci`, `npm run check`, `npm test -- --run`, `npm run build`; add accessible behavior tests |
| Model service/connector | Inference export checks plus Go connector/engine tests for success, timeout, malformed, oversized, completeness, identity, bounds, and fallback |
| Fixtures/contracts | Update schema/loader together; run validator, Go tests, frontend type-check, and relevant contract checks |
| Runtime/docs | `docker compose config`, YAML parse where available, `make -n`, stale-string search, and `git diff --check` |

## Git and definition of done

- Understand the worktree before editing. Never discard, stash, rebase, reset,
  or overwrite user/other-agent changes.
- Use focused changes and inspect the exact diff. Never stage generated output,
  dependencies, credentials, `.env`, logs, or unrelated work.
- Do not push/merge to `main`, force-push, or resolve review conversations
  without explicit authorization.
- Report only checks that actually ran and distinguish unrelated pre-existing
  failures.

A change is done only when implementation, contracts, types, tests, fixtures,
runtime configuration, and current docs agree; fallback remains deterministic;
attribution and authored-coordinate disclosure remain explicit; no
sensitive data was introduced; and the final diff is scoped and validated.
