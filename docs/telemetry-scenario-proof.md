# Telemetry scenario draft proof

This checkpoint proves the experimental `detected highlight -> scenario draft`
workflow with synthetic detector data. It does not depend on telemetry PR #3
being merged, and it does not publish or merge the experimental branch.

## Proof fixture

The input is a single NDJSON detection covering seconds 1402-1414 with these
detector facts:

- score `0.7455`;
- labels `team-fight-reversal`, `one-versus-many`, and
  `win-probability-swing`;
- four detector signals;
- the detected unit ID and reversal timestamp as semantic evidence.

The converter mapped the record to `team-fight-engagement` and created the
version 2.1 draft `lower-brush-reversal-1402`. The generated draft preserved
the score, time window, signals, labels, and semantic evidence exactly while
leaving rationale, tradeoffs, alternatives, and acceptance tests incomplete.

The analyst-completed scenario is named **Lower Brush Reversal**. It contains
an intermediate teaching setup, two tradeoffs, two plausible alternatives, and
two executable acceptance tests. Publishing it produced a 13-scenario version
2.1 review pack without modifying `fixtures/moments.json`.

The proof files are stored outside the repository under
`outputs/telemetry-draft-proof/`:

- `detection.ndjson` - synthetic detector input;
- `drafts.json` - completed analyst draft;
- `moments.review.json` - validated review pack;
- `moments.preview.json` - local preview pack.

## Gates and automated checks

The focused Go checks passed:

```text
go test ./cmd/scenario-draft ./internal/drafts ./internal/highlight
go vet ./...
git diff --check
```

The tests explicitly verify that:

- label mapping gives reversals priority and covers escape, objective, vision,
  resource, and positioning detections;
- conversion preserves the complete detector record;
- generated drafts remain intentionally incomplete;
- an incomplete draft cannot publish;
- a completed draft can join a validated fixture pack;
- unknown detector fields and a mismatched detector score are rejected.

The schema was also corrected to accept the simulator's serialized `shield`
and `guarded` unit state. The schema file parses successfully, and its unit
definition now requires a non-negative integer shield when present and a
boolean guarded value when present.

## Local gameplay check

The preview was opened at:

```text
http://127.0.0.1:5173/?moment=lower-brush-reversal-1402
```

The authored success line was played through the UI:

1. Select Move and click map coordinate X 40, Y 46.
2. Commit the move.
3. Select Contest and commit.

Observed result: `Scenario secured` on turn 2, with the outcome text
`The isolated mage was eliminated before reinforcements stabilized the fight.`
The coordinate hover/selection display showed X 40, Y 46, the post-commit
reference explained both authored turns, and the causal trace recorded the
movement, damage, elimination, vision, and outcome events.

The calculated-best-case panel also opened successfully. Both turn selectors
displayed a causal explanation and the strongest continuation after every
available command. The exhaustive search selected Hold then Contest; Hold was
tied with two openings for the strongest reachable result. This is a useful
scenario-quality observation: the authored Move line is fast and valid, but it
is not uniquely optimal under the current deterministic model. A later tuning
pass should decide whether that ambiguity is intentional or make the brush
reposition uniquely preferable.

## Environment limitation

Windows Application Control blocked newly compiled Go executables in this
workspace. The exact `scenario-draft` command entry points were therefore run
through temporary Go test harnesses, which called the same command functions;
the harness files were removed afterward. Focused package tests and vet passed.

For the browser gameplay check only, an older cached backend that accepts
fixture version 2.0 was used with a labeled runtime-only compatibility copy of
the version 2.1 preview pack. The authoritative detector draft and published
packs remain version 2.1. The cached API also predates category and skill-level
summary fields, so a runtime-only frontend copy supplied intermediate/category
fallback labels. These compatibility files are evidence aids under
`outputs/telemetry-draft-proof/`; they are not branch deliverables.

The full `go test ./...` run could compile the repository but Windows
Application Control blocked several generated test executables. This is an
environment execution restriction, not a reported test assertion failure.
