# Detector evaluation pack

The checked-in detector evaluation pack measures whether the deterministic
telemetry detector continues to find, classify, and rank a small set of
analyst-labelled synthetic moments. It is designed to catch regressions before
detector output reaches the scenario authoring workbench.

Run it from the repository root:

```bash
python3 -m ml.evaluate.detector
```

Use `--json` for the stable machine-readable report. A failing regression gate
returns a non-zero exit code.

## Pack contents

`fixtures/telemetry-evaluation/manifest.json` points to seven identity-free
normalized telemetry files:

| Case | Expected lesson |
| --- | --- |
| `objective-pit-contest` | Objective contest with two labelled moments; the stronger second moment must rank first. |
| `team-fight-reversal` | Combat-backed probability reversal. |
| `low-health-escape` | Successful low-health escape into sustained separation. |
| `isolated-positioning-mistake` | One-versus-many positioning error. |
| `cross-map-resource-trade` | Large health and gold asymmetry. |
| `vision-loss-window` | Explicit normalized `vision-loss` evidence. |
| `ordinary-laning` | Quiet negative case that must produce no highlight. |

Each positive case declares expected start/end timestamps, category, and one
primary moment. The manifest also records the exact detector settings and
regression thresholds. Its strict loader rejects unknown fields, missing files,
duplicate IDs, missing tactical-category coverage, invalid thresholds, and
positive cases without exactly one primary moment.

## Metrics

Predicted and expected windows match when their intersection-over-union meets
the manifest's `minimumMatchOverlap`. Predictions are matched once, in expected
annotation order, using the highest overlap and detector rank as deterministic
tie-breakers.

The evaluator reports:

- **Precision:** matched predictions divided by all predictions.
- **Recall:** matched predictions divided by all expected highlights.
- **Category accuracy:** matched predictions routed to the analyst-labelled
  scenario category.
- **Best moment ranked first:** positive matches whose first detector result is
  the case's primary expected moment.
- **False-positive match rate:** ordinary matches that produce any candidate.
- Raw false-positive-window and missed-highlight counts.

The current synthetic baseline is 7/7 expected windows detected, no false
positives or misses, 100% category accuracy, and the primary moment ranked first
in all six positive matches. These are strict regression expectations for this
small controlled corpus, not estimates of real-match accuracy.

## Category routing

Specific semantic evidence outranks the generic proximity-derived `team-fight`
tag:

1. `team-fight-reversal`
2. `successful-escape`
3. objective evidence
4. vision/fog/uncertainty evidence
5. resource/gold/trade evidence
6. `one-versus-many`
7. generic `team-fight`
8. positioning fallback

The normalized `vision-loss` event represents source-confirmed loss of useful
team vision. It contains no ward owner, player identity, chat, or proprietary
payload. Objective and vision events add auditable reason tags but do not change
the canonical four-signal score formula.

## Limits and next calibration work

The pack is authored to exercise known boundaries and is too small, clean, and
synthetic to estimate production precision or recall. Repeated event entries
represent multiple normalized events observed within a sampled interval. They
are not raw game packets.

Before production use, expand this process with independently labelled,
authorized matches, multiple analysts, ambiguous near-misses, patch/version
metadata, confidence intervals, and held-out calibration splits. Do not weaken
the checked-in thresholds merely to accommodate a detector change; update an
annotation only when analyst review finds it wrong, and explain that decision in
the change review.
