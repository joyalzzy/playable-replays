# Playable Replays unit-policy inference export

This bundle exports the committed `unit-policy-v2-carry-safety` policy from the
`agent/ml-unit-policy` branch of `joyalzzy/playable-replays`.

The model is a transparent linear policy with:

- 72 ordered normalized input features;
- four action heads: `move`, `hold`, `contest`, and `retreat`;
- two movement regression heads: normalized `dx` and `dy`;
- softmax action probabilities; and
- `tanh`-bounded movement output, normalized to unit length when necessary.

The policy is advisory. The Playable Replays Go simulator remains authoritative
for movement limits, target legality, combat, projectiles, objectives, and final
state.

## Files

- `unit_policy_v2_carry_safety.json`: canonical portable model artifact.
- `unit_policy_v2_carry_safety.npz`: NumPy arrays for direct numerical loading.
- `feature_names.json`: exact 72-feature order.
- `infer.py`: dependency-free snapshot and feature-vector inference CLI.
- `serve.py`: dependency-free HTTP server implementing `GET /healthz` and
  `POST /v1/actions`.
- `infer_features.mjs`: Node.js feature-vector inference example.
- `example_snapshot.json`: complete schema 2.0 input.
- `example_output.json`: expected Python CLI output for the example marksman.
- `validate_export.py`: JSON/NPZ consistency and inference smoke test.
- `manifest.json`: source commit, hashes, shapes, and model metadata.

## Run snapshot inference

```bash
python3 infer.py example_snapshot.json --unit blue-marksman
```

Return only the backend-compatible wire response:

```bash
python3 infer.py example_snapshot.json --wire-only
```

Input may also be read from stdin by using `-` as the path.

## Run as the model endpoint

```bash
python3 serve.py --listen 127.0.0.1:9000
```

Configure the Go backend with:

```text
BOT_MODEL_URL=http://127.0.0.1:9000/v1/actions
BOT_MODEL_NAME=playable-replays-linear-unit-policy
BOT_MODEL_VERSION=unit-policy-v2-carry-safety
```

Health check:

```bash
curl http://127.0.0.1:9000/healthz
```

Inference request:

```bash
curl -sS \
  -H 'Content-Type: application/json' \
  --data-binary @example_snapshot.json \
  http://127.0.0.1:9000/v1/actions
```

## Feature-vector inference

A feature input is either a JSON array of 72 numbers or:

```json
{"features": [1.0, 0.0, 0.5]}
```

with all 72 values supplied in the order in `feature_names.json`.

```bash
python3 infer.py features.json --mode features
node infer_features.mjs features.json
```

## Load the NPZ weights

```python
import numpy as np

weights = np.load("unit_policy_v2_carry_safety.npz")
action_weights = weights["action_weights"]      # shape: (4, 72)
movement_weights = weights["movement_weights"]  # shape: (2, 72)
action_order = weights["action_order"].tolist()
feature_names = weights["feature_names"].tolist()
```

For a batch `X` with shape `(N, 72)`, action logits are
`X @ action_weights.T`. Apply a row-wise softmax. Movement pre-activations are
`X @ movement_weights.T`; apply `tanh`, then normalize rows whose L2 norm is
greater than one.

## Validate

```bash
python3 validate_export.py
```

The JSON inference runtime uses only the Python standard library. NumPy is
needed only for reading the optional NPZ export or running `validate_export.py`.
