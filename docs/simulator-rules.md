# Simulator rules

## Credibility boundary

The simulator is a deterministic tactical teaching model. It does not recreate
a proprietary game engine and does not claim that a rollout would have occurred
in a real match. Credibility comes from explicit, inspectable rules and causal
feedback rather than from false precision.

## Authored fixture data

Fixture version 2.1 defines:

- Per-unit health, armor, attack range, attack damage, movement speed, vision,
  cooldown, and policy.
- Terrain position, radius, movement multiplier, and vision blocking.
- Optional objective position, radius, and required control turns.
- A primary victory rule, optional target unit, safe zone, and escape duration.
- A turn-by-turn reference plan with a plain-language reason, plus a coherent
  continuation for each possible opening action.
- A valid default for each possible first action, including a target for Move.
- Analyst-authored category, skill level, rationale, tradeoffs, plausible
  alternatives, and deterministic win/loss acceptance cases.

The loader rejects fixtures that omit these requirements. The dedicated fixture
validator also executes every authored acceptance case against the authoritative
engine. See [`scenario-authoring.md`](scenario-authoring.md).

## Turn resolution

Each legal command resolves in the following stable order:

1. Expire the previous turn's guard and shield; tick cooldowns.
2. Resolve the user's high-level command.
3. Recalculate team vision and log newly revealed contacts.
4. Resolve allied support, protection, or aggression policies.
5. Recalculate vision.
6. Resolve enemy support, protection, aggression, or skirmishing policies.
7. Recalculate vision without exposing hidden coordinates.
8. Update objective control and escape progress.
9. Recalculate scenario advantage from current state.
10. Evaluate explicit terminal conditions and build the debrief.

## Commands

- **Move:** travels toward the selected point using the unit's authored movement
  speed and the terrain multiplier at its starting position.
- **Hold:** adds a small shield and reduces incoming damage for the turn. It does
  not invent health regeneration.
- **Contest:** closes on the nearest visible enemy and attacks only when the
  target is within authored range. With no visible target, an objective scenario
  advances toward the objective.
- **Retreat:** moves toward the authored safe zone at disengage speed and applies
  the turn's defensive guard.

## Combat

Damage begins with the attacker's authored value plus a seeded variation of
minus two to plus two. Armor uses `damage × 100 / (100 + armor)`. Guard reduces
post-armor damage by 35 percent. Shields absorb damage before health. Attacks set
the unit's authored cooldown, and every health change is recorded in the causal
trace.

## Unit policies

- **Support:** shields a nearby damaged or exposed ally; otherwise follows.
- **Protector:** intercepts visible threats near the controlled unit.
- **Aggressive:** prioritizes reachable blue targets, with extra pressure on the
  controlled unit when visible.
- **Skirmisher:** attacks at range and creates distance when an opponent gets too
  close.

These are intentionally compact, inspectable policies—not learned player models.

## Visibility

Blue team vision is the union of living allied vision ranges. Brush and wall
terrain can conceal a target beyond close range. Hidden red units are omitted
from the public unit array; the response exposes only the number of unknown
contacts. If a concealed unit damages an ally, the trace identifies it as an
unseen threat until vision reveals it.

## Scenario advantage

Advantage is derived from the authored initial state plus changes in combined
team health, surviving-unit ratios, objective progress, pressure on an authored
target, and escape progress. Terminal outcomes bound the indicator toward the
winning side. It is a state summary, not a calibrated probability.

## Reference rollouts

The first reference action is hidden until the user commits. When the scenario
ends, the simulator replays Move, Hold, Contest, and Retreat from the same seed,
then follows the opening-specific authored continuation. The response includes each result, ending
advantage, outcome reason, duration, and key causal events.

## Calculated best allied line

After a scenario ends, the engine exhaustively searches all four modeled commands
at every remaining turn from the initial state. `Move` uses the scenario's
authored default destination, so the search is exhaustive over modeled commands
but not over every possible map coordinate.

Complete paths are ranked for the allied team by outcome, rules-based advantage,
allied remaining health, opponent remaining health, and resolution time. The
result includes the selected action at each turn, immediate causal events, the
strongest continuation available after every alternative command, and an
explanation of why the chosen command ranked highest.

The interface labels this as a simulated best case. It is not a guarantee about
a real match or a claim that every ability, item, execution, or hidden-information
branch was searched.

## Known limits

- Circular terrain zones approximate geometry; there is no navmesh or collision
  system.
- Policies reason over high-level positions and do not model abilities, items,
  animation timing, or publisher-specific mechanics.
- The advantage weights are authored and uncalibrated.
- Reference rollouts compare deterministic scenario policies, not real player
  behavior.
