import { useCallback, useEffect, useMemo, useState } from "react";
import { createTelemetryDraft, deleteAllTelemetryMatches, deleteTelemetryMatch, getLocalStorageStatus, getTelemetryDraft, getTelemetryMatch, getTelemetryTimeline, listTelemetryMatches, subscribeTelemetryMatch, updateLocalStorageRetention } from "../api";
import type { DraftPreview, LocalStorageStatus, TelemetryCandidate, TelemetryDraftResult, TelemetryMatch, TelemetrySignals, TelemetryTimeline as Timeline } from "../types";
import { DraftWorkbench } from "./DraftWorkbench";
import { LocalDataControls } from "./LocalDataControls";
import { TelemetryTimeline } from "./TelemetryTimeline";

type TelemetryHubProps = {
  onBack: () => void;
  onOpenTutorial: () => void;
  onPreviewDraft?: (preview: DraftPreview) => void;
};

const signalLabels: Array<{ key: keyof TelemetrySignals; label: string; invert?: boolean }> = [
  { key: "winProbabilitySwing", label: "Outcome swing" },
  { key: "eventDensity", label: "Combat activity" },
  { key: "entityProximity", label: "Close combat", invert: true },
  { key: "resourceAsymmetry", label: "Resource imbalance" }
];

export function TelemetryHub({ onBack, onOpenTutorial, onPreviewDraft }: TelemetryHubProps) {
  const [matches, setMatches] = useState<TelemetryMatch[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [match, setMatch] = useState<TelemetryMatch | null>(null);
  const [draft, setDraft] = useState<TelemetryDraftResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [streamConnected, setStreamConnected] = useState(false);
  const [draftingId, setDraftingId] = useState("");
  const [timeline, setTimeline] = useState<Timeline | null>(null);
  const [timelineLoading, setTimelineLoading] = useState(false);
  const [storage, setStorage] = useState<LocalStorageStatus | null>(null);
  const [storageBusy, setStorageBusy] = useState(false);

  const acceptMatch = useCallback((next: TelemetryMatch) => {
    setMatch(next);
    setMatches((current) => {
      const without = current.filter((item) => item.id !== next.id);
      return [next, ...without];
    });
  }, []);

  const refresh = useCallback(async () => {
    try {
      const next = await listTelemetryMatches();
      setMatches(next);
      setSelectedId((current) => current && next.some((item) => item.id === current) ? current : next[0]?.id ?? "");
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not reach the local telemetry service.");
    } finally {
      setLoading(false);
    }
  }, []);

  const refreshStorage = useCallback(async () => {
    try {
      setStorage(await getLocalStorageStatus());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not inspect local storage.");
    }
  }, []);

  useEffect(() => {
    void refresh();
    void refreshStorage();
    const timer = window.setInterval(() => void refresh(), 3000);
    const storageTimer = window.setInterval(() => void refreshStorage(), 5_000);
    return () => {
      window.clearInterval(timer);
      window.clearInterval(storageTimer);
    };
  }, [refresh, refreshStorage]);

  const selectedTimelineAvailable = useMemo(
    () => matches.find((item) => item.id === selectedId)?.timelineAvailable ?? true,
    [matches, selectedId]
  );

  useEffect(() => {
    if (!selectedId) {
      setMatch(null);
      setStreamConnected(false);
      return;
    }
    let active = true;
    void getTelemetryMatch(selectedId)
      .then((next) => active && acceptMatch(next))
      .catch((caught: unknown) => active && setError(caught instanceof Error ? caught.message : "Could not load the match."));
    let close: () => void = () => undefined;
    if (selectedTimelineAvailable) {
      close = subscribeTelemetryMatch(
        selectedId,
        (next) => active && acceptMatch(next),
        (connected) => active && setStreamConnected(connected)
      );
    } else {
      setStreamConnected(false);
    }
    return () => {
      active = false;
      close();
    };
  }, [acceptMatch, selectedId, selectedTimelineAvailable]);

  useEffect(() => {
    if (!selectedId) {
      setTimeline(null);
      setTimelineLoading(false);
      return;
    }
    if (!selectedTimelineAvailable) {
      setTimeline(null);
      setTimelineLoading(false);
      return;
    }
    let active = true;
    let firstLoad = true;
    const load = async () => {
      if (firstLoad) setTimelineLoading(true);
      try {
        const next = await getTelemetryTimeline(selectedId);
        if (active) setTimeline(next);
      } catch (caught) {
        if (active && firstLoad) {
          setError(caught instanceof Error ? caught.message : "Could not load the visual telemetry timeline.");
        }
      } finally {
        if (active && firstLoad) setTimelineLoading(false);
        firstLoad = false;
      }
    };
    setTimeline(null);
    void load();
    const timer = window.setInterval(() => void load(), 1000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [selectedId, selectedTimelineAvailable]);

  const strongestCandidate = useMemo(() => {
    if (!match?.candidates.length) return null;
    return [...match.candidates].sort((a, b) => b.detection.score - a.detection.score)[0] ?? null;
  }, [match]);

  async function guardDraft(candidate: TelemetryCandidate) {
    if (!match || match.status !== "finalized") return;
    setDraftingId(candidate.id);
    setDraft(null);
    try {
      const result = await createTelemetryDraft(match.id, candidate.id);
      setDraft(result);
      const current = await getTelemetryMatch(match.id);
      acceptMatch(current);
      await refreshStorage();
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The guarded draft could not be created.");
    } finally {
      setDraftingId("");
    }
  }

  async function openDraft(candidate: TelemetryCandidate) {
    if (!match) return;
    setDraftingId(candidate.id);
    try {
      setDraft(await getTelemetryDraft(match.id, candidate.id));
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The draft could not be opened.");
    } finally {
      setDraftingId("");
    }
  }

  async function changeRetention(days: number) {
    setStorageBusy(true);
    try {
      setStorage(await updateLocalStorageRetention(days));
      await refresh();
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not update local retention.");
    } finally {
      setStorageBusy(false);
    }
  }

  async function deleteSelected() {
    if (!selectedId) return;
    setStorageBusy(true);
    try {
      await deleteTelemetryMatch(selectedId);
      setMatch(null);
      setDraft(null);
      setTimeline(null);
      setSelectedId("");
      await Promise.all([refresh(), refreshStorage()]);
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not delete the selected telemetry data.");
    } finally {
      setStorageBusy(false);
    }
  }

  async function deleteAll() {
    setStorageBusy(true);
    try {
      await deleteAllTelemetryMatches();
      setMatches([]);
      setSelectedId("");
      setMatch(null);
      setDraft(null);
      setTimeline(null);
      await refreshStorage();
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not delete local telemetry data.");
    } finally {
      setStorageBusy(false);
    }
  }

  return (
    <main className="section-shell telemetry-hub">
      <header className="section-topbar">
        <div>
          <p className="eyebrow">PLAYABLE / REPLAYS</p>
          <h1>Match Telemetry</h1>
        </div>
        <button className="menu-return" type="button" onClick={onBack}>Back to main menu</button>
      </header>

      <section className="telemetry-live" aria-live="polite">
        <div className="telemetry-live__heading">
          <div>
            <p className="eyebrow">LOCAL LIVE DETECTOR</p>
            <h2>{match ? "Match signal monitor" : "Waiting for a telemetry replay"}</h2>
            <p>
              {match
                ? "Frames are scored as they arrive. A highlight stays provisional until the replay ends."
                : "Start the local collector with an authorized or synthetic normalized replay. This screen will update automatically."}
            </p>
          </div>
          <div className={`telemetry-connection ${streamConnected ? "is-live" : ""}`}>
            <span aria-hidden="true" />
            {match ? (streamConnected ? "Live updates" : "Polling local service") : loading ? "Checking service" : "Collector idle"}
          </div>
        </div>

        {error && <p className="telemetry-error" role="alert">{error}</p>}

        {!match ? (
          <div className="telemetry-empty">
            <div className="telemetry-empty__pulse" aria-hidden="true"><span /><span /><span /><span /><span /></div>
            <div>
              <strong>Replay the included safe demo</strong>
              <p>With the local API running, use the telemetry collector on <code>fixtures/telemetry-demo.json</code>. No player names or account identifiers are accepted.</p>
              <code>go run ./cmd/telemetry-collector --input ../fixtures/telemetry-demo.json --rate 4</code>
            </div>
          </div>
        ) : (
          <>
            <div className="telemetry-matchbar">
              <label>
                Replay
                <select value={selectedId} onChange={(event) => { setSelectedId(event.target.value); setDraft(null); }}>
                  {matches.map((item) => <option key={item.id} value={item.id}>{item.id}</option>)}
                </select>
              </label>
              <div><span>Status</span><strong className={`telemetry-state telemetry-state--${match.status}`}>{match.status}</strong></div>
              <div><span>Frames received</span><strong>{match.frameCount}</strong></div>
              <div><span>Latest time</span><strong>{Math.max(0, match.lastSecond)}s</strong></div>
              <div><span>Highlights</span><strong>{match.candidates.length}</strong></div>
              <div><span>Local copy</span><strong>{match.savedLocally ? "Saved" : "Not saved yet"}</strong></div>
            </div>

            {match.timelineAvailable
              ? <TelemetryTimeline timeline={timeline} candidate={strongestCandidate} loading={timelineLoading} />
              : <section className="telemetry-timeline telemetry-timeline--empty" aria-label="Visual telemetry timeline"><div><p className="eyebrow">SUMMARY RESTORED</p><h3>The movement timeline was not persisted</h3></div><p>Raw frames and positional traces remain memory-only. The safe match summary and analyst drafts are still available.</p></section>}

            <div className="telemetry-monitor-grid">
              <section className="telemetry-signal-panel">
                <div className="telemetry-panel-title">
                  <div><p className="eyebrow">STRONGEST WINDOW</p><h3>{strongestCandidate ? `${strongestCandidate.detection.startSecond}s–${strongestCandidate.detection.endSecond}s` : "Collecting enough frames"}</h3></div>
                  {strongestCandidate && <strong className="telemetry-score">{Math.round(strongestCandidate.detection.score * 100)}<small>/100</small></strong>}
                </div>
                {strongestCandidate ? (
                  <div className="telemetry-signal-list">
                    {signalLabels.map(({ key, label, invert }) => {
                      const raw = strongestCandidate.detection.signals[key];
                      const value = invert ? 1 - raw : raw;
                      return <div key={key}><span>{label}</span><div><i style={{ width: `${Math.max(0, Math.min(100, value * 100))}%` }} /></div><b>{Math.round(value * 100)}%</b></div>;
                    })}
                  </div>
                ) : <p className="telemetry-muted">A complete 12-second window is required before a highlight can appear.</p>}
              </section>

              <section className="telemetry-candidates">
                <div className="telemetry-panel-title"><div><p className="eyebrow">DETECTED HIGHLIGHTS</p><h3>Analyst review queue</h3></div></div>
                {match.candidates.length === 0 ? <p className="telemetry-muted">No window has crossed the detector threshold yet.</p> : match.candidates.map((candidate) => (
                  <article key={candidate.id}>
                    <div>
                      <span className={`candidate-status candidate-status--${candidate.status}`}>{candidate.status}</span>
                      <strong>{categoryLabel(candidate.category)}</strong>
                      <p>{candidate.detection.startSecond}s–{candidate.detection.endSecond}s · {candidate.detection.reasonTags.map(tagLabel).join(" · ")}</p>
                    </div>
                    <button
                      type="button"
                      disabled={match.status !== "finalized" || draftingId === candidate.id}
                      onClick={() => void (candidate.draftStatus === "not-created" ? guardDraft(candidate) : openDraft(candidate))}
                    >
                      {match.status !== "finalized"
                        ? "Available after replay"
                        : draftingId === candidate.id
                          ? "Opening…"
                          : candidate.draftStatus === "ready"
                            ? "Open ready draft"
                            : candidate.draftStatus === "incomplete"
                              ? "Continue authoring"
                              : "Create guarded draft"}
                    </button>
                  </article>
                ))}
              </section>
            </div>
          </>
        )}
      </section>

      {draft && match && (
        <DraftWorkbench
          matchId={match.id}
          draft={draft}
          onChange={setDraft}
          onPreview={onPreviewDraft ?? (() => undefined)}
        />
      )}

      <LocalDataControls
        status={storage}
        selectedMatchId={selectedId}
        busy={storageBusy}
        onRetentionChange={changeRetention}
        onDeleteSelected={deleteSelected}
        onDeleteAll={deleteAll}
      />

      <section className="telemetry-flow" aria-label="Telemetry scenario workflow">
        <article><span>01</span><div><strong>Detect</strong><p>Score fully covered telemetry windows and preserve timestamps, signals, labels, and evidence.</p></div></article>
        <article><span>02</span><div><strong>Draft</strong><p>Map the detector label to a tactical category and create a version 2.1 draft fixture.</p></div></article>
        <article><span>03</span><div><strong>Author</strong><p>Add synthetic units, rules, rationale, tradeoffs, alternatives, and win/loss acceptance tests.</p></div></article>
        <article><span>04</span><div><strong>Validate and preview</strong><p>Run deterministic checks before the scenario can enter a review pack.</p></div></article>
      </section>

      <section className="telemetry-status">
        <div>
          <p className="eyebrow">CURRENT BOUNDARY</p>
          <h3>Analyst-reviewed, not automatic publishing</h3>
          <p>Only normalized, authorized or synthetic telemetry is accepted. Drafts remain locked until rationale, tradeoffs, alternatives, and acceptance tests are authored.</p>
        </div>
        <button className="primary-link" type="button" onClick={onOpenTutorial}>Practice a fixed scenario</button>
      </section>
    </main>
  );
}

function categoryLabel(value: string): string {
  return value.split("-").map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(" ");
}

function tagLabel(value: string): string {
  return value.replaceAll("-", " ");
}
