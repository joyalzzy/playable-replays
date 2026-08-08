# Scenario authoring workflow

The version `3.0` fixture library contains three focused scenarios and accepts
only one to three entries. This is an intentionally reviewable teaching set,
not a telemetry-generated catalog.

| Scenario | Category | Skill level | Source |
| --- | --- | --- | --- |
| `resource-trade-932` | Resource trade | Beginner | T1–BLG game 1, 15:32 |
| `positioning-1295` | Positioning | Intermediate | T1–BLG game 3, 21:35 |
| `teamfight-reversal-1727` | Team-fight engagement | Advanced | T1–BLG game 5, 28:47 |

The Game 3 teaching frame follows the bundle's QA recommendation at 21:35. It
is an intentional ten-second rewind from source moment 05's 21:45 core window,
used to expose the pre-commit setup rather than silently shifting provenance.

Difficulty describes the number of tactical signals a learner must combine. It
is not a matchmaking rating or an assessment of a named player's ability.

## Evidence and coordinate disclosure

Every scenario requires `replayEvidence` containing the supplied bundle ID and
SHA-256, source-moment ID, game, decision time, VOD time, judgment, assessment,
coaching correction, caption evidence IDs, external evidence IDs, and
`coordinateMethod`.

`coordinateMethod` must explicitly say that positions are approximations. The
source bundle includes reviewed minimap frames, captions, and event timing, but
does not provide replay-exact positional telemetry. Therefore **all normalized
`0..100` unit, terrain, objective, turret, safe-zone, and movement-target
coordinates are analyst-authored approximations, not measured replay
telemetry**. Never imply otherwise in fixtures, UI, docs, or review notes.

Keep observation, interpretation, and teaching claims separate:

- evidence IDs identify the reviewed source;
- `assessment` summarizes the analyst's judgment of the source decision;
- `coachingCorrection` states the teaching adjustment; and
- simulator outcomes show only what this authored ruleset produces.

## Required fixture content

Each scenario must include:

- a stable ID/slug, title, description, seed, map, reason tags, and `1..20`
  authored reference turns (`maxTurns`); live play may continue beyond them;
- one category, one skill level, an analyst rationale, at least two intended
  tradeoffs, and at least two distinct plausible alternatives;
- exactly five blue and five red units, with one live blue controlled unit and
  exactly one marksman per team;
- class-valid health, movement, attack range, combat values, cooldowns,
  visibility, and a supported policy for every unit;
- authored tactical-goal rules, optional objective/escape rules, terrain, and
  any required pre-play mechanic briefing; these guide the lesson but do not
  replace the engine's universal 2:1 team-health terminal rule;
- a reference action and reason for every turn;
- an action default and a `maxTurns - 1` continuation for each of the four
  tactical commands: `move`, `hold`, `contest`, and `retreat`; and
- at least two executable acceptance cases covering both `won` and `lost` under
  the 2:1 total-health rule.

Only Move accepts a target. Dodge is not part of `actions`, `actionDefaults`,
reference continuations, plausible action alternatives, or the tactical action
enum. An acceptance case may contain up to 20 tactical actions, including
actions beyond the authored `maxTurns` reference horizon, and may list up to two
one-based turn numbers in `dodgeBeforeTurns`; the validator invokes the separate
Dodge reaction before that tactical turn.

The engine, not the fixture, supplies the canonical six turrets. A scenario may
author terrain/objectives and full-map unit placement, but it must not create a
different turret count or use turret placement as replay-exact evidence.

## Mechanic briefing

When a scenario uses a scenario-specific named element such as `baron-pit`, add
a `mechanicBriefing` entry that references that element ID and separately
explains what it does and why it matters here. The client keeps actions and map
targeting locked until the learner acknowledges the briefing.

## Authoring loop

1. Start from the closest current entry in `fixtures/moments.json`; keep the
   top-level pack at no more than three scenarios.
2. Select and record source evidence before tuning mechanics. Preserve the
   bundle hash and cite exact caption/external evidence IDs.
3. Write the teaching rationale, tradeoffs, plausible alternatives, and
   coordinate-approximation disclosure.
4. Author a complete 5v5 state on the `0..100` full map using class profiles and
   the controlled/non-player policy boundary.
5. Define terrain, tactical teaching goals, optional objective/escape state, the
   four-command reference plan, reasons, defaults, and continuations.
6. Add a credible success and failure acceptance line that reaches the 2:1
   total-health threshold. It may continue beyond `maxTurns`, up to 20 actions.
   Add Dodge turn numbers only where a pending projectile is actually available.
7. Validate from `backend/`:

   ```bash
   go run ./cmd/validate-fixtures -path ../fixtures/moments.json
   ```

8. Run `go test ./...`, frontend checks, and `git diff --check`, then inspect the
   scenario through `?moment=<slug>` on the full map.

The Go loader rejects unknown JSON fields, any pack outside `1..3`, duplicate
IDs/slugs, incomplete replay evidence, coordinate disclosure without the word
“approx”, invalid 5v5/class state, malformed rules, missing four-command paths,
and invalid acceptance cases. The dedicated validator also executes every
acceptance line with the deterministic authoritative engine.

## Acceptance-test shape

```json
{
  "name": "sidestep then preserve the winning line",
  "actions": [
    {"type": "hold"},
    {"type": "move", "target": {"x": 24, "y": 56}},
    {"type": "contest"},
    {"type": "hold"}
  ],
  "dodgeBeforeTurns": [2],
  "expectedStatus": "won",
  "expectedTerminalTurn": 4,
  "expectedOutcomeContains": "2:1 total-health lead"
}
```

`dodgeBeforeTurns: [2]` means “invoke the Dodge endpoint-equivalent reaction
after turn 1 created a projectile and immediately before submitting turn 2.” It
does not add a turn or a fifth command.

Acceptance cases establish stable simulator behavior. They do not prove the
line was uniquely correct in the historical match, identify player intent, or
calibrate a real-world win probability.
