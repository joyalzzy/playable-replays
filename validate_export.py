#!/usr/bin/env python3
from __future__ import annotations

import json
import math
from pathlib import Path

import numpy as np

from infer import FEATURE_NAMES, infer_snapshot, load_model

ROOT = Path(__file__).parent
model = load_model(ROOT / "unit_policy_v2_carry_safety.json")
npz = np.load(ROOT / "unit_policy_v2_carry_safety.npz")
assert npz["action_weights"].shape == (4, 72)
assert npz["movement_weights"].shape == (2, 72)
assert np.array_equal(npz["action_order"], np.asarray(model["actionOrder"]))
assert np.array_equal(npz["feature_names"], np.asarray(FEATURE_NAMES))
for index, action in enumerate(model["actionOrder"]):
    assert np.allclose(npz["action_weights"][index], model["actionWeights"][action], rtol=0, atol=0)
assert np.allclose(npz["movement_weights"][0], model["movementWeights"]["dx"], rtol=0, atol=0)
assert np.allclose(npz["movement_weights"][1], model["movementWeights"]["dy"], rtol=0, atol=0)

snapshot = json.loads((ROOT / "example_snapshot.json").read_text())
result = infer_snapshot(model, snapshot, "blue-marksman")
decision = result["decisions"][0]
assert decision["action"] in {"move", "retreat"}
assert abs(sum(decision["probabilities"].values()) - 1.0) < 1e-12
assert all(math.isfinite(value) for value in decision["probabilities"].values())
if decision["action"] == "move":
    assert decision["target"] is not None
    assert decision["target"]["x"] < 50
print(json.dumps({
    "status": "ok",
    "modelVersion": model["policyVersion"],
    "features": len(FEATURE_NAMES),
    "actionShape": list(npz["action_weights"].shape),
    "movementShape": list(npz["movement_weights"].shape),
    "exampleAction": decision["action"],
    "exampleTarget": decision["target"],
    "exampleProbabilities": decision["probabilities"],
}, indent=2, sort_keys=True))
