# Telemetry highlight extraction

`ml.telemetry` turns normalized, authorized or synthetic match telemetry into
ranked pivotal-window candidates. It is deterministic, uses no network service,
and feeds the existing interpretable scorer in `ml.highlight`.

This is a normalization boundary, not a decoder for any publisher's proprietary
replay format. A source-specific adapter must establish authorization and map
its fields into this contract before invoking the highlighter.

## Input contract

The version `1.0` JSON document contains strictly increasing frames. Unit IDs
and teams must stay stable, coordinates use the inclusive normalized `0..100`
map, and exactly two teams must be present. Unknown fields and event types are
rejected instead of being ignored.

```json
{
  "version": "1.0",
  "frames": [
    {
      "second": 742,
      "winProbability": 0.31,
      "events": ["damage", "objective"],
      "units": [
        {
          "id": "blue-carry",
          "team": "blue",
          "position": {"x": 31, "y": 54},
          "hp": 68,
          "maxHp": 100,
          "gold": 9100,
          "alive": true
        },
        {
          "id": "red-jungle",
          "team": "red",
          "position": {"x": 48, "y": 51},
          "hp": 55,
          "maxHp": 100,
          "gold": 9400,
          "alive": true
        }
      ]
    }
  ]
}
```

`events` is optional and currently accepts `damage`, `kill`, and `objective`.
`alive` is optional and defaults to `true`. All other displayed fields are
required. At least two frames are required.

## Derived signals

For every fully covered window, the extractor calculates:

| Signal | Calculation |
| --- | --- |
| Win-probability swing | Maximum minus minimum probability in the window. |
| Event density | Counted events per second divided by the configurable saturation rate, capped at `1`; the default saturation rate is `2` events/second. |
| Entity proximity | Closest live opposing-unit distance divided by the normalized map diagonal; smaller means closer. |
| Resource asymmetry | Largest per-frame mean of the teams' normalized HP gap and proportional gold gap. |

The four values enter the canonical weighted formula documented in
`AGENTS.md`. The extractor chooses the first frame at or beyond a requested
window end, so irregularly sampled inputs are fully covered and their true
duration is used for event density.

Candidates are sorted by score and start time. Greedy non-maximum suppression
then removes windows whose overlap exceeds the configured fraction of the
shorter window. This prevents a single fight from producing many nearly
identical share links.

## Command

From the repository root:

```bash
python3 -m ml.telemetry path/to/normalized-telemetry.json
```

The command emits one JSON object per selected candidate. Threshold, window,
stride, and overlap controls are available through `--help`.

## Accuracy boundaries

- A high score means the heuristic found an interactive candidate; it is not a
  calibrated probability that the moment is a clutch.
- The generic input does not yet carry objective ownership history, escape
  state, or team-fight phase labels, so those semantic event types are not
  inferred by this module.
- Source timestamps, win-probability estimates, event classification, and unit
  resources must be validated by the source adapter. Corrupt inputs are rejected
  where detectable, but a structurally valid frame can still be semantically
  wrong.
- Thresholds and the event-rate saturation constant need calibration against an
  analyst-labelled, match-level validation set before production use.
