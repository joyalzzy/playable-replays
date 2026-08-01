# Model and evaluation plan

The runnable prototype needs no model, GPU, API key, or network access. The
seeded built-in opponent policy remains the default.

## Highlight selection

Start with the included weighted baseline and an analyst-labelled validation
set. Measure precision at the number of moments a viewer would realistically
see per match, recall of known pivotal events, and calibration of the displayed
confidence. Add a small gradient-boosted model only after the baseline and label
quality are understood.

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

## Optional opponent-position model

`OPPONENT_MODEL_URL` enables a server-to-server adapter for experimenting with
opponent behavior. Once per accepted user turn, the Go API posts a snapshot with
the session and moment IDs, the next-frame `turn` index, map bounds, controlled
unit ID, and units. The versioned payload is marked as privileged authoritative
server state. The model returns only desired next-frame positions:

```json
{
  "positions": [
    {"unitId": "red-jungle", "position": {"x": 45, "y": 52}}
  ]
}
```

That response is a proposal, not state. The simulator accepts targets only for
eligible opponent units, clamps displacement using each class's `moveRange`,
keeps coordinates inside the map, and resolves every attack and outcome itself.
Missing targets use built-in behavior. Network, HTTP, decoding, or validation
failures must fail closed to the seeded deterministic policy rather than stall a
turn or partially hand authority to the service.

For a first learned connector, train the same compact trajectory policy to
predict a normalized next-position delta (or a tactical action plus target) for
each opponent from observable history. The serving adapter converts that output
to the OpenAPI `positions` response. Evaluate next-position error alongside:

- legal-response and class-range clamp rates;
- fallback/error rate and tail latency;
- multi-turn rollout stability and collision/path plausibility;
- action diversity, calibration, and analyst preference; and
- performance split by held-out player, patch, matchup, and game phase.

External model responses may be nondeterministic. Record the accepted response,
model/version identifier, and fixture/action sequence when exact replay is
required. Do not send proprietary, identity-bearing, or hidden telemetry to a
third-party service without authorization and an explicit retention policy.
The complete request and response schema is the `opponentModelTurn` webhook in
[`../contracts/openapi.yaml`](../contracts/openapi.yaml).

## Optional language layer

If free-text commands or coaching are added, fine-tune a permissively licensed
2B–4B instruction model with QLoRA. It should only emit validated command JSON
or explanations grounded in simulator output. Keep it outside the authoritative
simulation path.
