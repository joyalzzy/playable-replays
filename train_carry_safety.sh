#!/usr/bin/env sh
set -eu
python3 -m ml.unit_policy.carry_safety train \
  --seed 2026 \
  --bootstrap-examples 5000 \
  --curriculum-examples 6500 \
  --epochs 36 \
  --policy-version unit-policy-v2-carry-safety \
  --output ml/unit_policy/models/unit_policy_v2_carry_safety.json
