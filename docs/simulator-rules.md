# Simulator rules

## Credibility boundary

The simulator is a deterministic tactical teaching model. It does not recreate
a proprietary game engine and does not claim that a rollout would have happened
in the T1–Bilibili Gaming 2024 Worlds Final. Source evidence motivates the three
teaching decisions; all `0..100` map coordinates are authored normalized
approximations informed by reviewed minimap frames, not replay telemetry or
replay-exact positions.

Scenario advantage is a rules-based state indicator, not a calibrated win
probability. Reference and best-case lines are simulator comparisons, not proof
of optimal real-match play or player intent.

## Full map and classes

The authoritative map is inclusive `0..100` in both axes. The frontend renders
the full map returned by the server, never a scenario-focused crop. Terrain is
modeled as circular zones with movement and vision properties; there is no
navmesh or collision system.

Every session includes exactly six canonical server-supplied turrets: blue and
red top, middle, and bottom. Each has `hp`, `maxHp`, and `alive` state. In the
current model, turrets are visual landmarks only: they do not attack, block
movement, or receive damage.

| Class | Maximum health | Move range | Attack range |
| --- | ---: | ---: | ---: |
| Tank | 160 | 7 | 10 |
| Fighter | 125 | 10 | 14 |
| Marksman | 90 | 11 | 28 |
| Mage | 95 | 9 | 24 |
| Support | 110 | 8 | 20 |
| Assassin | 100 | 13 | 12 |

The engine overwrites fixture/model-derived class stats with these profiles.
Move targets specify intent; actual displacement remains capped by the
server-owned class range and the terrain multiplier at the starting position.

## Tactical commands

`Session.legalActions` contains exactly four values:

- **Move:** travel toward the selected map point. Move requires an in-bounds
  target and is capped by class movement and terrain.
- **Hold:** gain four shield and guard for this turn. It does not regenerate
  health.
- **Contest:** focus the visible authored elimination target, otherwise close
  on the nearest visible enemy and attack in range. If no enemy is visible in
  an objective scenario, advance toward the objective.
- **Retreat:** move toward the authored safe zone at disengage speed and apply
  guard.

Dodge is deliberately not in this list. No fifth tactical command is supported.

## Pending marksman projectiles

A marksman attack creates a `Projectile` rather than dealing immediate damage.
The projectile records its source/target unit IDs, launch position, fixed visual
target point, team, and damage. Its damage is half the target's maximum health,
rounded up. The marksman's normal cooldown begins when it fires.

The projectile remains in `Session.projectiles` after the turn that launched
it. At the beginning of the next accepted tactical turn, before the user's new
command, every still-pending projectile resolves against its target unit and is
removed. Moving normally does not cancel a targeted projectile; use the
eligible Dodge reaction. A dead/missing target causes the projectile to expire.

For hidden red marksmen, public state and logs redact the source unit ID/name as
needed, while still exposing the incoming threat and its target path.

## Separate two-charge Dodge reaction

Every session starts with `dodgeCharges: 2`. `dodgeAvailable` is true only while
the session is active, a charge remains, the controlled unit is alive, and a
red projectile currently targets that unit.

`POST /api/v1/sessions/{id}/dodge` then:

1. validates availability;
2. removes the first eligible incoming projectile;
3. moves the controlled unit to an automatically selected, class-limited
   sidestep point;
4. consumes one charge and logs the evade; and
5. returns the updated `Session`.

Dodge does **not** increment `turn`, reset defense, tick cooldowns, invoke the
bot model, resolve other projectiles, advance objectives/escape state, or enter
the tactical decision tree. Calling it without an eligible projectile/charge
returns `422 dodge_unavailable` and leaves gameplay state unchanged.

## Tactical-turn resolution

After complete action validation, one tactical turn resolves in this order:

1. Clear the previous turn's guard/shield and tick unit cooldowns.
2. Increment `turn`.
3. Resolve and remove all projectiles pending from the previous turn.
4. If the controlled unit survives, resolve its Move/Hold/Contest/Retreat.
5. Recalculate blue-team vision and log newly revealed contacts.
6. If configured, request and atomically validate one model action for every
   live non-controlled unit; otherwise choose deterministic actions.
7. Resolve allies, recalculate vision, resolve enemies, record model/fallback
   status, and recalculate vision again.
8. Update objective control and escape progress.
9. Recalculate scenario advantage.
10. Evaluate terminal conditions and reveal the authored reference for that
    turn.
11. Update Dodge availability and, for a terminal state, build the debrief.

If a pending projectile eliminates the controlled unit at step 3, later action
and bot resolution are skipped; outcome/reference/debrief state still updates.

## Bot actions and fallback

The optional schema `2.0` model supplies high-level Move/Hold/Contest/Retreat
actions for every live non-controlled allied and enemy unit. Go validates the
full response atomically and resolves it under the same class, combat,
visibility, and state rules. The model never controls the user's unit and
cannot issue Dodge.

No configured model, or any model/transport/validation failure, activates the
deterministic policy for that turn. `botControl.source` reports `pending`,
`external-model`, or `deterministic-fallback`; accepted external results also
include operator-configured model name/version.

Built-in unit policies remain intentionally compact. An authored elimination
target holds its exposed state; another unit below 35 percent health retreats;
support/protector units hold; a blue bot with no visible enemy moves toward the
controlled unit; otherwise the bot contests. Contest closes on the
engine-selected visible enemy (with aggressive red bots strongly prioritizing
the controlled unit) and attacks when range/cooldown allows. When the
controlled unit Holds with shield available in an elimination scenario, an
aggressive blue bot synchronizes onto the visible authored target; this is the
timing mechanic behind the positioning lesson.

These policies and model actions are bot behavior, not imitations of a named
professional player.

## Non-projectile combat

Non-marksman damage begins with authored attack damage plus seeded variation
from minus two to plus two. Armor applies `damage × 100 / (100 + armor)`. Guard
reduces post-armor damage by 35 percent. Shields absorb damage before health.
Attacks set the authored cooldown; every health change is recorded in the
causal log.

Blue vision is the union of living allied vision ranges. Brush/wall terrain can
conceal red units beyond close range. Hidden red units are omitted from the
public unit array, and the session exposes visible and unknown counts instead.

## Objective, escape, advantage, and outcome

Objective control advances from living units inside the authored radius;
escape progress advances only under authored safe-zone rules. Advantage combines
the authored initial state with team health, surviving-unit ratios, objective
progress, pressure on an authored target, and escape progress, then clamps to a
bounded display range. Explicit fixture victory/defeat rules determine terminal
status.

## Reference outcomes and best case

The current turn's authored reference is hidden until the learner commits.
When a scenario ends, deterministic reference rollouts compare the four legal
opening commands using authored continuations. The calculated best allied line
exhaustively searches all four commands at every remaining turn; Move uses the
scenario's authored default target rather than every possible coordinate.

Reference simulation invokes the same separate Dodge reaction automatically
when an incoming projectile is eligible and a charge remains. Thus projectile
handling is represented without making Dodge a fifth search branch. Paths rank
by terminal outcome, rules-based advantage, allied health, opponent health, and
resolution time, and expose causal events and alternatives in the debrief.

## Known limits

- The geometry is normalized and authored; it is not measured replay state.
- Turrets are landmarks, and there is no navmesh, collision, minions, items,
  animation timing, publisher-specific ability kit, or full fog-of-war system.
- Projectiles are one-turn targeted teaching mechanics, not physical collision
  simulation.
- Advantage weights and scenario rules are authored and uncalibrated.
- Model output can vary; deterministic fallback and reference lines remain the
  stable comparison baseline.
