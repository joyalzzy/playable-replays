# League of Legends replay guidance

Read this file only for League of Legends broadcasts or playable-replay datasets derived from them.

## Candidate event priority

Prioritize decisions around:

1. Elder Dragon, Baron, Soul-point or Soul dragons, and match-ending pushes.
2. Picks or team fights that immediately convert into a major objective or base damage.
3. Herald use, tower trades, cross-map responses, and contested recalls when they change map control.
4. Failed contests, blind face-checks, uneven engages, overextensions, and numerical-disadvantage reversals.

Routine uncontested dragons or towers are low priority unless the trade, setup, or follow-up exposes a teachable choice.

## Timeline evidence

Use official event data when available. Games of Legends timeline pages are useful secondary structured evidence for kills, dragons, Heralds, Barons, and towers. Cross-check game number, teams, side selection, patch, and event order before accepting a timeline.

Treat timeline rows as event-time evidence only. They do not prove that a strategic decision was right. Keep editorial match analysis and caster commentary as separate sources.

## Clock alignment

The scoreboard clock is the strongest VOD-to-game anchor during live play. OCR only a tight clock region and sample multiple nearby frames. Reject readings during:

- replays or picture-in-picture segments;
- pauses, analyst desk, or crowd shots;
- clock animations or occlusion;
- frames where digits disagree across adjacent samples.

Use kills, objective announcements, and caption phrases to disambiguate repeated clock values across games. Store game ID with every anchor.

Edited co-streams frequently remove draft, pauses, downtime, and parts of lane phases. Build piecewise anchors and never concatenate official game durations to predict VOD time.

## Coaching state

Base an assessment on observable information available near the decision:

- player positions and arrival times;
- alive/dead state and numerical advantage;
- health, mana/energy, major cooldowns, summoners, and ultimates when visible;
- objective health, smite access, turn potential, and pit control;
- lane priority, wave state, teleport or recall timing;
- vision, fog entry, choke points, and flank access;
- champion range, engage, disengage, scaling, and wave-clear;
- bounty, turret, inhibitor, Soul, Baron, or Elder value;
- whether the team can end, reset, trade cross-map, or disengage.

State hidden-information limits. Do not invent voice communications, exact cooldowns, wards, or opponent intent that are not observable.

## Decision taxonomy

Use concise event types such as:

- `dragon-contest`, `baron-start`, `baron-contest`, `objective-trade`;
- `pick-to-objective`, `tower-dive`, `base-siege`, `forced-engage`;
- `failed-facecheck`, `failed-all-in`, `disengage`, `reengage`;
- `cross-map-trade`, `recall-timing`, `match-ending-conversion`.

Distinguish planning from execution. Example: `assessment.label=correct` can coexist with a note that a missed recall or mechanical error made the position unnecessarily risky.

## Clip boundaries

Begin before the first irreversible commitment, not merely at the first kill. Include lane priority, vision setup, recalls, or the caster warning that defines the choice. End after the consequence is clear: objective secured, trade completed, reset achieved, or base broken.

For an ending push, keep enough aftermath to establish the win but avoid unrelated celebration unless the user wants broadcast storytelling footage.
