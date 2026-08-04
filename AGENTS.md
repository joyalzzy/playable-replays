<!-- markdownlint-disable MD013 -->

# Playable Replays agent guide

This file applies to the entire repository. A more deeply nested `AGENTS.md`
may add directory-specific rules, but it must not weaken the architecture,
contract, determinism, data-safety, or validation rules below.

## Mission and non-negotiable boundaries

Playable Replays is a shareable MOBA tactical-board prototype. It turns
authorized or synthetic telemetry into short, replayable decision scenarios.

- The Go simulator is authoritative. Browsers submit high-level actions; an
  optional position model submits advisory targets for live non-player units.
  Only the simulator resolves movement, damage, cooldowns, visibility, scoring,
  and terminal state.
- In the required no-model mode, the same fixture seed and legal action sequence
  must produce the same trajectory and outcome, apart from the generated session
  ID. Do not introduce wall-clock time, global randomness, or unordered-map
  dependence. A model-backed run is reproducible only when the accepted model
  response and model/version metadata are recorded.
- Invalid input must leave session state unchanged.
- Included fixtures are synthetic. Never add proprietary telemetry, secrets,
  personal data, or unlicensed game assets.
- Describe outcomes as deterministic counterfactual estimates, not factual
  reconstructions, optimal decisions, or proof of a professional player's
  intent or style.
- The web tactical board is the MVP. In-game engine integration is deferred and
  requires publisher approval.
- The runnable project must continue to work without a model, GPU, API key, or
  external network service.

## Repository map and ownership

| Path | Status | Responsibility |
| --- | --- | --- |
| `backend/cmd/server/` | Implemented | Process startup, environment configuration, structured logging, graceful shutdown. |
| `backend/internal/api/` | Implemented | HTTP routing, JSON transport, status codes, CORS/security headers, and in-memory session coordination. |
| `backend/internal/engine/` | Implemented | Deterministic authoritative simulator, non-player position integration, and opponent fallback policy. Keep game rules here, not in handlers or React. |
| `backend/internal/model/` | Implemented | Shared Go domain and wire structs with JSON tags. |
| `backend/internal/fixtures/` | Implemented | Versioned synthetic fixture loading and runtime validation. |
| `frontend/src/api.ts` | Implemented | The browser's only HTTP client boundary. |
| `frontend/src/types.ts` | Implemented | TypeScript mirror of the public API schema. |
| `frontend/src/components/` | Implemented | Presentational and interaction-focused React components plus colocated tests. |
| `contracts/openapi.yaml` | Implemented | Public Go API contract. Update it with every public request or response change. |
| `contracts/moment.schema.json` | Implemented | Draft 2020-12 schema for fixture files. |
| `fixtures/moments.json` | Implemented | Version `1.0` synthetic replay moments. |
| `ml/` | Implemented baseline | Offline highlight scoring, future model-training/evaluation code, and Python tests. It is never imported by the live Go server. |
| `model-daemon/` | Reserved; not on `main` yet | Future optional online inference service. It must expose the versioned model contract below and remain non-authoritative. Do not create a second simulator here. |
| `docs/` | Implemented | Architecture, limitations, model plan, and decisions that outgrow this file. |
| `.github/workflows/ci.yml` | Implemented | Go, frontend, and offline-ML validation on pushes to `main`, on PRs, and by manual dispatch. |

Keep offline training in `ml/`; do not create a parallel `model-training/`
tree unless an intentional migration updates imports, commands, CI, Docker,
README, and this file together. When `model-daemon/` is introduced, keep model
loading/serving there and the connector/interface in the Go backend.

Add these subdirectories only when their implementation exists; do not commit
empty scaffolding:

```text
ml/
  ingest/       # authorized/synthetic telemetry readers
  features/     # versioned feature extraction
  train/        # offline training entry points and configs
  evaluate/     # held-out and rollout evaluation
  export/       # manifests and export code, not large weights
  tests/
model-daemon/
  src/          # HTTP serving and model adapter
  tests/        # contract, timeout, and fallback tests
```

## Intended runtime flow

The current prototype begins with hand-authored synthetic fixtures; telemetry
ingestion and automatic fixture generation are future work.

1. Authorized or synthetic telemetry is processed offline in `ml/`.
2. Selected windows become versioned fixtures under `fixtures/`.
3. The Go API loads fixtures and creates an in-memory deterministic session.
4. React sends one legal high-level action per turn.
5. The Go engine validates and resolves the turn and returns a full session
   snapshot for this synthetic prototype.
6. A future model daemon may suggest teammate and opponent targets, but the
   engine rejects control of the user's unit and validates and clamps every
   accepted target before applying normal simulation rules.

The browser must never call the model daemon directly. The model endpoint is an
operator-configured server-to-server dependency, and its failure must activate
a deterministic built-in fallback rather than fail the user's turn.

## Toolchain and dependency policy

### Go backend

- Go `1.26.5`; module path:
  `github.com/joyalzzy/playable-replays/backend`.
- The current module uses only the standard library. Prefer the standard
  library and add a module only when it materially reduces risk or complexity.
- Use `go mod tidy` after dependency changes and commit both `go.mod` and
  `go.sum` when a sum file is created.
- Format with `gofmt`; validate with `go vet ./...` and `go test -race ./...`.
- Production builds use `CGO_ENABLED=0`, `-trimpath`, and stripped symbols in
  `backend/Dockerfile`.

### TypeScript frontend

- TypeScript `^5.7.3` (currently locked to `5.9.3`), React `19`, Vite `6`, Node
  `22` in CI/Docker, and ECMAScript modules (`"type": "module"`).
- TypeScript is required for frontend work unless a documented technical
  limitation makes it impossible.
- `strict`, `isolatedModules`, `noEmit`, and `noUncheckedIndexedAccess` are
  enabled. Do not weaken these settings or introduce untyped `any` escapes.
- Runtime modules: `react`, `react-dom`.
- Build modules: `typescript`, `vite`, `@vitejs/plugin-react`.
- Test/lint modules: Vitest, jsdom, Testing Library, ESLint, React Hooks, and
  React Refresh plugins.
- Use `npm ci` for reproducible validation. Commit `package-lock.json` with any
  dependency change; never commit `node_modules/`, `dist/`, or coverage output.

### Offline ML and training

- Python `3.11+`; CI uses Python `3.12` and `unittest`.
- The current baseline is standard-library-only and deterministic. Avoid
  import-time network access, downloads, or training side effects.
- Put preprocessing, training, evaluation, and export code under `ml/` with
  tests under `ml/tests/`. Keep online serving code in `model-daemon/`.
- Do not commit raw replay shards, caches, checkpoints, or large model weights.
  Store only small synthetic fixtures, versioned metadata, and reproducible
  configuration.
- Treat `maknee/league-of-legends-decoded-replay-packets` only as a telemetry
  bootstrap corpus. It is not sufficient by itself to verify named pro-player
  imitation, roles, optimal action ranking, or player identity.

### Operations

- Docker Compose runs `api` and `web`, bound to loopback by default.
- The API listens on `127.0.0.1:8080`; Vite listens on `0.0.0.0:5173` in its
  development container and proxies `/api` and `/healthz` to the API.
- Never commit `.env` files. Add documented placeholders to `.env.example` when
  new configuration is introduced.

## Commands

Run commands from the repository root unless noted.

| Intent | Command |
| --- | --- |
| Aggregate tests and builds | `make test` |
| Go unit/API tests | `make test-go` |
| Offline ML tests | `make test-ml` |
| Frontend install, type-check, tests, and build | `make test-web` |
| Start API | `make dev-api` |
| Start Vite frontend | `make dev-web` |
| Build API and frontend | `make build` |
| Format Go | `cd backend && gofmt -w .` |
| CI-equivalent Go checks | `cd backend && test -z "$(gofmt -l .)" && go vet ./... && go test -race ./...` |
| Offline scorer smoke run | `python3 -m ml.highlight` |
| Pre-PR whitespace check | `git diff --check` |

After `gofmt -w .`, run `git diff --check` and inspect the diff. Formatting must
not be the only reason unrelated files change.

## Environment variables

| Variable | Owner | Default/purpose |
| --- | --- | --- |
| `LISTEN_ADDR` | Go API | `127.0.0.1:8080`; HTTP listen address. |
| `FIXTURE_PATH` | Go API | `../fixtures/moments.json` when run from `backend/`; Compose uses `/app/fixtures/moments.json`. |
| `VITE_API_TARGET` | Vite dev server | `http://127.0.0.1:8080`; Compose uses `http://api:8080`. |
| `OPPONENT_MODEL_URL` | Go connector | Optional absolute HTTP(S) URL, conventionally `http://127.0.0.1:9000/v1/positions`. Absence means deterministic built-in policy. |
| `OPPONENT_MODEL_NAME` | Go connector | Required with `OPPONENT_MODEL_URL`; stable operator-owned model name stored with accepted suggestions. |
| `OPPONENT_MODEL_VERSION` | Go connector | Required with `OPPONENT_MODEL_URL`; stable operator-owned model version stored with accepted suggestions. |

Configuration for a model URL is operator-owned. Never accept it from a browser
request or use arbitrary user-supplied URLs; that would create an SSRF boundary.

## Public API contract

The canonical public description is `contracts/openapi.yaml`. Go JSON structs
in `backend/internal/model/types.go` and TypeScript types in
`frontend/src/types.ts` are manual mirrors. A public shape change is incomplete
until all three, the frontend client, and relevant tests are updated together.

All coordinates use a normalized inclusive `0..100` map. JSON field names are
`camelCase`; Go names are idiomatic `PascalCase` with explicit JSON tags.

| Method and path | Success | Request and purpose | Errors |
| --- | --- | --- | --- |
| `GET /healthz` | `200 {"status":"ok"}` | Liveness check. | None in contract. |
| `GET /api/v1/moments` | `200 {"moments":[MomentSummary...]}` | List playable moment metadata and bounded highlight scores. | None in contract. |
| `POST /api/v1/sessions` | `201 Session` | `{"momentId":"<id>"}` creates an in-memory session. | `400 invalid_request`, `404 moment_not_found`. |
| `GET /api/v1/sessions/{id}` | `200 Session` | Read current session state. | `404 session_not_found`. |
| `POST /api/v1/sessions/{id}/turns` | `200 Session` | `{"action":{"type":"...","target":{"x":0,"y":0}}}` resolves one turn. Runtime requires `target` for `move`; the client omits it otherwise, although `main` currently accepts and ignores non-move targets. | OpenAPI: `400 invalid_request`, `404 session_not_found`, `422 illegal_action`. The handler also has an undocumented `500 simulation_error` path. |
| `POST /api/v1/sessions/{id}/reset` | `200 Session` | Reset to the fixture seed while preserving the session ID. No request body is required; the current client sends `{}`. | `404 session_not_found`. |

Current action values on `main` are `move`, `hold`, `contest`, and `retreat`.
Whenever an action is added or changed, update the engine validation/resolution,
reward/reference policy, logs, OpenAPI enum, Go/TypeScript types, action UI,
fixtures as needed, and backend/frontend tests in the same change.

Application errors use:

```json
{"error":{"code":"stable_machine_code","message":"human-readable message"}}
```

Defined request bodies are capped at 64 KiB and reject unknown fields. They are
required to contain exactly one JSON value; the current decoder's trailing-data
check is incomplete and is listed below as a gap. Preserve
`application/json; charset=utf-8`,
`X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and the
localhost-only development CORS allowlist. New router-level 404/405 handling
should also use the structured error shape rather than Go's default text body.

Simulator constants live in `backend/internal/engine/engine.go`; do not duplicate
or override them in React or the model daemon. Cooldowns are turns/frames, never
seconds. The resolution order is validation, turn increment, user action,
non-player position/opponent response, cooldown tick, fog update, outcome
update, then reference action selection. Reordering it is a behavior change and
needs focused tests.

## Model daemon contract

There is no online model service or model endpoint on current `main`. The
following is the reserved version `1.0` contract for `model-daemon/`; do not
invent a competing path or let the frontend depend on it.

Keep the four model-related roles separate:

- `ml/highlight.py` is the current offline heuristic highlight scorer.
- A future trajectory/action policy is trained and evaluated offline in `ml/`.
- `model-daemon/` serves optional next-frame non-player position inference.
- A future language layer may parse commands or explain simulator output, but it
  may only emit validated action JSON and is never part of game resolution.

| Method and path | Contract |
| --- | --- |
| `GET /healthz` | Return `200 {"status":"ok"}` without loading user/session data. |
| `POST /v1/positions` | Accept one authoritative next-frame snapshot and return advisory targets for live non-player teammates and opponents. |

The backend is configured with the complete `OPPONENT_MODEL_URL` plus required
`OPPONENT_MODEL_NAME` and `OPPONENT_MODEL_VERSION` identity values, so
deployments may route the service differently while retaining `/v1/positions`
as the local and documented convention.

Request shape:

```json
{
  "schemaVersion": "1.0",
  "stateScope": "authoritative_server_state",
  "sessionId": "session-1",
  "momentId": "objective-steal-742",
  "turn": 1,
  "mapBounds": {"minX": 0, "maxX": 100, "minY": 0, "maxY": 100},
  "controlledUnitId": "blue-carry",
  "units": [
    {
      "id": "red-jungle",
      "team": "red",
      "role": "jungler",
      "class": "assassin",
      "position": {"x": 48, "y": 51},
      "hp": 55,
      "maxHp": 100,
      "moveRange": 13,
      "attackRange": 12,
      "cooldownTurns": 0,
      "visible": true,
      "alive": true
    }
  ]
}
```

Response shape:

```json
{
  "positions": [
    {"unitId": "blue-support", "position": {"x": 26, "y": 63}},
    {"unitId": "red-jungle", "position": {"x": 55, "y": 51}}
  ]
}
```

Model integration rules:

- Treat the request as privileged server state; never forward it to the browser
  or an untrusted/user-selected endpoint.
- Suggestions may target any live unit except `controlledUnitId`, including
  teammates and opponents. Reject duplicate, unknown, dead, or controlled units,
  missing fields, unknown fields, non-finite coordinates, and coordinates
  outside map bounds.
- Omitted teammates hold position. Omitted opponents use the seeded built-in
  policy. An invalid response applies no model targets, leaves teammates
  stationary, and activates that deterministic opponent fallback.
- Cap request/response bodies at 64 KiB and snapshots/suggestions at 64 units.
- Use a short bounded timeout (the connector target is 1.5 seconds), HTTP(S)
  only, and deterministic fallback for timeouts, non-2xx responses, malformed
  JSON, oversized bodies, or unusable suggestions.
- Clamp accepted displacement to the unit's server-owned per-frame movement
  limit. A model may not author HP, damage, cooldowns, visibility, score, legal
  actions, win probability, or terminal state.
- Version request/response schema changes. Update the connector, daemon,
  OpenAPI webhook/component schemas, tests, Compose configuration, `.env.example`,
  README, and this file together.
- Store the accepted response, including teammate and opponent targets, plus
  model name/version in server-side rollout metadata; these fields do not
  belong in the strict position response. The current engine keeps
  session-scoped in-memory records and clears them on
  reset. Never claim durable action-sequence reproducibility until records are
  exported with the fixture and user actions.

If an inference framework requires Python, isolate it in `model-daemon/` behind
this HTTP contract. Keep the application backend and authoritative simulator in
Go. Prefer a small task-specific trajectory policy; a 2B-4B language model is
only appropriate for optional command parsing or explanations and must emit
validated action JSON outside the authoritative path.

## Contract and fixture invariants

- Fixture files use top-level version `1.0`; reject unknown versions.
- Moment IDs are stable and include event kind plus start window, such as
  `objective-steal-742`. Slugs use lowercase letters, digits, and hyphens.
- `controlledUnitId` must identify an included live unit. IDs must be unique;
  each moment must contain at least two units and one or more reason tags.
- `maxTurns` is `1..20`; signal values are normalized to `0..1`; coordinates
  are `0..100`; health/cooldown values cannot be negative.
- Keep the JSON Schema, loader validation, fixtures, and fixture tests aligned.
  Schema validation alone does not replace semantic validation in Go.
- Hidden enemies must not render in React. The current synthetic prototype still
  serializes their coordinates for inspectability, so it is not anti-cheat
  hardened. A production view must omit hidden coordinates entirely.
- Session status values are `active`, `won`, and `lost`; log actors are `user`
  and `policy` unless the public contract is deliberately versioned.

The highlight score is intentionally duplicated in Go and Python and must stay
identical:

```text
0.45 * winProbabilitySwing
+ 0.20 * eventDensity
+ 0.20 * (1 - entityProximity)
+ 0.15 * resourceAsymmetry
```

Clamp the result to `0..1`. If the formula, feature direction, thresholds, or
reason-tag vocabulary changes, update Go, Python, tests, fixtures, OpenAPI where
applicable, and explanatory UI/docs in one PR.

## Coding conventions

### Go

- Keep transport in `internal/api`, deterministic rules in `internal/engine`,
  wire/domain structs in `internal/model`, and fixture concerns in
  `internal/fixtures`.
- Use constructors to establish valid state. Return defensive copies of slices
  and pointer fields from engine state.
- Seed a private `rand.Rand` from the fixture. Do not use the global RNG.
- Validate an action completely before incrementing the turn or mutating a unit.
- Wrap errors with useful context and preserve sentinel errors for status-code
  mapping. Do not expose internal details in `500` responses.
- Protect the session map and per-session mutations consistently. Add race tests
  when concurrency behavior changes.
- Do not hold the global session-registry mutex across model/network I/O. Fetch
  the session safely, then use per-session synchronization for its mutation.
- Use structured `slog`; do not log secrets, full privileged model snapshots, or
  raw proprietary telemetry.

### TypeScript and React

- Use function components and hooks. Keep API calls in `src/api.ts`, shared
  public shapes in `src/types.ts`, and reusable UI in `src/components/`.
- Import types with `import type`. Avoid duplicated ad hoc response interfaces.
- The UI may prevalidate for feedback, but the backend remains the authority.
  Never implement damage, scoring, legality, or model fallback only in React.
- Preserve stable share URLs using the `?moment=<slug>` query parameter.
- Render only `visible && alive` units. Keep loading, busy, terminal, and error
  states explicit and accessible.
- Prefer semantic controls, keyboard/focus support, labelled ranges/tooltips,
  and Testing Library queries by role/name over implementation selectors.
- Keep CSS class naming component-oriented (the existing styles use BEM-like
  names) and test behavior rather than snapshots of styling details.

### Python and models

- Use `snake_case`, type hints, immutable dataclasses where appropriate, and
  pure functions for scoring/selection.
- Validate normalized inputs at the boundary. Make ordering deterministic with
  explicit tie-breakers.
- Separate dataset ingestion from feature extraction, training, evaluation, and
  export. Record dataset version/patch, split strategy, seed, feature schema,
  and metrics with every trained artifact.
- Evaluate held-out players, matches, patches, and matchups only when stable
  identifiers actually exist. Otherwise label results as generic trajectory
  modeling, not player-specific style reproduction.
- Begin with behavioral cloning and report top-k action agreement, calibration,
  rollout stability, policy diversity, and analyst preference. Agreement is not
  proof that a model reproduced a professional player's decision process.

## Known `main` gaps: do not canonize these

These are current prototype limitations, not conventions to copy into new code:

- Unknown paths and wrong methods use Go's plain-text 404/405 even though the
  middleware labels responses as JSON. New routing work should make these
  structured and test them.
- Go zero-value decoding does not fully enforce OpenAPI-required fields; partial
  points and empty semantic requests need stricter presence validation. The
  trailing-JSON check also needs a regression test that requires `io.EOF`.
- Runtime fixture validation covers fewer invariants than the JSON Schema, and
  CI does not currently run OpenAPI or JSON Schema validation.
- The reset client sends `{}` although the endpoint declares no body.
- The frontend trusts successful JSON through TypeScript casts; there is no
  generated client or runtime response validator.
- `npm run lint` exists, but no ESLint configuration is committed and lint is not
  a Make/CI gate. Do not report lint as passing until that is deliberately fixed.
- `make build` writes `backend/server`, which is not currently ignored. Treat it
  as generated output and never stage it; a future cleanup should build into an
  ignored output directory or extend `.gitignore`.
- Sessions are process-local, sequentially identified, unauthenticated, and not
  rate-limited. Durable storage, authentication, abuse protection, and production
  CORS are deferred production work.

## Change and validation matrix

| Change touches | Required checks and companion updates |
| --- | --- |
| Go API/engine/model | `gofmt`, `go vet ./...`, `go test -race ./...`; update OpenAPI, TypeScript types/client, and API tests for public changes. |
| Frontend | `npm ci`, `npm run check`, `npm test -- --run`, `npm run build`; add Testing Library coverage for changed behavior. |
| Fixtures/contracts | Update schema and loader together; run Go fixture/API tests, frontend type-check if public shapes changed, and `python3 -m ml.highlight`. |
| ML/training | `python3 -m unittest discover -s ml/tests -v` and a deterministic `python3 -m ml.highlight` smoke run; document new dependencies/artifacts. |
| Model daemon/connector | Unit-test both sides plus timeout, malformed, oversized, duplicate, unknown-unit, out-of-bounds, and deterministic-fallback cases; run a live mock-daemon journey. |
| Docs-only | `git diff --check`; verify every path, command, endpoint, version, and environment variable against source. |

Before declaring work complete, run the narrow tests while iterating and then
the full relevant suite. A public contract change is not complete with only one
language's tests passing.

## Git and pull-request hygiene

- Inspect a clean or fully understood worktree before branching. Never discard,
  stash, rebase, or overwrite user changes without explicit approval.
- Fetch first and start new work from `origin/main`, not an unrelated open PR,
  unless the user explicitly asks for stacked work.
- Use `agent/<short-description>` branches. Keep one coherent concern per PR.
- Inspect `git status` and the exact diff before staging; never include unrelated
  user changes, generated output, dependencies, logs, `.env`, or credentials.
- Review `git diff --cached` before committing. Never force-push unless the user
  explicitly authorizes rewriting that exact branch.
- Use a terse imperative commit subject and open a draft PR by default.
- The PR body must explain what changed, why, impact/boundaries, and the exact
  validation performed. Never claim checks that did not run.
- Do not push directly to or merge remote `main`, and do not resolve review
  conversations, unless the user explicitly asks.

## Definition of done

A change is done only when implementation, contracts, tests, fixtures,
configuration, and documentation agree; deterministic/no-model behavior still
works; sensitive or proprietary data has not been introduced; relevant local
checks pass; and the final diff contains only the intended scope.
