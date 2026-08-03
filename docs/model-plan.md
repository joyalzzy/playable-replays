# Model and evaluation plan

The runnable prototype needs no model, GPU, or API key.

The current `advantage` field is a rules-based summary of simulator state, not
a learned or calibrated probability. It must not be relabelled as win
probability without a representative labelled dataset and calibration study.

## Highlight selection

Start with the included weighted baseline and an analyst-labelled validation
set. Measure precision at the number of moments a viewer would realistically
see per match, recall of known pivotal events, and calibration of the displayed
confidence. Add a small gradient-boosted model only after the baseline and label
quality are understood.

## Counterfactual policy

Train a compact 10–30M parameter sequence policy over normalized telemetry
trajectories. Inputs should include observable positions, resources, cooldowns,
objectives, vision, time, matchup, and the user's legal action set. The output
is a distribution over high-level legal actions plus parameters such as a map
target.

Use behavioral cloning first, then evaluate held-out players, patches, and
matchups. Report top-k action agreement, calibration, rollout stability,
policy diversity, and analyst preference. Do not call policy agreement
"reproducing a pro player's decision."

## Optional language layer

If free-text commands or coaching are added, fine-tune a permissively licensed
2B–4B instruction model with QLoRA. It should only emit validated command JSON
or explanations grounded in simulator output. Keep it outside the authoritative
simulation path.
