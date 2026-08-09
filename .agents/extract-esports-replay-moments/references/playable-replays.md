# Playable Replays v3.0 export

Read this file when the requested output must load in
[`joyalzzy/playable-replays`](https://github.com/joyalzzy/playable-replays).

## Contents

- Canonical sources
- Output files
- Authoring draft
- Evidence mapping
- Project invariants
- Export and validation

## Canonical sources

- Schema: `contracts/moment.schema.json` on the project's `main` branch.
- Fixture destination: `fixtures/moments.json`.
- Authoring rules: `AGENTS.md` and `docs/scenario-authoring.md`.
- Authoritative validation: `go run ./cmd/validate-fixtures -path <pack>` from
  the project's `backend/` directory.

Before each export, prefer the schema in the user's current project checkout.
Otherwise retrieve the linked `main` schema and record its SHA-256. The bundled
`playable-replays-moment.schema.json` is an offline snapshot from project commit
`1ca9840282a987549ffa706ab695a25876efe5f4`, with SHA-256
`8c7a31f5d26ba9584df998b6db225bf4a91cf8f0fd30f99b15a88905125d8de6`;
do not assume it is newer than a supplied checkout.

## Output files

Add these to the replay deliverable:

```text
analysis/playable-replays-drafts.json
playable-replays/moments.json
schema/playable-replays-moment.schema.json
```

`playable-replays/moments.json` is the importable fixture pack. Keep the draft
file because it distinguishes replay-derived evidence from authored simulator
state. Never replace `fixtures/moments.json` in a project checkout unless the
user explicitly asks for that repository change.

## Authoring draft

Use this exact top-level draft shape:

```json
{
  "draft_version": "1.0",
  "bundle": {
    "id": "source-evidence-bundle-id",
    "sha256": "64 lowercase hexadecimal characters"
  },
  "moments": [
    {
      "source_moment_id": "01_g1_dragon_fight",
      "game": 1,
      "coordinate_method": "Authored normalized approximations from reviewed minimap frames; not replay-exact telemetry.",
      "fixture": {
        "id": "dragon-cross-map-trade",
        "slug": "dragon-cross-map-trade",
        "title": "Bank the Cross-map Trade",
        "description": "A concise playable teaching objective.",
        "map": "Full MOBA map · one turret per lane",
        "seed": 1001,
        "maxTurns": 4,
        "controlledUnitId": "blue-marksman",
        "reasonTags": ["cross-map-trade"],
        "signals": {},
        "units": [],
        "rules": {},
        "authoring": {}
      }
    }
  ]
}
```

The abbreviated nested objects are placeholders, not a valid scenario. Start
from the closest current project fixture and author every field required by the
canonical schema. Do not include `startTimeSeconds` or `replayEvidence` inside
`fixture`; the exporter derives them from the source manifest and draft.

## Evidence mapping

| Project field | Replay dataset source |
| --- | --- |
| `startTimeSeconds` | Parsed decision-time game clock, not VOD time |
| `sourceMomentId` | Draft `source_moment_id` |
| `decisionTime` | `game_time.decision` |
| `sourceVodSeconds` | `vod.decision_s` |
| `judgment` | `assessment.label` |
| `assessment` | `assessment.summary` |
| `coachingCorrection` | `assessment.better_alternative` |
| `captionEvidence` | Unique transcript and caster-context caption IDs |
| `externalEvidence` | External timeline event IDs |
| `coordinateMethod` | Draft disclosure containing `approx` |

Only export source moments marked `project_ready`. The source evidence bundle
hash identifies the already-built evidence bundle; it is not the hash of the
new ZIP that will contain this export.

## Project invariants

- Export exactly one to three moments at version `3.0`.
- Use only `move`, `hold`, `contest`, and `retreat` as tactical actions. Dodge
  may appear only as `dodgeBeforeTurns` in acceptance tests.
- Author exactly five blue and five red units, one marksman per team, and one
  live blue `controlled` unit.
- Treat positions, HP, combat stats, terrain, objectives, policies, plans, and
  outcomes as simulator authoring informed by replay evidence—not telemetry.
- Use normalized inclusive `0..100` coordinates and canonical class profiles.
- Supply complete action defaults and `maxTurns - 1` continuations for all four
  actions, plus a reference action and reason for every authored turn.
- Include at least two distinct alternatives and executable acceptance lines
  covering both a win and a loss.
- Explain every scenario-specific mechanic such as `baron-pit` before play.

## Export and validation

```bash
python3 scripts/export_playable_replays.py export \
  --manifest deliverable/analysis/moments.json \
  --drafts deliverable/analysis/playable-replays-drafts.json \
  --schema /path/to/playable-replays/contracts/moment.schema.json \
  --output deliverable/playable-replays/moments.json \
  --project-root /path/to/playable-replays
```

The script validates the exact JSON Schema, mirrors the project's semantic
loader checks, writes atomically, and—when `--project-root` is supplied—runs the
Go engine-backed fixture and acceptance validator before finalizing the file.
Do not call an export project-usable until this last validation passes against
the target checkout. After copying it into the project, run `make
validate-fixtures`, relevant Go tests, and inspect each scenario in the UI.
