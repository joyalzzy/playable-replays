# Telemetry detection to scenario draft

The scenario-draft command bridges offline telemetry highlight detection and the
analyst-authored version `2.1` scenario library. It never invents combat state,
terrain, intent, coaching rationale, or acceptance results from detector output.

Both prerequisite changes are currently developed on separate branches. Until
their pull requests merge, this workflow is a stacked integration and must be
rebased onto `main` after the telemetry and authored-scenario work lands.

## Create drafts from detector NDJSON

Generate detector records from normalized, authorized or synthetic telemetry:

```bash
python3 -m ml.telemetry path/to/normalized-telemetry.json > outputs/detections.ndjson
```

From `backend/`, convert every NDJSON record into an intentionally incomplete
draft:

```bash
go run ./cmd/scenario-draft create \
  --input ../outputs/detections.ndjson \
  --output ../outputs/scenario-drafts.json
```

The output bundle uses version `2.1`. Every `drafts[].scenario` preserves:

- detector schema version, start and end timestamps, and four-decimal score;
- the exact four canonical scoring signals;
- all detector reason tags;
- one-versus-many unit IDs, successful-escape unit IDs, and the optional
  team-fight reversal timestamp.

The detector provenance remains in `scenario.sourceDetection` after publication.
The loader verifies that its score recomputes from its signals, the scenario
start and signals still match, source tags remain present, and semantic evidence
is structurally valid.

The cross-language regression journey can be run from the repository root:

```bash
python3 scripts/test_telemetry_scenario_pipeline.py
```

It generates synthetic normalized telemetry, runs the Python detector, creates
an incomplete Go draft, proves publication is blocked, supplies known-valid
analyst-authored scenario content, validates it, and produces a local preview
pack while checking that all detector provenance is unchanged.

## Category mapping

Mapping is deterministic and uses this priority:

| Detector evidence | Draft category |
| --- | --- |
| `team-fight-reversal` | `team-fight-engagement` |
| `successful-escape` | `escape` |
| Tag containing `objective` | `objective-contest` |
| Tag containing `vision`, `fog`, or `uncertainty` | `vision-uncertainty` |
| Tag containing `resource`, `gold`, or `trade` | `resource-trade` |
| `one-versus-many` | `positioning` |
| Generic `team-fight` | `team-fight-engagement` |
| All other records | `positioning` |

An analyst may correct the category when broader match context supports a
different lesson. The source tags remain unchanged for auditability.

## Complete and validate a draft

The generated scenario deliberately has blank title, description, map, skill
level, analyst rationale, synthetic units, and simulator rules. Its tradeoffs,
alternatives, and acceptance tests are empty arrays. Copying the closest
existing scenario can accelerate authoring, but retain the generated identity,
signals, timestamps, reason tags, and `sourceDetection` block.

Validation is expected to fail until the analyst has supplied the missing work:

```bash
go run ./cmd/scenario-draft validate \
  --draft ../outputs/scenario-drafts.json \
  --index 0
```

A completed draft must pass the single-scenario semantic validator and replay
at least one modeled win and one modeled loss acceptance case. A valid JSON
shape alone is not enough.

## Publish to a review pack

Publishing refuses incomplete drafts, duplicate IDs or slugs, broken category
or difficulty coverage, score/provenance mismatches, and failed acceptance
cases. It also refuses to overwrite the base library directly.

```bash
go run ./cmd/scenario-draft publish \
  --draft ../outputs/scenario-drafts.json \
  --index 0 \
  --base ../fixtures/moments.json \
  --output ../outputs/moments.review.json
```

The output is a complete version `2.1` fixture pack for review. Replacing the
committed library remains a deliberate, separately reviewed file operation.

## Local preview

The preview command performs the same publication and acceptance gates, writes
a separate fixture pack, and prints the API, frontend, and stable scenario URL
commands:

```bash
go run ./cmd/scenario-draft preview \
  --draft ../outputs/scenario-drafts.json \
  --index 0 \
  --base ../fixtures/moments.json \
  --output ../outputs/moments.preview.json
```

The generated commands point `FIXTURE_PATH` at the absolute preview pack. The
normal web UI then opens the completed scenario at `?moment=<generated-slug>`.

## Accuracy and data boundaries

- A detector score ranks heuristic interactivity; it is not a clutch
  probability or a predicted match outcome.
- Semantic detector evidence does not establish intent, terrain interaction,
  vision state, or the correct coaching lesson.
- Use only authorized or synthetic normalized telemetry. Drafts and published
  fixtures must not contain player identity or proprietary raw replay data.
- Analysts remain responsible for synthetic simulator state, causal rationale,
  tradeoffs, alternatives, reference lines, and executable acceptance cases.
