# Playable Replays unit-policy training pipeline

Source-only export of the unit-level tactical policy training pipeline from the
`agent/ml-unit-policy` line of work. **No pretrained model artifact is included.**
The `models/` directory is intentionally empty until you train a model.

The implementation uses Python 3.12 and the standard library only. It trains a
transparent model with:

- a four-action linear softmax head: `move`, `hold`, `contest`, `retreat`;
- two tanh movement-regression heads for normalized `dx` and `dy`;
- 72 deterministic tactical features;
- weighted source mixing and class balancing;
- group-isolated train/validation splitting to keep a replay match in one split;
- synthetic bootstrap, reviewed demonstration JSONL, and labeled replay JSONL inputs.

## Layout

```text
ml/unit_policy/
  training.py          General trainer and CLI
  demonstrations.py    Synthetic bootstrap and reviewed JSONL loader
  replay_adapter.py     Pro-labeled movement JSONL adapter
  carry_safety.py       Carry-safety curriculum used for the v2-style model
  carry_aggression.py   Optional aggression-focused retraining
  carry_v1_style.py     Optional contest-heavy curriculum
  features.py           Fixed 72-feature extractor
  model.py              Serializable linear policy
  policy.py             Evaluation/inference helper used by demos/tests
  schema.py             Strict schema 2.0 validation
  demo.py               Small training-and-decision smoke demo
  tests/                Pipeline unit tests
  models/               Intentionally empty output directory
```

## Train the carry-safety configuration

From this archive's root:

```bash
python3 -m ml.unit_policy.carry_safety train \
  --seed 2026 \
  --bootstrap-examples 5000 \
  --curriculum-examples 6500 \
  --epochs 36 \
  --policy-version unit-policy-v2-carry-safety \
  --output ml/unit_policy/models/unit_policy_v2_carry_safety.json
```

The defaults on that command match the recovered v2 training configuration.
Training is deterministic for the same Python implementation, input files,
seed, and arguments.

## General mixed-source training

```bash
python3 -m ml.unit_policy.training \
  --bootstrap-examples 1500 \
  --synthetic-weight 0.25 \
  --replay-jsonl data/labeled-pro-movements.jsonl \
  --replay-min-confidence 0.60 \
  --replay-min-feature-coverage 0.70 \
  --replay-min-profile-confidence 0.60 \
  --policy-version pro-guided-unit-policy-v1 \
  --output ml/unit_policy/models/pro_guided_unit_policy_v1.json
```

`--input-jsonl` accepts reviewed complete snapshot/action records. A record must
contain a valid schema `2.0` snapshot and exactly one action for every live
non-controlled unit.

`--replay-jsonl` accepts rows with schema `pro-labeled-movements-v1`. The replay
adapter uses only pre-decision geometry and role information as features.
Observed outcomes affect targets/weights but are not copied into the inference
feature vector. Match identity is hashed into a group ID for leakage-safe
splitting.

## Tests

```bash
python3 -m unittest discover -s ml/unit_policy/tests -v
```

## Quick smoke run

```bash
python3 -m ml.unit_policy.demo \
  --bootstrap-examples 200 \
  --curriculum-examples 240 \
  --epochs 3
```

## Important boundaries

- No model weights are bundled in this ZIP.
- No MSI/Caedrel-derived labeled replay JSONL data is bundled.
- Synthetic examples are illustrations, not professional replay telemetry.
- Replay labels are observational associations, not proof of optimal play or
  professional-player intent.
- The Go simulator remains authoritative for movement limits and combat.
