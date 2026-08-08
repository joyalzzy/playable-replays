# Bot model and evaluation plan

## Implemented model path

`model-daemon/` is an implemented Python 3.12 service, not a placeholder. For
each accepted tactical turn, the Go server can send it a privileged schema
`2.0` snapshot. The daemon makes a real OpenAI Responses API request with a
strict JSON Schema and returns one high-level action for every live unit except
the user-controlled unit.

There is no local model stub or simulated success path in the daemon. A missing
API key, upstream failure, invalid response, or refusal returns non-`200`; the
Go engine then applies its deterministic bot fallback. This preserves a
runnable no-key experience while keeping successful `external-model` status
truthful.

## Server-to-server contract

The Go API is configured with the all-or-nothing group:

- `BOT_MODEL_URL`, conventionally `http://127.0.0.1:9000/v1/actions`;
- `BOT_MODEL_NAME`, a stable operator-owned bridge/policy name; and
- `BOT_MODEL_VERSION`, the rollout identity shown with accepted results.

The request declares:

```json
{
  "schemaVersion": "2.0",
  "stateScope": "authoritative_server_state",
  "sessionId": "session-1",
  "momentId": "positioning-1295",
  "turn": 1,
  "mapBounds": {"minX": 0, "maxX": 100, "minY": 0, "maxY": 100},
  "controlledUnitId": "t1-faker",
  "legalActions": ["move", "hold", "contest", "retreat"],
  "projectiles": [],
  "units": [
    {
      "id": "t1-faker",
      "team": "blue",
      "role": "mid",
      "class": "mage",
      "fallbackPolicy": "controlled",
      "position": {"x": 36, "y": 62},
      "hp": 80,
      "maxHp": 95,
      "moveRange": 9,
      "attackRange": 24,
      "cooldownTurns": 0,
      "shield": 0,
      "guarded": false,
      "visible": true,
      "alive": true
    },
    {
      "id": "blg-knight",
      "team": "red",
      "role": "mid",
      "class": "mage",
      "fallbackPolicy": "aggressive",
      "position": {"x": 54, "y": 43},
      "hp": 87,
      "maxHp": 95,
      "moveRange": 9,
      "attackRange": 24,
      "cooldownTurns": 0,
      "shield": 0,
      "guarded": false,
      "visible": true,
      "alive": true
    }
  ]
}
```

`units` contains class, fallback policy, position, health, shield/guard state,
movement/attack ranges, cooldown, visibility, and alive state. Optional objective state and
pending projectiles provide the context needed for tactical choices. Because
the daemon acts for both teams, this is privileged authoritative state and must
never be exposed to a browser-selected endpoint.

The response is deliberately small:

```json
{
  "actions": [
    {
      "unitId": "blg-knight",
      "action": {"type": "move", "target": {"x": 52, "y": 45}}
    }
  ]
}
```

The model must return exactly one action for every live non-controlled unit.
Only `move` may include a target. Dodge is intentionally absent: it is a
user-only, two-charge server reaction and not part of bot inference or tactical
search.

## Validation and authority

The daemon validates request shape, size, schema version, state scope, map
bounds, legal actions, unit identities/classes/state, and the OpenAI response.
It requests strict Structured Outputs and rejects incomplete, duplicate,
unknown, illegal, refused, malformed, or oversized output.

Go validates the daemon response again. Validation is atomic: one bad or
missing action rejects the whole result. Move targets must be finite and inside
the normalized map; actual displacement is then capped by the unit's class
range. Go remains solely responsible for movement resolution, combat,
projectiles, fog, objective state, advantage, terminal state, and public-state
redaction.

The daemon does not retry. The Go client uses a bounded HTTP timeout and invokes
deterministic fallback once. `Session.botControl` reports `pending`, accepted
`external-model` with name/version, or `deterministic-fallback`.

## Prompt and model configuration

The checked-in `model-daemon/prompt.txt` defines the tactical role and instructs
the model to use only the supplied state and legal commands. Runtime variables
are:

| Variable | Default | Purpose |
| --- | --- | --- |
| `OPENAI_API_KEY` | none | Required for a successful model call. |
| `OPENAI_MODEL` | `gpt-5.6` | Responses API model ID. |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Operator-owned HTTPS API base path; cleartext HTTP is accepted only for a loopback test server. |
| `OPENAI_TIMEOUT_SECONDS` | `8` | Upstream deadline, bounded to `0.1..120` and below the Go client's nine-second deadline. |
| `LISTEN_ADDR` | `127.0.0.1:9000` | Daemon bind address; Compose uses `0.0.0.0:9000`. |

`BOT_MODEL_VERSION` is operator-supplied provenance rather than an upstream
claim. When `OPENAI_MODEL` changes, update `BOT_MODEL_VERSION` to the same
deployed model identifier (or to a distinct, accurate rollout identifier) in
the same configuration change.

The key must remain daemon-side. Never place it in React, a session request,
fixture, source file, log, screenshot, or committed `.env`.

## Evaluation gates

Unit tests use a local fake Responses API and require neither a key nor network.
They cover valid structured output, health, missing keys, invalid snapshots,
upstream errors, malformed/refused/incomplete output, and HTTP boundary rules.
The Go connector/engine tests must independently cover response size, malformed
JSON, unit completeness/uniqueness, target bounds, timeout, atomic rejection,
and deterministic fallback.

Before changing the model, prompt, or schema, report:

- complete legal-response rate and deterministic-fallback rate;
- p50/p95/p99 daemon and end-to-end turn latency;
- illegal, missing-unit, duplicate-unit, and out-of-bounds rejection counts;
- action distribution and multi-turn stability across all three fixtures;
- outcome/reference divergence under repeated model runs; and
- analyst preference on blinded tactical traces.

Model agreement is not evidence that the system reproduced a professional
player's decision process. Scenario advantage is an authored rules-based state
indicator, not a learned or calibrated win probability.

## Reproducibility and data policy

The engine stores accepted action arrays and configured model identity in
session-scoped memory. Exact replay after process exit additionally requires
exporting those arrays with the fixture and user actions; durable export is not
implemented. Treat model-backed runs as attributable but not durably
reproducible until that exists.

The source bundle provides reviewed video/caption/event evidence. Scenario map
coordinates are authored normalized approximations, not replay telemetry. Do
not send source media, proprietary data, personal data, or new identity-bearing
state to an external model without authorization and an explicit retention
policy.
