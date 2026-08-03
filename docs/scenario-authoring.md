# Scenario authoring workflow

The authored library contains 12 synthetic tactical scenarios. It is deliberately
bounded to 10–20 entries so analysts can review every scenario while the team
collects enough variety to study replay selection, ranking, and retention.

## Coverage

| Category | Beginner | Intermediate | Advanced | Total |
| --- | ---: | ---: | ---: | ---: |
| Objective contest | 1 | 0 | 1 | 2 |
| Team-fight engagement | 1 | 1 | 0 | 2 |
| Escape | 1 | 0 | 1 | 2 |
| Positioning | 1 | 0 | 1 | 2 |
| Resource trade | 0 | 1 | 1 | 2 |
| Vision uncertainty | 1 | 1 | 0 | 2 |
| **Total** | **5** | **3** | **4** | **12** |

Difficulty describes how many tactical signals the learner must combine. It is
not a matchmaking rating or a claim about a player's ability.

## Required evidence

Fixture version `2.1` requires every scenario to include:

- one category and one skill level;
- an analyst rationale explaining the teaching decision;
- at least two intended tradeoffs;
- at least two plausible alternative actions, each with the condition that
  makes it reasonable and its cost;
- a reference action and reason for every turn;
- a default Move destination and complete continuations for all four commands;
- at least two executable acceptance cases, including one modeled win and one
  modeled loss.

Acceptance cases assert the terminal status, terminal turn, and a stable phrase
from the outcome reason. They verify the authored simulator behavior; they do
not prove that the line is optimal in a real match.

## Authoring loop

1. Copy the closest existing scenario in `fixtures/moments.json`.
2. Give it a stable event-and-window ID, unique slug and seed, and synthetic
   units and telemetry signals.
3. Write the learning rationale and tradeoffs before tuning combat values.
4. Author the reference plan, alternatives, victory condition, terrain, and
   hidden-information boundary.
5. Add a credible success line and a credible failure line under
   `authoring.acceptanceTests`.
6. Run the validator from `backend/`:

   ```bash
   go run ./cmd/validate-fixtures -path ../fixtures/moments.json
   ```

7. Run `go test ./...`, the frontend checks, and `git diff --check` before
   review. Inspect the scenario in the browser at `?moment=<slug>`.

The validator rejects unknown JSON fields, invalid coordinates or combat state,
missing categories or skill levels, duplicate IDs or slugs, incomplete reference
paths, malformed alternatives, and packs outside the 10–20 scenario boundary.
It then replays every acceptance case with the authoritative deterministic Go
engine and prints the category and skill coverage matrix.

## Acceptance-test shape

```json
{
  "name": "support-side reposition wins exchange",
  "actions": [
    {"type": "move", "target": {"x": 24, "y": 56}},
    {"type": "hold"},
    {"type": "contest"},
    {"type": "hold"}
  ],
  "expectedStatus": "won",
  "expectedTerminalTurn": 4,
  "expectedOutcomeContains": "stronger tactical state"
}
```

Keep assertions about causal simulator outcomes. Do not encode analyst identity,
proprietary match data, player identity, or claims that the reference line is
the only correct real-match decision.
