# Bot model and evaluation plan

## Implemented model path

The model is a separately trained/fine-tuned unit policy, not an upstream API bridge. Its
portable runtime is maintained on the `ml-inference` branch because model
artifacts and inference tooling are intentionally separate from the app on
`main` and `dev`. App branches pin that runtime at `ml-inference/` as a Git
submodule.

The exported `unit-policy-v2-carry-safety` model is a transparent linear policy
with 72 ordered normalized features, four action heads (`move`, `hold`,
`contest`, and `retreat`), and two movement-regression heads. The
standard-library `serve.py` process loads the committed JSON artifact locally;
it makes no upstream network call and requires no API key.

The `ml` branch contains offline replay preparation, training pipelines,
notebooks, and their tests. The `ml-inference` branch contains only the portable
artifacts and runtime needed for deployment. Its `manifest.json` records the
source commit, artifact hashes, feature order, training metadata, and validation
results.

## Server-to-server contract

The Go API is configured with the all-or-nothing group:

- `BOT_MODEL_URL`, conventionally `http://127.0.0.1:9000/v1/actions`;
- `BOT_MODEL_NAME`, normally `playable-replays-linear-unit-policy`; and
- `BOT_MODEL_VERSION`, currently `unit-policy-v2-carry-safety`.

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

The service returns exactly one action for every live non-controlled unit:

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

Only `move` may include a target. Dodge is a separate user reaction and is not
part of model inference or tactical search. Because the policy acts for both
teams, the request contains privileged authoritative state and must never be
sent to a browser-selected endpoint.

## Validation and authority

The inference service validates body size, schema version, state scope, map
bounds, legal actions, and unit state before evaluating the model. Go validates
the response again. Validation is atomic: one bad or missing action rejects the
whole result. Move targets must be finite and inside the normalized map; actual
displacement is then capped by the unit's server-owned class range.

Go remains solely responsible for movement resolution, combat, projectiles,
fog, objective state, advantage, terminal state, and public-state redaction. A
timeout, transport failure, non-`200`, malformed or oversized response,
incomplete unit set, illegal action, or invalid target activates fallback once
for the whole turn. For the same fixture seed and tactical action sequence,
fallback behavior is deterministic. `Session.botControl` reports `pending`,
accepted `external-model` with name/version, or `fallback`.

## Run and validate the model

Initialize the pinned submodule from the repository root:

```bash
git submodule update --init --recursive
```

Then start the endpoint from the submodule:

```bash
cd ml-inference
python3 serve.py --listen 127.0.0.1:9000
```

The runtime uses only the Python standard library. Validate the JSON/NPZ export
and smoke-test inference with:

```bash
python3 validate_export.py
python3 infer.py example_snapshot.json --wire-only
```

NumPy is needed only to read the optional NPZ artifact or run export
validation. The inference branch README documents Python, Node.js, HTTP, and
Docker usage.

## Evaluation gates

The committed export records deterministic group-isolated training and
validation metadata, feature count and order, action accuracy/recall, movement
error, source composition, leakage checks, and Python/Node parity. Before
replacing the model or feature contract, also report:

- complete legal-response rate and fallback rate;
- p50/p95/p99 inference and end-to-end turn latency;
- illegal, missing-unit, duplicate-unit, and out-of-bounds rejection counts;
- action distribution and multi-turn stability across all three fixtures;
- outcome/reference divergence across the fixture suite; and
- analyst preference on blinded tactical traces.

Model agreement is not evidence that the system reproduced a professional
player's decision process. Scenario advantage is an authored rules-based state
indicator, not a learned or calibrated win probability.

## Reproducibility and data policy

The engine stores accepted action arrays and configured model identity in
session-scoped memory. Exact replay after process exit additionally requires
exporting those arrays with the fixture and user actions; durable export is not
implemented.

The source bundle provides reviewed video/caption/event evidence. Scenario map
coordinates are authored normalized approximations, not replay telemetry.
Professional replay labels are observational associations, not proof of optimal
play or player intent. Do not add source media, proprietary data, personal data,
or hidden model snapshots to browser-visible or logged state.
