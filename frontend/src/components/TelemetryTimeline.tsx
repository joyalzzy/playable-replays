import { useEffect, useMemo, useState } from "react";
import type {
  TelemetryCandidate,
  TelemetryTimeline as Timeline,
  TelemetryTimelineEvent,
  TelemetryTimelineUnit
} from "../types";

type TelemetryTimelineProps = {
  timeline: Timeline | null;
  candidate: TelemetryCandidate | null;
  loading: boolean;
};

type Track = {
  id: string;
  side: "a" | "b";
  points: string;
};

const trailLength = 18;

export function TelemetryTimeline({ timeline, candidate, loading }: TelemetryTimelineProps) {
  const frames = timeline?.frames ?? [];
  const firstSecond = frames[0]?.second ?? 0;
  const lastSecond = frames[frames.length - 1]?.second ?? 0;
  const [selectedSecond, setSelectedSecond] = useState(0);
  const [followLatest, setFollowLatest] = useState(true);
  const [playing, setPlaying] = useState(false);

  useEffect(() => {
    setFollowLatest(true);
    setPlaying(false);
  }, [timeline?.matchId]);

  useEffect(() => {
    if (followLatest && frames.length > 0) {
      setSelectedSecond(lastSecond);
    }
  }, [followLatest, frames.length, lastSecond]);

  useEffect(() => {
    if (!playing || frames.length === 0) return;
    const timer = window.setInterval(() => {
      setSelectedSecond((current) => {
        const next = frames.find((frame) => frame.second > current);
        if (next) return next.second;
        setPlaying(false);
        return lastSecond;
      });
    }, 650);
    return () => window.clearInterval(timer);
  }, [frames, lastSecond, playing]);

  const selectedFrame = useMemo(() => {
    if (frames.length === 0) return null;
    return frames.reduce((closest, frame) =>
      Math.abs(frame.second - selectedSecond) < Math.abs(closest.second - selectedSecond) ? frame : closest
    );
  }, [frames, selectedSecond]);

  const trails = useMemo<Track[]>(() => {
    if (!selectedFrame) return [];
    const history = frames.filter((frame) => frame.second <= selectedFrame.second).slice(-trailLength);
    const byTrack = new Map<string, { side: "a" | "b"; points: string[] }>();
    history.forEach((frame) => frame.units.forEach((unit) => {
      const track = byTrack.get(unit.trackId) ?? { side: unit.side, points: [] };
      track.points.push(`${unit.position.x},${unit.position.y}`);
      byTrack.set(unit.trackId, track);
    }));
    return [...byTrack.entries()]
      .map(([id, track]) => ({ id, side: track.side, points: track.points.join(" ") }))
      .sort((a, b) => a.id.localeCompare(b.id));
  }, [frames, selectedFrame]);

  const currentEvents = useMemo(() => {
    if (!selectedFrame || !timeline) return [];
    return timeline.events.filter((event) => event.second === selectedFrame.second);
  }, [selectedFrame, timeline]);

  const reversalSecond = candidate?.detection.semanticEvidence.teamFightReversalSecond ?? null;
  const duration = Math.max(1, lastSecond - firstSecond);
  const percentAt = (second: number) => Math.max(0, Math.min(100, ((second - firstSecond) / duration) * 100));

  function chooseSecond(second: number) {
    setSelectedSecond(second);
    setFollowLatest(false);
    setPlaying(false);
  }

  function togglePlayback() {
    setFollowLatest(false);
    if (!playing && selectedSecond >= lastSecond) {
      setSelectedSecond(candidate ? Math.max(firstSecond, candidate.detection.startSecond) : firstSecond);
    }
    setPlaying((current) => !current);
  }

  if (loading && frames.length === 0) {
    return <section className="telemetry-timeline telemetry-timeline--empty" aria-label="Visual telemetry timeline"><p>Loading the bounded visual timeline…</p></section>;
  }

  if (!timeline || frames.length === 0) {
    return (
      <section className="telemetry-timeline telemetry-timeline--empty" aria-label="Visual telemetry timeline">
        <div><p className="eyebrow">VISUAL TIMELINE</p><h3>Movement will appear with the first accepted frame</h3></div>
        <p>Only anonymous A/B tracks, normalized positions, and normalized event types are displayed.</p>
      </section>
    );
  }

  return (
    <section className="telemetry-timeline" aria-labelledby="telemetry-timeline-title">
      <header className="telemetry-timeline__header">
        <div>
          <p className="eyebrow">IDENTITY-FREE VISUAL TIMELINE</p>
          <h3 id="telemetry-timeline-title">What happened on the map</h3>
          <p>Trace anonymous unit movement, event timing, and the detector’s strongest decision window.</p>
        </div>
        <div className="telemetry-timeline__privacy">
          <span>Source IDs removed</span>
          <strong>{timeline.truncated ? `Sampled every ${timeline.sampleEvery} frames` : `All ${timeline.sourceFrameCount} frames shown`}</strong>
        </div>
      </header>

      <div className="telemetry-timeline__body">
        <div className="telemetry-map" role="img" aria-label={`Anonymous tactical map at ${selectedFrame?.second ?? selectedSecond} seconds`}>
          <svg viewBox="0 0 100 100" aria-hidden="true">
            <defs>
              <linearGradient id="telemetry-river" x1="0" y1="0" x2="1" y2="1"><stop stopColor="#164c63" /><stop offset="1" stopColor="#14364f" /></linearGradient>
              <filter id="telemetry-glow"><feGaussianBlur stdDeviation="1.4" result="blur" /><feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge></filter>
            </defs>
            <rect className="telemetry-map__ground" x="0" y="0" width="100" height="100" rx="5" />
            <path className="telemetry-map__river" d="M -8 12 L 12 -8 L 108 88 L 88 108 Z" />
            <path className="telemetry-map__lane" d="M 9 91 L 50 50 L 91 9" />
            <path className="telemetry-map__lane telemetry-map__lane--side" d="M 8 91 L 8 27 L 27 8 L 91 8" />
            <path className="telemetry-map__lane telemetry-map__lane--side" d="M 9 92 L 73 92 L 92 73 L 92 9" />
            <circle className="telemetry-map__base telemetry-map__base--a" cx="9" cy="91" r="4" />
            <circle className="telemetry-map__base telemetry-map__base--b" cx="91" cy="9" r="4" />
            {trails.map((track) => <polyline key={track.id} className={`telemetry-track telemetry-track--${track.side}`} points={track.points} />)}
            {selectedFrame?.units.map((unit) => <TimelineUnitMarker key={unit.trackId} unit={unit} reversal={reversalSecond === selectedFrame.second} />)}
          </svg>
          <div className="telemetry-map__legend"><span className="is-a">Side A</span><span className="is-b">Side B</span><span>Last {Math.min(trailLength, frames.length)} samples traced</span></div>
        </div>

        <aside className="telemetry-timeline__readout" aria-live="polite">
          <div className="telemetry-time-readout"><span>Selected time</span><strong>{selectedFrame?.second ?? selectedSecond}s</strong></div>
          {candidate ? (
            <div className="telemetry-window-card">
              <span>Detected window</span>
              <strong>{candidate.detection.startSecond}s–{candidate.detection.endSecond}s</strong>
              <p>{Math.round(candidate.detection.score * 100)}/100 highlight score</p>
            </div>
          ) : <p className="telemetry-muted">No window has crossed the detector threshold yet.</p>}
          {reversalSecond !== null && <div className="telemetry-reversal"><span aria-hidden="true">↺</span><div><strong>Reversal at {reversalSecond}s</strong><p>The probability direction changed here with nearby combat evidence.</p></div></div>}
          <div className="telemetry-event-readout">
            <span>Events at this sample</span>
            {currentEvents.length === 0
              ? <p>None recorded</p>
              : currentEvents.map((event) => <EventBadge key={`${event.second}-${event.type}`} event={event} />)}
          </div>
        </aside>
      </div>

      <div className="telemetry-timeline__controls">
        <div className="telemetry-rail" aria-hidden="true">
          {candidate && <i className="telemetry-rail__window" style={{ left: `${percentAt(candidate.detection.startSecond)}%`, width: `${Math.max(1, percentAt(candidate.detection.endSecond) - percentAt(candidate.detection.startSecond))}%` }} />}
          {reversalSecond !== null && <i className="telemetry-rail__reversal" style={{ left: `${percentAt(reversalSecond)}%` }} />}
          {timeline.events.map((event, index) => <i key={`${event.second}-${event.type}-${index}`} className={`telemetry-rail__event telemetry-rail__event--${event.type}`} style={{ left: `${percentAt(event.second)}%` }} />)}
        </div>
        <input
          aria-label="Telemetry time"
          type="range"
          min={firstSecond}
          max={lastSecond}
          step={1}
          value={Math.max(firstSecond, Math.min(lastSecond, selectedSecond))}
          onChange={(event) => chooseSecond(Number(event.target.value))}
        />
        <div className="telemetry-timeline__ticks"><span>{firstSecond}s</span><span>{lastSecond}s</span></div>
        <div className="telemetry-playback">
          <button type="button" onClick={togglePlayback}>{playing ? "Pause timeline" : "Play timeline"}</button>
          {candidate && <button type="button" onClick={() => chooseSecond(reversalSecond ?? candidate.detection.startSecond)}>Jump to detected moment</button>}
          <button type="button" onClick={() => { setFollowLatest(true); setPlaying(false); setSelectedSecond(lastSecond); }}>Latest frame</button>
        </div>
      </div>
    </section>
  );
}

function TimelineUnitMarker({ unit, reversal }: { unit: TelemetryTimelineUnit; reversal: boolean }) {
  return (
    <g className={`telemetry-unit telemetry-unit--${unit.side} ${unit.alive ? "" : "is-down"} ${reversal ? "is-reversal" : ""}`} transform={`translate(${unit.position.x} ${unit.position.y})`}>
      {reversal && <circle className="telemetry-unit__reversal" r="5" />}
      <circle r="2.9" />
      <text y="0.8" textAnchor="middle">{unit.trackId}</text>
    </g>
  );
}

function EventBadge({ event }: { event: TelemetryTimelineEvent }) {
  return <div className={`telemetry-event-badge telemetry-event-badge--${event.type}`}><i aria-hidden="true">{eventIcon(event.type)}</i><span>{eventLabel(event.type)}</span><b>×{event.count}</b></div>;
}

function eventLabel(type: TelemetryTimelineEvent["type"]): string {
  if (type === "vision-loss") return "Vision loss";
  return type.charAt(0).toUpperCase() + type.slice(1);
}

function eventIcon(type: TelemetryTimelineEvent["type"]): string {
  if (type === "damage") return "✦";
  if (type === "kill") return "×";
  if (type === "objective") return "◆";
  return "◐";
}
