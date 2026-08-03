# Architecture

## Prototype boundary

The MVP is a shareable tactical board, not an in-game replay. It works entirely
from synthetic fixtures and uses high-level commands so it can later be adapted
to an authorized telemetry source without coupling the user interface to one
publisher's game engine.

```mermaid
flowchart TD
    A["Authorized or synthetic telemetry"] --> B["Offline highlight scorer"]
    B --> C["Versioned moment fixture"]
    C --> D["Go authoritative simulator"]
    D --> E["TypeScript tactical board"]
    E -->|Legal action| D
```

## Determinism

Each fixture carries a stable seed. The backend owns the random generator and
all state transitions. The same fixture and action sequence therefore produce
the same state, score, and terminal result. Invalid actions leave the state
unchanged.

## Information boundary

The full simulator state remains on the server. Public session responses omit
hidden enemy units entirely, including their coordinates and statistics, and
instead return visible and unknown enemy counts. Terrain can block long-range
vision, and reveal events are logged only when a blue unit obtains vision.
This is a meaningful information boundary, but it is still a prototype rather
than an anti-cheat-hardened production service.

## AI boundary

The baseline highlight selector is an interpretable weighted score. A future
trajectory model may choose among legal high-level actions, but must never
author damage, positions, cooldown resolution, visibility, or victory state.
Optional natural-language commands should be parsed into the same action schema
and rejected unless valid.

## Production components deferred

- Publisher-specific telemetry adapter and authorization
- Compact behavioral-cloning trajectory policy
- Identity-aware pro-player style models
- Ranking evaluation with analyst-labelled pivotal moments
- Durable session storage, authentication, abuse controls, and rate limiting
- In-game engine integration and publisher approval

## Authored simulation rules

Fixture version 2.1 declares unit combat statistics and policies, terrain,
vision, objectives, escape routes, explicit victory conditions, and reference
plans. The server resolves a turn in this order: cooldown and defense reset,
user action, allied policies, enemy policies, visibility, objective and escape
progress, state-derived advantage, then terminal conditions.

Reference advice is withheld until the user commits. Complete deterministic
rollouts for all four legal first actions are returned only when the scenario
ends. They are labelled as authored baselines rather than historical outcomes.
