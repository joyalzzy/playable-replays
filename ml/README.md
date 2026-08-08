# Offline ML

The live Go simulator does not import this package. `highlight.py` preserves the
authored highlight scorer; the replay pipeline trains a separate, generic
behavioral-cloning baseline from decoded packet data.

## Replay action baseline

Download an authorized shard outside Git, then train with a match-level split:

```bash
python -m ml.train.replay_policy \
  .local-data/datasets/lol-replays/12_22/batch_001.jsonl.gz \
  --output .local-data/models/replay-policy.json \
  --max-matches 12 \
  --seed 742
```

The trainer streams JSONL matches, maps movement, basic attack, spell, and item
packets to four coarse action classes, and predicts the next class from match
phase and recent action context. Its JSON artifact records the source SHA-256,
patch, split seed, feature schema, sample sizes, and held-out top-k agreement and
multiclass Brier score. Raw shards and artifacts remain under ignored
`.local-data/`; do not commit either.

The baseline is intentionally not connected to `model-daemon/` or the Go
engine. It does not produce the simulator's `hold`, `contest`, or `retreat`
actions, infer intent, rank optimal decisions, or model a named player's style.
Movement is heavily overrepresented, so top-1 agreement must be interpreted
alongside action distribution and calibration. Production work needs broader
patch coverage, role/champion features, fixed train/validation/test manifests,
class-aware metrics, rollout evaluation, and an adapter to the versioned online
contract.
