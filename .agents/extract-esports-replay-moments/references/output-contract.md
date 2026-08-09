# Replay dataset output contract

Read this file before creating or validating a replay dataset.

## Directory layout

```text
deliverable/
  README.md
  analysis/
    moments.json
    moments.csv
    coaching_explanations.md
    alignment.json
    playable-replays-drafts.json  # only for a Playable Replays export
  audio/<moment-id>.flac
  captions/raw/
  captions/normalized/
  clips/<moment-id>.mp4
  qa/<moment-id>_start.jpg
  qa/<moment-id>_decision.jpg
  qa/<moment-id>_aftermath.jpg
  research/sources.json
  research/external_timeline.json
  playable-replays/moments.json   # importable v3.0 fixture pack, when requested
  schema/pivotal-moments.schema.json
  schema/playable-replays-moment.schema.json  # exact project schema used
  thumbnails/<moment-id>.jpg
  transcripts/<moment-id>.json
  transcripts/<moment-id>.srt
  transcripts/<moment-id>.txt
  CHECKSUMS.sha256
```

The complete archive contains the whole tree. A data-only archive omits `clips/` and `audio/` but retains transcripts, analysis, research, schema, QA images, and checksums relevant to included files.

## Render configuration

Pass this smaller configuration to `scripts/render_moments.py`:

```json
{
  "moments": [
    {
      "id": "01_g1_dragon_fight",
      "vod_start_s": 615.25,
      "vod_decision_s": 638.8,
      "vod_end_s": 671.0
    }
  ]
}
```

IDs use only lowercase letters, digits, underscores, and hyphens. Timestamps are seconds in the source VOD and satisfy `start < decision < end`.

## Canonical analysis manifest

`analysis/moments.json` is the project-ingestion source of truth. Use schema version `1.0.0`:

```json
{
  "schema_version": "1.0.0",
  "source": {
    "url": "https://example.test/watch?v=source",
    "platform_id": "source",
    "title": "Verified broadcast title",
    "duration_s": 7226.65,
    "video_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
    "caption_sources": ["youtube-auto-en"]
  },
  "moments": [
    {
      "id": "01_g1_dragon_fight",
      "game_id": "game-1",
      "title": "Dragon contest becomes a rout",
      "event_type": "dragon-fight",
      "project_ready": true,
      "vod": {
        "start_s": 615.25,
        "decision_s": 638.8,
        "end_s": 671.0
      },
      "game_time": {
        "start": "15:25",
        "decision": "15:48",
        "end": "16:20"
      },
      "alignment": {
        "method": "scoreboard-ocr+event-timeline+captions",
        "confidence": "high",
        "anchor_ids": ["g1-a03", "g1-a04"]
      },
      "transcript": {
        "source": "youtube-auto-en",
        "speaker_label": "co-streamer / broadcast feed",
        "caption_ids": ["cap-001201", "cap-001202"],
        "limitations": ["automatic captions", "not diarized"]
      },
      "caster_context": {
        "summary": "Paraphrase of the relevant warning or reaction.",
        "caption_ids": ["cap-001201"]
      },
      "assessment": {
        "label": "conditional",
        "summary": "The initial trade is defensible; the follow-up fight is not.",
        "reasoning": "Observable state and causal explanation.",
        "better_alternative": "Take the cross-map value and disengage.",
        "recommended_action": "retreat",
        "uncertainty": "Hidden cooldown communication is unavailable."
      },
      "evidence": {
        "external_event_ids": ["timeline-g1-15-51-dragon"],
        "source_urls": ["https://example.test/timeline"]
      },
      "assets": {
        "clip": "clips/01_g1_dragon_fight.mp4",
        "audio": "audio/01_g1_dragon_fight.flac",
        "thumbnail": "thumbnails/01_g1_dragon_fight.jpg",
        "transcript_json": "transcripts/01_g1_dragon_fight.json",
        "transcript_srt": "transcripts/01_g1_dragon_fight.srt",
        "transcript_text": "transcripts/01_g1_dragon_fight.txt"
      }
    }
  ]
}
```

Use relative POSIX paths rooted at `deliverable/`; never use absolute paths or `..`. Keep CSV and Markdown as derived views of the JSON, not competing sources of truth.

Set `project_ready` on every moment. For the downscaled Playable Replays simulator, mark only the strongest 1–3 moments true and use `recommended_action` values `move`, `hold`, `contest`, or `retreat`.

When an importable Playable Replays pack is requested, read
`playable-replays.md`. Keep `analysis/moments.json` as the evidence source of
truth and `analysis/playable-replays-drafts.json` as the separate simulator
authoring layer. Produce `playable-replays/moments.json` only through
`scripts/export_playable_replays.py`; do not add project-only fields to this
canonical analysis manifest.

## Alignment file

`analysis/alignment.json` stores piecewise anchors:

```json
{
  "anchors": [
    {
      "id": "g1-a03",
      "game_id": "game-1",
      "segment_id": "game-1-live-02",
      "game_time": "15:25",
      "vod_time_s": 615.25,
      "evidence": ["scoreboard OCR", "dragon event order"],
      "confidence": "high"
    }
  ]
}
```

Start a new segment after an edit, replay jump, pause removal, or clock discontinuity. Never interpolate across segments.
Every `alignment.anchor_ids` entry in a moment must resolve to an `anchors[].id` in this file.

## External timeline file

Store `research/external_timeline.json` as an object with an `events` array. Give every event a unique `id`, game/event time, event type, evidence source, and confidence or value-status note. Local or synthetic event records are acceptable when clearly labeled. Every `evidence.external_event_ids` entry in a moment must resolve to an `events[].id` in this file.

## Transcript records

Store transcript JSON as an array of records:

```json
{
  "caption_id": "cap-001201",
  "start_s": 631.12,
  "end_s": 634.48,
  "text": "uncertain automatic-caption text",
  "source": "youtube-auto-en",
  "confidence": null
}
```

Clip-local SRT timestamps begin at zero. JSON may retain source-VOD and clip-local time. Never silently correct uncertain words; add a normalized or interpreted field while retaining raw text.

## Research provenance

For every external source record, keep page title, direct URL, access date, publisher, supported fields or events, whether each value is reported or inferred, and discrepancies against other sources.

## Checksums and archives

Use standard `sha256sum` format with paths relative to the deliverable root. Exclude the checksum file itself and sort paths bytewise. Test ZIPs with `unzip -t` and record archive hashes in a separate download manifest.
