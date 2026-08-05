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

`events` is optional and currently accepts `damage`, `kill`, `objective`, and
`vision-loss`. `vision-loss` means the source adapter has established a loss of
useful team vision; it must not include ward ownership, player identity, chat,
or proprietary payloads.
`alive` is optional and defaults to `true`. All other displayed fields are
required. At least two frames are required. `winProbability` must consistently
describe the same source-defined reference team throughout a match.

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

## Semantic labels

Selected windows receive a `one-versus-many` reason tag when the same live unit
is observed within 20 normalized map units of at least two live opponents, with
no other live ally in that radius, across sampled frames spanning at least two
seconds. The detector is deterministic and does not change the canonical score;
it makes an already-selected interactive window easier to classify and route to
an appropriate tactical-board scenario.

The qualifying unit ID is available through
`one_versus_many_unit_ids(...)` for future fixture generation. IDs are sorted so
results do not depend on roster input order.

Selected windows receive a `successful-escape` tag when a live unit at or below
35% health transitions from within 20 map units of a live opponent to at least
35 map units from every live opponent, then remains separated across sampled
frames spanning at least two seconds. The unit must finish the window alive,
and at least one opponent must remain alive, so defeating the only pursuer is
not classified as an escape. `successful_escape_unit_ids(...)` exposes the
qualifying IDs in deterministic order.

Selected windows receive a `team-fight-reversal` tag when the reference team's
win probability forms an interior peak or trough with at least a 0.25 swing on
both sides, the turning frame has live opponents within 20 map units, and the
window contains at least two damage or kill events. The strongest qualifying
turn is selected, with the earliest timestamp breaking ties. This rejects large
but monotonic swings and probability oscillations without nearby combat.

`team_fight_reversal_second(...)` exposes the selected turning timestamp for
future fixture generation.

An `objective` event adds `objective-contest` evidence to a selected window,
and a `vision-loss` event adds `vision-uncertainty`. These evidence tags route
the analyst draft but do not change the four-signal score. Specific semantic
evidence takes priority over the generic proximity-derived `team-fight` tag so,
for example, a successful escape is not misrouted as a team fight merely because
the unit began near an opponent.

## Command

From the repository root:

```bash
python3 -m ml.telemetry path/to/normalized-telemetry.json
```

The command emits one version `1.0` JSON object per selected candidate as
newline-delimited JSON (NDJSON):

```json
{"schemaVersion":"1.0","startSecond":742,"endSecond":754,"score":0.754,"reasonTags":["win-probability-swing","team-fight","one-versus-many","team-fight-reversal"],"signals":{"winProbabilitySwing":0.82,"eventDensity":0.7,"entityProximity":0.18,"resourceAsymmetry":0.54},"semanticEvidence":{"oneVersusManyUnitIds":["blue-carry"],"successfulEscapeUnitIds":[],"teamFightReversalSecond":746}}
```

`signals` contains the exact four normalized inputs used by the canonical
scorer and matches the fixture contract's signal names. Output construction
rejects a candidate if those inputs do not recompute to its score, preventing a
downstream fixture from silently carrying different scoring evidence.

`semanticEvidence` records the deterministic facts that caused semantic tags:
sorted qualifying unit IDs and the selected reversal timestamp. Empty arrays
and `null` are emitted when a detector does not qualify. This lets downstream
fixture generation audit each label without exposing the underlying authorized
telemetry. Threshold, window, stride, and overlap controls are available
through `--help`.

## Accuracy boundaries

- A high score means the heuristic found an interactive candidate; it is not a
  calibrated probability that the moment is a clutch.
- The generic input does not yet carry objective ownership history, escape
  state, terrain, vision, ability ranges, or team-fight phase labels. The
  semantic tags are therefore proximity-based observations. `successful-escape`
  does not prove disengagement intent or which party caused the separation, and
  `one-versus-many` does not claim that the isolated player initiated, survived,
  or won the engagement.
- Source timestamps, win-probability estimates, event classification, and unit
  resources must be validated by the source adapter. Corrupt inputs are rejected
  where detectable, but a structurally valid frame can still be semantically
  wrong.
- Thresholds and the event-rate saturation constant need calibration against an
  analyst-labelled, match-level validation set before production use.

The checked-in synthetic regression pack and its current metrics are documented
in [`detector-evaluation.md`](detector-evaluation.md).
