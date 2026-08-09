---
name: extract-esports-replay-moments
description: Extract pivotal moments from esports match VODs or livestream recordings and turn them into verified replay datasets and Playable Replays v3.0 scenario packs. Use for requests to download or inspect an authorized/public match video; identify fights, objectives, towers, clutches, reversals, or decision points; align edited broadcast time with in-game time; transcribe surrounding commentary; judge or explain player decisions; render clips/audio/thumbnails; package clips, transcripts, provenance, schemas, and checksums; or export fixtures compatible with joyalzzy/playable-replays. Especially useful for League of Legends VOD analysis and Playable Replays training or scenario data.
---

# Extract Esports Replay Moments

Produce a reproducible dataset, not merely a highlight reel. Preserve the distinction between source evidence, commentary interpretation, and coaching inference.

## Establish scope

1. Verify the supplied URL or file: title, teams, event, date, duration, resolution, audio, chapters, and captions.
2. State any mismatch between the user's description and the actual source before processing.
3. Confirm which deliverables the user requested: source media, pivotal clips, transcripts, coaching labels, research data, or all of them.
4. Process only public or user-authorized media. Do not bypass DRM, authentication, paywalls, or access controls.
5. Estimate whether the source is one game or a multi-game broadcast. Preserve both VOD timestamps and in-game timestamps.

For League of Legends, read [references/league-of-legends.md](references/league-of-legends.md). Read [references/output-contract.md](references/output-contract.md) before creating data. When the output must load in `joyalzzy/playable-replays`, also read [references/playable-replays.md](references/playable-replays.md) and use the target checkout's current schema when available.

## Acquire source and captions

Prefer an existing local source. For a public URL, use `yt-dlp` when available and record its version and exact source URL. Select a practical archival format, normally H.264/AAC at up to 720p60 unless the user asks otherwise. Save source metadata and raw captions alongside the media.

```bash
yt-dlp \
  --format 'bestvideo[height<=720]+bestaudio/best[height<=720]' \
  --merge-output-format mp4 \
  --write-info-json --write-auto-subs --write-subs \
  --sub-langs 'en.*' --sub-format vtt \
  --output 'source_media/%(id)s.%(ext)s' URL
```

Adapt format selectors to the site and available streams. If acquisition is blocked, report the blocker or ask for an uploaded source; do not silently substitute another broadcast or embed one-off signed-host workarounds.

Keep raw VTT/JSON3 caption files unchanged. Use `scripts/normalize_vtt.py` to create JSONL, CSV, SRT, and readable text derivatives from either format. Automatic captions are uncertain evidence, not a definitive transcript. If local ASR is available and useful, run it independently and retain both sources.

## Research and select moments

Run source acquisition, caption normalization, and external timeline research in parallel when they are independent. Research match identity and event timelines using primary or authoritative sources where possible. For each candidate, record source URL, event/game time, event type, and confidence. External timelines are timing evidence, not decision ground truth.

Favor moments with a meaningful choice and visible consequence:

- contested or traded major objectives;
- fights around towers, inhibitors, or base pressure;
- picks that convert into an objective;
- failed contests, blind entries, overextensions, or forced recalls;
- numerical-disadvantage reversals, escapes, or match-ending decisions.

Avoid dumping routine uncontested objectives. Choose a compact representative set unless the user asks for exhaustive extraction. For a downscaled Playable Replays project, default to the strongest 1–3 project-ready scenarios. Include enough setup and aftermath to explain the decision, normally 20–45 seconds before and 15–45 seconds after, then adjust from commentary and visible action.

## Align in-game and VOD time

Never infer VOD time by adding game durations in an edited upload. Cuts, pauses, replays, and removed downtime make the offset piecewise.

1. Build anchors containing `game_id`, `game_time`, `vod_time_s`, evidence, and confidence.
2. Use visible scoreboard-clock OCR, event order, caption phrases, and external timelines together.
3. Scan locally around expected events, then manually inspect boundary and decision frames.
4. Use the nearest valid anchors within the same uninterrupted segment. Create a new segment after any edit, replay jump, or frozen clock.
5. Store alignment method and confidence per moment. Keep uncertain anchors explicit.

When Tesseract is available, use `scripts/scan_game_clock.py` over narrow candidate ranges. Supply an explicit scoreboard crop and game/segment IDs. Treat its JSONL output as unverified candidates until adjacent frames and event order agree.

## Transcribe and interpret commentary

Extract 16 kHz mono FLAC for every final clip. Keep three layers separate:

1. `transcript`: timestamped caption or ASR events, including source and confidence.
2. `caster_context`: conservative paraphrases linked to caption IDs or transcript spans.
3. `assessment`: independent coaching judgment with evidence, uncertainty, and a better alternative when applicable.

Do not turn uncertain automatic-caption text into precise quotations. Do not attribute a speaker unless diarization or the audio makes identity reliable. Label mixed feeds accurately, such as `co-streamer / broadcast feed`.

## Create coaching labels

Use `correct`, `incorrect`, `conditional`, or `uncertain`. Explain the evaluated decision, observable constraints, causal consequence, better alternative, evidence, and unknowns. Treat caster opinion as context, not truth. Separate mechanical execution from decision quality.

When targeting the downscaled Playable Replays simulator, recommend only `move`, `hold`, `contest`, or `retreat`. Do not emit removed actions such as `dodge` or `outplay`.

## Export a Playable Replays fixture pack

Treat replay analysis and simulator authoring as separate layers. First finish and validate the evidence dataset. Then select only one to three `project_ready` moments and author a complete v3.0 simulator draft following [references/playable-replays.md](references/playable-replays.md). Positions and gameplay values are normalized teaching approximations informed by reviewed frames, not replay-exact telemetry.

Assemble the canonical pack with:

```bash
python3 scripts/export_playable_replays.py export \
  --manifest deliverable/analysis/moments.json \
  --drafts deliverable/analysis/playable-replays-drafts.json \
  --schema /path/to/playable-replays/contracts/moment.schema.json \
  --output deliverable/playable-replays/moments.json \
  --project-root /path/to/playable-replays
```

Use the bundled schema snapshot only when no target checkout or newer canonical schema is available. The exporter derives `startTimeSeconds` and `replayEvidence` from the evidence manifest, rejects non-project-ready or weakly sourced drafts, validates the JSON Schema and project semantic invariants, and writes atomically. With `--project-root`, it also runs the project's engine-backed fixture and acceptance validator before finalizing.

Do not call a fixture project-usable unless validation against the target checkout passes. Schema success alone is insufficient: every acceptance line must execute to its declared win or loss. Do not replace a repository's `fixtures/moments.json` unless the user explicitly asks for that repository change.

## Render atomically

Create the render configuration from [references/output-contract.md](references/output-contract.md), then run:

```bash
python3 scripts/render_moments.py \
  --source source_media/match.mp4 \
  --moments work/render-moments.json \
  --output deliverable --jobs 3
```

The renderer writes each moment into a temporary directory, fully decodes timed media, checks duration, and only then moves files into the deliverable. Never accept file presence or plausible size as proof of completion. Use `--overwrite` only when replacement is intended. Choose the thumbnail at the decision frame and manually review the start, decision, and aftermath QA frames.

Slice normalized captions into clip-local transcript files:

```bash
python3 scripts/slice_transcripts.py \
  --captions work/captions/normalized.jsonl \
  --moments work/render-moments.json \
  --analysis deliverable/analysis/moments.json \
  --output deliverable/transcripts
```

This fails when a cited caster-context caption lies outside its clip, which means the clip boundary or citation must be corrected.

## Build and validate the deliverable

Follow [references/output-contract.md](references/output-contract.md). Copy `references/pivotal-moments.schema.json` into the deliverable's `schema/` directory and validate `analysis/moments.json` against it. For a Playable Replays export, also include the authoring draft, importable `playable-replays/moments.json`, and exact schema used; validate the export separately with `scripts/export_playable_replays.py`.

Before packaging:

1. Decode every MP4 and FLAC from start to finish.
2. Confirm boundaries against visible action, commentary setup, and aftermath.
3. Review every start/decision/aftermath frame.
4. Check manifest references, unique IDs, timestamp ordering, and durations.
5. Record SHA-256 hashes for source and deliverable assets.
6. Create a complete ZIP and a data-only ZIP when media is large.
7. Run archive integrity tests and compare downloads with the checksum manifest.

Use `scripts/validate_bundle.py` for contract/schema, checksum, media-probe, full-decode, and ZIP checks. Its self-contained `bundled-contract-1.0.0` engine enforces the shipped contract when the optional `jsonschema` package is absent. Fix every failure before delivery.

After validation writes `CHECKSUMS.sha256`, use `scripts/package_bundle.py` to create deterministic complete and data-only archives plus `DOWNLOADS.sha256`. Keep the full source separate unless requested.

```bash
python3 scripts/package_bundle.py deliverable \
  --output-dir artifacts \
  --prefix replay-dataset \
  --source source_media/match.mp4
```

## Deliver

Lead with source and bundle downloads, followed by a compact moment table with game, title, game-time window, and assessment. When created, link the importable Playable Replays fixture pack and state whether target-checkout acceptance validation passed. Disclose source-identity mismatches, caption limitations, diarization uncertainty, external timing sources, and authored-coordinate limitations.

Save user-facing outputs durably when available. Do not claim persistence unless saving returns a confirmed file identity or version. If durable saving fails, say so plainly and keep verified local links available while the workspace remains active.
