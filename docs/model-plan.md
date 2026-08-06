# Model and evaluation plan

The runnable prototype needs no model, GPU, API key, or network access. The
seeded built-in non-player policies remain the default.

The current `advantage` field is a rules-based summary of simulator state, not
a learned or calibrated probability. It must not be relabelled as win
probability without a representative labelled dataset and calibration study.

## Highlight selection

`ml.telemetry` now provides the deterministic preprocessing baseline: strict
normalized-frame validation, fully covered sliding windows, derivation of the
four canonical signals, and overlap suppression. Source-specific authorized
replay adapters and automatic scenario-fixture generation remain future work.
The contract and exact calculations are documented in
[`telemetry-highlights.md`](telemetry-highlights.md).

Evaluate the weighted baseline on whole matches, not independently curated
clips. Measure precision at the number of moments a viewer would realistically
see per match, recall of known pivotal events, duplicates after overlap
suppression, and calibration of any displayed confidence. Tune thresholds and
event-rate saturation only on a training split, then report held-out results by
match and patch. Add a small gradient-boosted model only after the baseline,
label quality, and leakage risks are understood.

## Counterfactual policy

Train a compact 10–30M parameter sequence policy over normalized telemetry
trajectories. Inputs should include observable positions, resources, cooldowns,
objectives, vision, time, matchup, and the user's legal action set. The output
is a distribution over high-level legal actions plus parameters such as a map
target.

Use behavioral cloning first, then evaluate held-out players, patches, and
matchups. Report top-k action agreement, calibration, rollout stability,
policy diversity, and analyst preference. Do not call policy agreement
"reproducing a pro player's decision."

## Optional non-player position model

`POSITION_MODEL_URL` enables a server-to-server adapter for experimenting with
teammate and opponent behavior. Enabling it also requires stable
`POSITION_MODEL_NAME` and `POSITION_MODEL_VERSION` values. The deprecated
`OPPONENT_MODEL_*` aliases remain available as an all-or-nothing
environment-name compatibility group; their endpoint must still implement
position-model schema `1.1`. Once per accepted user turn, the Go API posts
a snapshot with the session and moment IDs, the current-frame `turn` index, map
bounds, controlled unit ID, and units. The versioned payload is marked as
privileged authoritative server state. The model returns only desired
next-frame positions:

```json
{
  "positions": [
    {"unitId": "red-jungle", "position": {"x": 45, "y": 52}}
  ]
}
```

That response is a proposal, not state. Under connector schema `1.1`, the
simulator accepts targets for any live snapshot unit except the user-controlled
unit, including allied teammates. It validates the whole response before
movement, clamps displacement using each class's `moveRange`, keeps coordinates
inside the map, and resolves every attack and outcome itself. Missing teammates
hold position; missing opponents use built-in chase behavior. Network, HTTP,
decoding, or validation failures must fail closed to the seeded deterministic
policy rather than stall a turn, partially move a team, or hand authority to
the service.

For a first learned connector, train the same compact trajectory policy to
predict a normalized next-position delta (or a tactical action plus target) for
each AI-controlled teammate and opponent from observable history. The serving
adapter converts that output
to the OpenAPI `positions` response. Evaluate next-position error alongside:

- legal-response and class-range clamp rates;
- fallback/error rate and tail latency;
- multi-turn rollout stability and collision/path plausibility;
- action diversity, calibration, and analyst preference; and
- performance split by held-out player, patch, matchup, and game phase.

External model responses may be nondeterministic. The engine keeps validated
responses and configured model identity in session-scoped memory; exact replay
after process exit still requires durable export with the fixture/action
sequence. Do not send proprietary, identity-bearing, or hidden telemetry to a
third-party service without authorization and an explicit retention policy.
The complete request and response schema is the `positionModelTurn` webhook in
[`../contracts/openapi.yaml`](../contracts/openapi.yaml).

## Optional language layer

If free-text commands or coaching are added, fine-tune a permissively licensed
2B–4B instruction model with QLoRA. It should only emit validated command JSON
or explanations grounded in simulator output. Keep it outside the authoritative
simulation path.
